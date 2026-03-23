package syncer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bookstack "github.com/dimahum/bookstack-api"
)

func TestBuildExcludeSet(t *testing.T) {
	set := buildExcludeSet([]string{"AGENTS.md", "drafts/", "path/to/skip.md"})
	want := map[string]bool{
		"AGENTS.md": true,
		"drafts":    true,
		"skip.md":   true,
	}
	for k, v := range want {
		if got := set[k]; got != v {
			t.Errorf("set[%q] = %v, want %v", k, got, v)
		}
	}
	if len(set) != len(want) {
		t.Errorf("set has %d entries, want %d", len(set), len(want))
	}
}

func TestListMdFiles(t *testing.T) {
	dir := t.TempDir()

	// Create some files.
	writeFile(t, filepath.Join(dir, "page1.md"), "# Page 1")
	writeFile(t, filepath.Join(dir, "page2.md"), "# Page 2")
	writeFile(t, filepath.Join(dir, "notes.txt"), "plain text")
	writeFile(t, filepath.Join(dir, "AGENTS.md"), "should be excluded")

	// A subdirectory should be ignored.
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "sub", "inner.md"), "inner")

	excludes := buildExcludeSet([]string{"AGENTS.md"})
	got, err := listMdFiles(dir, excludes)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d files, want 2: %v", len(got), got)
	}
	for _, f := range got {
		base := filepath.Base(f)
		if base != "page1.md" && base != "page2.md" {
			t.Errorf("unexpected file in result: %s", f)
		}
	}
}

func TestProcessImages_externalURLsUnchanged(t *testing.T) {
	cfg := Config{URL: "https://bs.example.com", TokenID: "id", TokenSecret: "secret"}
	md := "![diagram](https://example.com/image.png)"
	result, err := processImages(cfg, md, "/some/file.md", 1)
	if err != nil {
		t.Fatal(err)
	}
	if result != md {
		t.Errorf("expected external URL unchanged\ngot:  %s\nwant: %s", result, md)
	}
}

func TestProcessImages_noImages(t *testing.T) {
	cfg := Config{URL: "https://bs.example.com", TokenID: "id", TokenSecret: "secret"}
	md := "# Hello\n\nNo images here."
	result, err := processImages(cfg, md, "/some/file.md", 1)
	if err != nil {
		t.Fatal(err)
	}
	if result != md {
		t.Errorf("expected content unchanged\ngot:  %s\nwant: %s", result, md)
	}
}

func TestProcessImages_missingLocalFile(t *testing.T) {
	cfg := Config{URL: "https://bs.example.com", TokenID: "id", TokenSecret: "secret"}
	md := "![missing](./does-not-exist.png)"
	_, err := processImages(cfg, md, "/some/file.md", 1)
	if err == nil {
		t.Error("expected error for missing local image file, got nil")
	}
}

// writeFile is a helper that creates a file with content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile(%q): %v", path, err)
	}
}

func TestBuildExcludeSet_empty(t *testing.T) {
	set := buildExcludeSet(nil)
	if len(set) != 0 {
		t.Errorf("expected empty set, got %v", set)
	}
}

func TestListMdFiles_emptyDir(t *testing.T) {
	dir := t.TempDir()
	got, err := listMdFiles(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected no files, got %v", got)
	}
}

func TestListMdFiles_excludesApplied(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "keep.md"), "# keep")
	writeFile(t, filepath.Join(dir, "skip.md"), "# skip")

	excludes := buildExcludeSet([]string{"skip.md"})
	got, err := listMdFiles(dir, excludes)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 file, got %d: %v", len(got), got)
	}
	if filepath.Base(got[0]) != "keep.md" {
		t.Errorf("unexpected file: %s", got[0])
	}
}

// newAttachmentServer starts an httptest server that simulates the
// BookStack POST /api/attachments endpoint and returns the given attachment ID.
func newAttachmentServer(t *testing.T, statusCode, id int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if statusCode < 400 {
			if err := json.NewEncoder(w).Encode(attachmentResponse{ID: id}); err != nil {
				t.Errorf("encoding response: %v", err)
			}
		}
	}))
}

