package main

import (
	"flag"
	"log"
	"strings"

	"github.com/dimahum/bookstack-sync/internal/syncer"
)

func main() {
	url := flag.String("url", "", "BookStack base URL (e.g. https://bookstack.example.com)")
	tokenID := flag.String("token-id", "", "BookStack API token ID")
	tokenSecret := flag.String("token-secret", "", "BookStack API token secret")
	shelf := flag.String("shelf", "", "Shelf name to add the book to (optional)")
	dir := flag.String("dir", ".", "Local directory to sync")
	excludeStr := flag.String("exclude", "", "Comma-separated list of file/directory names to ignore (e.g. AGENTS.md,drafts)")

	flag.Parse()

	if *url == "" {
		log.Fatal("flag -url is required")
	}
	if *tokenID == "" {
		log.Fatal("flag -token-id is required")
	}
	if *tokenSecret == "" {
		log.Fatal("flag -token-secret is required")
	}

	var excludes []string
	if *excludeStr != "" {
		for _, e := range strings.Split(*excludeStr, ",") {
			if trimmed := strings.TrimSpace(e); trimmed != "" {
				excludes = append(excludes, trimmed)
			}
		}
	}

	cfg := syncer.Config{
		URL:         *url,
		TokenID:     *tokenID,
		TokenSecret: *tokenSecret,
		ShelfName:   *shelf,
		Dir:         *dir,
		Excludes:    excludes,
	}

	if err := syncer.Run(cfg); err != nil {
		log.Fatalf("sync failed: %v", err)
	}
}
