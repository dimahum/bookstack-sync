// Package syncer implements the logic for syncing a local directory of
// Markdown files to BookStack.
package syncer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	bookstack "github.com/dimahum/bookstack-api"
)

// Config holds all parameters needed to run a sync.
type Config struct {
	// URL is the base URL of the BookStack instance.
	URL string
	// TokenID is the BookStack API token ID.
	TokenID string
	// TokenSecret is the BookStack API token secret.
	TokenSecret string
	// ShelfName is the name of the shelf to add the new book to.
	// If empty, the book is created without a shelf.
	ShelfName string
	// Dir is the local directory to sync from.
	Dir string
	// Excludes is a list of file or directory names to skip.
	Excludes []string
}

// Run performs the sync from the local directory to BookStack.
func Run(cfg Config) error {
	client := bookstack.NewClient(cfg.URL, cfg.TokenID, cfg.TokenSecret)
	client.WithHTTPClient(&http.Client{Timeout: 60 * time.Second})

	absDir, err := filepath.Abs(cfg.Dir)
	if err != nil {
		return fmt.Errorf("resolving directory: %w", err)
	}
	bookName := filepath.Base(absDir)

	// Optionally find the target shelf by name.
	var shelfID int
	if cfg.ShelfName != "" {
		shelfID, err = findShelfByName(client, cfg.ShelfName)
		if err != nil {
			return fmt.Errorf("finding shelf %q: %w", cfg.ShelfName, err)
		}
		if shelfID == 0 {
			return fmt.Errorf("shelf %q not found", cfg.ShelfName)
		}
	}

	// Create the book.
	book, err := client.CreateBook(&bookstack.CreateBookRequest{Name: bookName})
	if err != nil {
		return fmt.Errorf("creating book %q: %w", bookName, err)
	}
	log.Printf("Created book %q (ID=%d)", book.Name, book.ID)

	// Add the new book to the shelf.
	if shelfID > 0 {
		if err := addBookToShelf(client, shelfID, book.ID); err != nil {
			return fmt.Errorf("adding book to shelf: %w", err)
		}
		log.Printf("Added book to shelf ID=%d", shelfID)
	}

	excludeSet := buildExcludeSet(cfg.Excludes)

	entries, err := os.ReadDir(absDir)
	if err != nil {
		return fmt.Errorf("reading directory %q: %w", absDir, err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if excludeSet[name] {
			continue
		}

		fullPath := filepath.Join(absDir, name)

		if entry.IsDir() {
			mdFiles, err := listMdFiles(fullPath, excludeSet)
			if err != nil {
				return fmt.Errorf("listing md files in %q: %w", fullPath, err)
			}
			if len(mdFiles) == 0 {
				continue
			}

			chapter, err := client.CreateChapter(&bookstack.CreateChapterRequest{
				BookID: book.ID,
				Name:   name,
			})
			if err != nil {
				return fmt.Errorf("creating chapter %q: %w", name, err)
			}
			log.Printf("  Created chapter %q (ID=%d)", chapter.Name, chapter.ID)

			for _, mdPath := range mdFiles {
				if err := syncPage(client, cfg, mdPath, book.ID, chapter.ID); err != nil {
					return err
				}
			}
		} else if strings.HasSuffix(name, ".md") {
			if err := syncPage(client, cfg, fullPath, book.ID, 0); err != nil {
				return err
			}
		}
	}

	return nil
}

// findShelfByName returns the ID of the first shelf with the given name, or 0
// if none is found.
func findShelfByName(client *bookstack.Client, name string) (int, error) {
	opts := &bookstack.ListOptions{Count: 500}
	for {
		resp, err := client.ListShelves(opts)
		if err != nil {
			return 0, err
		}
		for _, s := range resp.Data {
			if s.Name == name {
				return s.ID, nil
			}
		}
		if opts.Offset+len(resp.Data) >= resp.Total {
			break
		}
		opts.Offset += len(resp.Data)
	}
	return 0, nil
}

// addBookToShelf appends a book ID to an existing shelf.
func addBookToShelf(client *bookstack.Client, shelfID, bookID int) error {
	shelf, err := client.GetShelf(shelfID)
	if err != nil {
		return err
	}
	ids := make([]int, 0, len(shelf.Books)+1)
	for _, b := range shelf.Books {
		ids = append(ids, b.ID)
	}
	ids = append(ids, bookID)
	_, err = client.UpdateShelf(shelfID, &bookstack.UpdateShelfRequest{Books: ids})
	return err
}

// buildExcludeSet converts the excludes slice into a set for O(1) lookup.
func buildExcludeSet(excludes []string) map[string]bool {
	set := make(map[string]bool, len(excludes))
	for _, e := range excludes {
		set[filepath.Base(e)] = true
	}
	return set
}

// listMdFiles returns the paths of all .md files directly inside dir,
// skipping any names present in excludeSet.
func listMdFiles(dir string, excludeSet map[string]bool) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if excludeSet[e.Name()] {
			continue
		}
		if strings.HasSuffix(e.Name(), ".md") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files, nil
}

