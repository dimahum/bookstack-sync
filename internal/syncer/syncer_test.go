package syncer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

