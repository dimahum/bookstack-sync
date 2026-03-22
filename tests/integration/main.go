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
	"log"
	"os"
	"path/filepath"

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

	// Step 1: Create the "Example" shelf.
	shelf, err := client.CreateShelf(&bookstack.CreateShelfRequest{Name: "Example"})
	mustOK(err, "create shelf")
	fmt.Printf("[OK] Created shelf [%d]: %q\n", shelf.ID, shelf.Name)

	// Step 2: Run the sync.
	cfg := syncer.Config{
		URL:         bsURL,
		TokenID:     tokenID,
		TokenSecret: tokenSecret,
		Dir:         exampleDir,
		ShelfName:   "Example",
	}
	mustOK(syncer.Run(cfg), "syncer.Run")
	fmt.Println("[OK] Sync completed")

	// Step 3: Find the book named "example" (the base name of exampleDir).
	booksResp, err := client.ListBooks(nil)
	mustOK(err, "list books")

	var bookID int
	for _, b := range booksResp.Data {
		if b.Name == "example" {
			bookID = b.ID
			break
		}
	}
	if bookID == 0 {
		log.Fatal(`[FAIL] book "example" not found after sync`)
	}
	fmt.Printf("[OK] Found book \"example\" (ID=%d)\n", bookID)

	// Step 4: Verify the book's top-level contents.
	book, err := client.GetBook(bookID)
	mustOK(err, "get book")

	var rootPageNames []string
	chapterIDs := map[string]int{} // chapter name → chapter ID

	for _, entry := range book.Contents {
		switch entry.Type {
		case "page":
			rootPageNames = append(rootPageNames, entry.Name)
		case "chapter":
			chapterIDs[entry.Name] = entry.ID
		}
	}

	// Expect exactly one root-level page: "README".
	if len(rootPageNames) != 1 || rootPageNames[0] != "README" {
		log.Fatalf("[FAIL] expected 1 root page \"README\", got %v", rootPageNames)
	}
	fmt.Printf("[OK] Root pages: %v\n", rootPageNames)

	// Expect two chapters: "guide" and "api".
	for _, want := range []string{"guide", "api"} {
		if _, ok := chapterIDs[want]; !ok {
			log.Fatalf("[FAIL] expected chapter %q — got chapters: %v", want, mapKeys(chapterIDs))
		}
	}
	fmt.Println("[OK] Chapters: guide, api")

	// Step 5: Verify guide chapter pages: "intro" and "advanced".
	guideChap, err := client.GetChapter(chapterIDs["guide"])
	mustOK(err, "get chapter \"guide\"")

	guidePageNames := pageNames(guideChap.Pages)
	if !containsAll(guidePageNames, "intro", "advanced") {
		log.Fatalf("[FAIL] chapter \"guide\": expected pages [intro, advanced], got %v", guidePageNames)
	}
	fmt.Printf("[OK] chapter \"guide\" pages: %v\n", guidePageNames)

	// Step 6: Verify api chapter pages: "overview".
	apiChap, err := client.GetChapter(chapterIDs["api"])
	mustOK(err, "get chapter \"api\"")

	apiPageNames := pageNames(apiChap.Pages)
	if !containsAll(apiPageNames, "overview") {
		log.Fatalf("[FAIL] chapter \"api\": expected page [overview], got %v", apiPageNames)
	}
	fmt.Printf("[OK] chapter \"api\" pages: %v\n", apiPageNames)

	fmt.Println("\nIntegration test PASSED ✓")
}

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