// syncPage creates a BookStack page from a local Markdown file.
// If chapterID > 0 the page is created inside that chapter; otherwise it
// belongs directly to the book.
func syncPage(client *bookstack.Client, cfg Config, mdPath string, bookID, chapterID int) error {
	raw, err := os.ReadFile(mdPath)
	if err != nil {
		return fmt.Errorf("reading %q: %w", mdPath, err)
	}

	pageName := strings.TrimSuffix(filepath.Base(mdPath), ".md")

	// Create the page with the raw content first so we have a page ID.
	req := &bookstack.CreatePageRequest{
		Name:     pageName,
		Markdown: string(raw),
	}
	if chapterID > 0 {
		req.ChapterID = chapterID
	} else {
		req.BookID = bookID
	}

	page, err := client.CreatePage(req)
	if err != nil {
		return fmt.Errorf("creating page %q: %w", pageName, err)
	}
	log.Printf("    Created page %q (ID=%d)", page.Name, page.ID)

	// Process images: upload local images as attachments and rewrite links.
	processed, err := processImages(cfg, string(raw), mdPath, page.ID)
	if err != nil {
		return fmt.Errorf("processing images for %q: %w", mdPath, err)
	}

	if processed != string(raw) {
		if _, err := client.UpdatePage(page.ID, &bookstack.UpdatePageRequest{
			Markdown: processed,
		}); err != nil {
			return fmt.Errorf("updating page %q with image links: %w", pageName, err)
		}
	}

	return nil
}

// imageRefRe matches Markdown image syntax: ![alt text](path/or/url).
var imageRefRe = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)

// processImages scans mdContent for local image references, uploads each
// image as a BookStack attachment on the given page, and returns the
// Markdown with those references replaced by attachment links.
func processImages(cfg Config, mdContent, mdPath string, pageID int) (string, error) {
	var processErr error

	result := imageRefRe.ReplaceAllStringFunc(mdContent, func(match string) string {
		if processErr != nil {
			return match
		}

		sub := imageRefRe.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		alt := sub[1]
		imgRef := sub[2]

		// Skip external URLs.
		if strings.HasPrefix(imgRef, "http://") || strings.HasPrefix(imgRef, "https://") {
			return match
		}

		// Resolve the image path relative to the Markdown file.
		imgPath := imgRef
		if !filepath.IsAbs(imgRef) {
			imgPath = filepath.Join(filepath.Dir(mdPath), imgRef)
		}

		attURL, err := uploadFileAttachment(cfg, imgPath, pageID)
		if err != nil {
			processErr = err
			return match
		}

		// Replace the inline image with a Markdown link to the attachment.
		return fmt.Sprintf("[%s](%s)", alt, attURL)
	})

	return result, processErr
}

// attachmentResponse is the minimal JSON response from POST /api/attachments
// when uploading a file.
type attachmentResponse struct {
	ID int `json:"id"`
}

// uploadFileAttachment uploads imgPath as a file attachment on the given
// BookStack page and returns the URL to the attachment.
func uploadFileAttachment(cfg Config, imgPath string, pageID int) (string, error) {
	f, err := os.Open(imgPath)
	if err != nil {
		return "", fmt.Errorf("opening image %q: %w", imgPath, err)
	}
	defer f.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	if err := w.WriteField("name", filepath.Base(imgPath)); err != nil {
		return "", err
	}
	if err := w.WriteField("uploaded_to", fmt.Sprintf("%d", pageID)); err != nil {
		return "", err
	}

	fw, err := w.CreateFormFile("file", filepath.Base(imgPath))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(fw, f); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}

	apiURL := strings.TrimRight(cfg.URL, "/") + "/api/attachments"
	req, err := http.NewRequest(http.MethodPost, apiURL, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Token "+cfg.TokenID+":"+cfg.TokenSecret)
	req.Header.Set("Content-Type", w.FormDataContentType())

	httpClient := &http.Client{Timeout: 60 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("uploading image %q: %w", imgPath, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading attachment response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("uploading image %q: HTTP %d – %s", imgPath, resp.StatusCode, string(body))
	}

	var att attachmentResponse
	if err := json.Unmarshal(body, &att); err != nil {
		return "", fmt.Errorf("decoding attachment response: %w", err)
	}

	return strings.TrimRight(cfg.URL, "/") + "/attachments/" + fmt.Sprintf("%d", att.ID), nil
}
