// Command integration runs a full end-to-end sync against a live BookStack
// instance and verifies that the resulting book structure matches the
// testdata/example directory tree:
//
//	example/          → book "example"
//	  README.md       → root page "README"
//	  guide/          → chapter "guide"
//	    intro.md      → page "intro"
//	    advanced.md   → page "advanced"
//	  api/            → chapter "api"
//	    overview.md   → page "overview"
//
// The test runs the sync twice:
//   - Round 1 syncs the unmodified testdata/example tree and verifies the
//     resulting book structure.
//   - Round 2 copies the tree to a temp directory, updates every .md file
//     with a modified heading, re-runs the sync, and then verifies that no
//     duplicates were created and that page content was updated in place.
//
// Required environment variables:
//
//	BOOKSTACK_URL          – base URL of the BookStack instance
//	BOOKSTACK_TOKEN_ID     – API token ID
//	BOOKSTACK_TOKEN_SECRET – API token secret
//
// Usage (from the repo root):
//
//	go run ./tests/integration/
package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	bookstack "github.com/dimahum/bookstack-api"
	"github.com/dimahum/bookstack-sync/internal/syncer"
)

func main() {
	bsURL := requireEnv("BOOKSTACK_URL")
	tokenID := requireEnv("BOOKSTACK_TOKEN_ID")
	tokenSecret := requireEnv("BOOKSTACK_TOKEN_SECRET")

	client := bookstack.NewClient(bsURL, tokenID, tokenSecret)

	wd, err := os.Getwd()
	mustOK(err, "get working directory")
	exampleDir := filepath.Join(wd, "testdata", "example")

	fmt.Println("=== Integration Test: bookstack-sync ===")
	fmt.Printf("Syncing %q to %s\n\n", exampleDir, bsURL)

	// -----------------------------------------------------------------------
	// Step 1: Create the "Example" shelf.
	// -----------------------------------------------------------------------
	shelf, err := client.CreateShelf(&bookstack.CreateShelfRequest{Name: "Example"})
	mustOK(err, "create shelf")
	fmt.Printf("[OK] Created shelf [%d]: %q\n", shelf.ID, shelf.Name)

	cfg := syncer.Config{
		URL:         bsURL,
		TokenID:     tokenID,
		TokenSecret: tokenSecret,
		Dir:         exampleDir,
		ShelfName:   "Example",
	}

	// -----------------------------------------------------------------------
	// Step 2 – Round 1: initial sync.
	// -----------------------------------------------------------------------
	fmt.Println("\n--- Round 1: initial sync ---")
	mustOK(syncer.Run(cfg), "syncer.Run (round 1)")
	fmt.Println("[OK] Round 1 sync completed")

	bookID := findBook(client, "example")
	verifyBookStructure(client, bookID, "round 1")

	// -----------------------------------------------------------------------
	// Step 3 – Round 2: modify files, re-sync, verify no duplicates and
	// updated content.
	// -----------------------------------------------------------------------
	fmt.Println("\n--- Round 2: modified-content re-sync ---")

	// Copy testdata/example into a temp directory so we don't touch the repo.
	tmpDir, err := os.MkdirTemp("", "bookstack-sync-integration-*")
	mustOK(err, "create temp dir")
	defer os.RemoveAll(tmpDir)

	tmpExampleDir := filepath.Join(tmpDir, "example")
	mustOK(copyDir(exampleDir, tmpExampleDir), "copy testdata/example to temp dir")

	// Modify every .md file: prepend "UPDATED: " to the first heading.
	mustOK(modifyMdFiles(tmpExampleDir), "modify md files")

	cfg2 := syncer.Config{
		URL:         bsURL,
		TokenID:     tokenID,
		TokenSecret: tokenSecret,
		Dir:         tmpExampleDir,
		ShelfName:   "Example",
	}
	mustOK(syncer.Run(cfg2), "syncer.Run (round 2)")
	fmt.Println("[OK] Round 2 sync completed")

	// Verify no duplicates: there must still be exactly one book named "example".
	booksResp, err := client.ListBooks(nil)
	mustOK(err, "list books after round 2")
	var bookCount int
	for _, b := range booksResp.Data {
		if b.Name == "example" {
			bookCount++
		}
	}
	if bookCount != 1 {
		log.Fatalf("[FAIL] expected exactly 1 book named \"example\" after round 2, got %d", bookCount)
	}
	fmt.Println("[OK] No duplicate books created")

	bookID2 := findBook(client, "example")
	if bookID2 != bookID {
		log.Fatalf("[FAIL] book ID changed between rounds (%d → %d); a new book was created instead of updated", bookID, bookID2)
	}

	book2, err := client.GetBook(bookID2)
	mustOK(err, "get book after round 2")

	// Verify no duplicate chapters.
	chapterCounts := map[string]int{}
	chapterIDs2 := map[string]int{}
	for _, entry := range book2.Contents {
		if entry.Type == "chapter" {
			chapterCounts[entry.Name]++
			chapterIDs2[entry.Name] = entry.ID
		}
	}
	for _, name := range []string{"guide", "api"} {
		if chapterCounts[name] != 1 {
			log.Fatalf("[FAIL] expected exactly 1 chapter %q after round 2, got %d", name, chapterCounts[name])
		}
	}
	fmt.Println("[OK] No duplicate chapters created")

	// Verify no duplicate pages within each chapter.
	for _, chName := range []string{"guide", "api"} {
		chap, err := client.GetChapter(chapterIDs2[chName])
		mustOK(err, "get chapter "+chName+" after round 2")
		pageCounts := map[string]int{}
		for _, p := range chap.Pages {
			pageCounts[p.Name]++
		}
		for name, count := range pageCounts {
			if count > 1 {
				log.Fatalf("[FAIL] duplicate page %q in chapter %q after round 2 (count=%d)", name, chName, count)
			}
		}
		fmt.Printf("[OK] No duplicate pages in chapter %q\n", chName)
	}

	// Verify that page content was actually updated.
	// Find the root-level "README" page and check that its markdown contains
	// the "UPDATED:" marker we prepended in modifyMdFiles.
	var readmePageID int
	for _, entry := range book2.Contents {
		if entry.Type == "page" && entry.Name == "README" {
			readmePageID = entry.ID
			break
		}
	}
	if readmePageID == 0 {
		log.Fatal("[FAIL] README page not found after round 2")
	}
	readmePage, err := client.GetPage(readmePageID)
	mustOK(err, "get README page after round 2")
	if !strings.Contains(readmePage.Markdown, "UPDATED:") {
		log.Fatalf("[FAIL] README page content was not updated after round 2\ngot markdown: %s", readmePage.Markdown)
	}
	fmt.Println("[OK] README page content was updated in place")

	fmt.Println("\nIntegration test PASSED ✓")
}