func TestUploadFileAttachment_success(t *testing.T) {
	ts := newAttachmentServer(t, http.StatusOK, 42)
	defer ts.Close()

	dir := t.TempDir()
	imgPath := filepath.Join(dir, "image.png")
	writeFile(t, imgPath, "fake png bytes")

	cfg := Config{URL: ts.URL, TokenID: "tid", TokenSecret: "tsecret"}
	got, err := uploadFileAttachment(cfg, imgPath, 7)
	if err != nil {
		t.Fatal(err)
	}
	want := ts.URL + "/attachments/42"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUploadFileAttachment_serverError(t *testing.T) {
	ts := newAttachmentServer(t, http.StatusInternalServerError, 0)
	defer ts.Close()

	dir := t.TempDir()
	imgPath := filepath.Join(dir, "image.png")
	writeFile(t, imgPath, "fake png bytes")

	cfg := Config{URL: ts.URL, TokenID: "tid", TokenSecret: "tsecret"}
	_, err := uploadFileAttachment(cfg, imgPath, 7)
	if err == nil {
		t.Error("expected error for HTTP 500, got nil")
	}
}

func TestUploadFileAttachment_missingFile(t *testing.T) {
	cfg := Config{URL: "http://localhost", TokenID: "tid", TokenSecret: "tsecret"}
	_, err := uploadFileAttachment(cfg, "/nonexistent/path/image.png", 1)
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestProcessImages_localImage(t *testing.T) {
	ts := newAttachmentServer(t, http.StatusOK, 99)
	defer ts.Close()

	dir := t.TempDir()
	imgPath := filepath.Join(dir, "photo.png")
	writeFile(t, imgPath, "fake png bytes")
	mdPath := filepath.Join(dir, "page.md")

	cfg := Config{URL: ts.URL, TokenID: "tid", TokenSecret: "tsecret"}
	md := "![a photo](./photo.png)"
	result, err := processImages(cfg, md, mdPath, 1)
	if err != nil {
		t.Fatal(err)
	}

	want := ts.URL + "/attachments/99"
	if result == md {
		t.Error("expected markdown to be modified, but it was unchanged")
	}
	if !strings.Contains(result, want) {
		t.Errorf("expected result to contain %q\ngot: %s", want, result)
	}
}

// ---------------------------------------------------------------------------
// Helpers shared by newer tests
// ---------------------------------------------------------------------------

// writeJSON writes v as JSON to w and fails the test on error.
func writeJSON(t *testing.T, w http.ResponseWriter, v interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("writeJSON: %v", err)
	}
}

// newBSClient creates a bookstack.Client pointed at the given server URL.
func newBSClient(t *testing.T, serverURL string) *bookstack.Client {
	t.Helper()
	return bookstack.NewClient(serverURL, "tid", "tsecret")
}

// bsBook is the minimal book summary used when building fake list responses.
type bsBook struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ---------------------------------------------------------------------------
// findBookByName
// ---------------------------------------------------------------------------

// TestFindBookByName_found verifies that findBookByName returns the correct ID
// when the server returns a book whose name matches.
func TestFindBookByName_found(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/books" {
			http.NotFound(w, r)
			return
		}
		writeJSON(t, w, map[string]interface{}{
			"data":  []bsBook{{ID: 7, Name: "my-book"}, {ID: 8, Name: "other"}},
			"total": 2,
		})
	}))
	defer ts.Close()

	id, err := findBookByName(newBSClient(t, ts.URL), "my-book")
	if err != nil {
		t.Fatal(err)
	}
	if id != 7 {
		t.Errorf("got ID=%d, want 7", id)
	}
}

