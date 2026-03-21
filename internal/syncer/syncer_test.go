package syncer

import (
	"os"
	"path/filepath"
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