// -----------------------------------------------------------------------
// Verification helpers
// -----------------------------------------------------------------------

// findBook returns the ID of the book named "example", or calls log.Fatal.
func findBook(client *bookstack.Client, name string) int {
	booksResp, err := client.ListBooks(nil)
	mustOK(err, "list books")
	for _, b := range booksResp.Data {
		if b.Name == name {
			return b.ID
		}
	}
	log.Fatalf(`[FAIL] book %q not found`, name)
	panic("unreachable")
}

// verifyBookStructure checks that the book contains the expected chapters and
// pages and prints a summary. It is called after both sync rounds.
func verifyBookStructure(client *bookstack.Client, bookID int, label string) {
	fmt.Printf("[OK] Found book \"example\" (ID=%d)\n", bookID)

	book, err := client.GetBook(bookID)
	mustOK(err, "get book ("+label+")")

	var rootPageNames []string
	chapterIDs := map[string]int{}
	for _, entry := range book.Contents {
		switch entry.Type {
		case "page":
			rootPageNames = append(rootPageNames, entry.Name)
		case "chapter":
			chapterIDs[entry.Name] = entry.ID
		}
	}

	if len(rootPageNames) != 1 || rootPageNames[0] != "README" {
		log.Fatalf("[FAIL] %s: expected 1 root page \"README\", got %v", label, rootPageNames)
	}
	fmt.Printf("[OK] %s root pages: %v\n", label, rootPageNames)

	for _, want := range []string{"guide", "api"} {
		if _, ok := chapterIDs[want]; !ok {
			log.Fatalf("[FAIL] %s: expected chapter %q — got chapters: %v", label, want, mapKeys(chapterIDs))
		}
	}
	fmt.Printf("[OK] %s chapters: guide, api\n", label)

	guideChap, err := client.GetChapter(chapterIDs["guide"])
	mustOK(err, "get chapter \"guide\" ("+label+")")
	guidePageNames := pageNames(guideChap.Pages)
	if !containsAll(guidePageNames, "intro", "advanced") {
		log.Fatalf("[FAIL] %s chapter \"guide\": expected [intro, advanced], got %v", label, guidePageNames)
	}
	fmt.Printf("[OK] %s chapter \"guide\" pages: %v\n", label, guidePageNames)

	apiChap, err := client.GetChapter(chapterIDs["api"])
	mustOK(err, "get chapter \"api\" ("+label+")")
	apiPageNames := pageNames(apiChap.Pages)
	if !containsAll(apiPageNames, "overview") {
		log.Fatalf("[FAIL] %s chapter \"api\": expected [overview], got %v", label, apiPageNames)
	}
	fmt.Printf("[OK] %s chapter \"api\" pages: %v\n", label, apiPageNames)
}

// -----------------------------------------------------------------------
// File manipulation helpers
// -----------------------------------------------------------------------

// copyDir recursively copies the directory tree at src to dst.
// Non-regular files (symlinks, devices, etc.) are skipped.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFile(path, target, info.Mode())
	})
}

// copyFile copies a single regular file from src to dst.
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}

	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// modifyMdFiles walks dir and prepends "UPDATED: " to the first Markdown
// heading (a line starting with "# ") in every .md file it finds.
func modifyMdFiles(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(string(data), "\n")
		modified := false
		for i, line := range lines {
			if strings.HasPrefix(line, "# ") {
				lines[i] = "# UPDATED: " + strings.TrimPrefix(line, "# ")
				modified = true
				break
			}
		}
		if !modified {
			return nil
		}
		return os.WriteFile(path, []byte(strings.Join(lines, "\n")), info.Mode())
	})
}

// -----------------------------------------------------------------------
// Shared helpers
// -----------------------------------------------------------------------

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required environment variable %q is not set", key)
	}
	return v
}

func mustOK(err error, op string) {
	if err != nil {
		log.Fatalf("[FAIL] %s: %v", op, err)
	}
}

func pageNames(pages []bookstack.ChapterPage) []string {
	names := make([]string, 0, len(pages))
	for _, p := range pages {
		names = append(names, p.Name)
	}
	return names
}

func mapKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func containsAll(haystack []string, needles ...string) bool {
	set := make(map[string]bool, len(haystack))
	for _, h := range haystack {
		set[h] = true
	}
	for _, n := range needles {
		if !set[n] {
			return false
		}
	}
	return true
}