// TestFindBookByName_notFound verifies that findBookByName returns 0 when no
// book with the given name exists.
func TestFindBookByName_notFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]interface{}{
			"data":  []bsBook{{ID: 1, Name: "other-book"}},
			"total": 1,
		})
	}))
	defer ts.Close()

	id, err := findBookByName(newBSClient(t, ts.URL), "my-book")
	if err != nil {
		t.Fatal(err)
	}
	if id != 0 {
		t.Errorf("got ID=%d, want 0", id)
	}
}

// ---------------------------------------------------------------------------
// syncPage – create vs update path
// ---------------------------------------------------------------------------

// TestSyncPage_update verifies that syncPage issues a PUT when an existing
// page ID is provided (no images so no attachment upload is needed).
func TestSyncPage_update(t *testing.T) {
	const existingID = 55
	var sawPUT bool

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := fmt.Sprintf("/api/pages/%d", existingID)
		if r.Method == http.MethodPut && r.URL.Path == wantPath {
			sawPUT = true
			writeJSON(t, w, map[string]interface{}{"id": existingID, "name": "mypage"})
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}))
	defer ts.Close()

	dir := t.TempDir()
	mdPath := filepath.Join(dir, "mypage.md")
	writeFile(t, mdPath, "# My Page")

	cfg := Config{URL: ts.URL, TokenID: "tid", TokenSecret: "tsecret"}
	if err := syncPage(newBSClient(t, ts.URL), cfg, mdPath, 1, 0, existingID); err != nil {
		t.Fatal(err)
	}
	if !sawPUT {
		t.Error("expected a PUT request to update the existing page, but none was sent")
	}
}

// TestSyncPage_create verifies that syncPage issues a POST when no existing
// page ID is provided.
func TestSyncPage_create(t *testing.T) {
	var sawPOST bool

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/pages" {
			sawPOST = true
			writeJSON(t, w, map[string]interface{}{"id": 99, "name": "newpage"})
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}))
	defer ts.Close()

	dir := t.TempDir()
	mdPath := filepath.Join(dir, "newpage.md")
	writeFile(t, mdPath, "# New Page")

	cfg := Config{URL: ts.URL, TokenID: "tid", TokenSecret: "tsecret"}
	if err := syncPage(newBSClient(t, ts.URL), cfg, mdPath, 1, 0, 0); err != nil {
		t.Fatal(err)
	}
	if !sawPOST {
		t.Error("expected a POST request to create the page, but none was sent")
	}
}

// ---------------------------------------------------------------------------
// addBookToShelf idempotency
// ---------------------------------------------------------------------------

// TestAddBookToShelf_alreadyPresent verifies that addBookToShelf does NOT call
// UpdateShelf when the book is already on the shelf.
func TestAddBookToShelf_alreadyPresent(t *testing.T) {
	var sawPUT bool

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/shelves/1":
			writeJSON(t, w, map[string]interface{}{
				"id":    1,
				"name":  "s",
				"books": []map[string]interface{}{{"id": 42}},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/shelves/1":
			sawPUT = true
			writeJSON(t, w, map[string]interface{}{"id": 1, "name": "s"})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	if err := addBookToShelf(newBSClient(t, ts.URL), 1, 42); err != nil {
		t.Fatal(err)
	}
	if sawPUT {
		t.Error("expected no PUT when book is already on the shelf")
	}
}

// TestAddBookToShelf_notPresent verifies that addBookToShelf calls UpdateShelf
// to add the book when it is not yet on the shelf.
func TestAddBookToShelf_notPresent(t *testing.T) {
	var sawPUT bool

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/shelves/1":
			writeJSON(t, w, map[string]interface{}{
				"id":    1,
				"name":  "s",
				"books": []map[string]interface{}{{"id": 10}},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/shelves/1":
			sawPUT = true
			writeJSON(t, w, map[string]interface{}{"id": 1, "name": "s"})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	if err := addBookToShelf(newBSClient(t, ts.URL), 1, 42); err != nil {
		t.Fatal(err)
	}
	if !sawPUT {
		t.Error("expected a PUT request to add the book to the shelf")
	}
}

