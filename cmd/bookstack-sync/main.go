package main

import (
	"log"
	"os"

	"github.com/alecthomas/kingpin/v2"
	"github.com/dimahum/bookstack-sync/internal/syncer"
)

func main() {
	app := kingpin.New("bookstack-sync", "Sync a local Markdown directory to BookStack.")
	app.HelpFlag.Short('h')

	url := app.Flag("url", "BookStack base URL (e.g. https://bookstack.example.com)").
		Required().String()
	tokenID := app.Flag("token-id", "BookStack API token ID").
		Required().String()
	tokenSecret := app.Flag("token-secret", "BookStack API token secret").
		Required().String()
	dir := app.Flag("dir", "Local directory to sync").
		Default(".").String()
	shelf := app.Flag("shelf", "Shelf name to add the book to (optional)").
		String()
	excludes := app.Flag("exclude", "File or directory name to ignore (may be repeated, e.g. --exclude AGENTS.md --exclude drafts)").
		Strings()

	kingpin.MustParse(app.Parse(os.Args[1:]))

	cfg := syncer.Config{
		URL:         *url,
		TokenID:     *tokenID,
		TokenSecret: *tokenSecret,
		ShelfName:   *shelf,
		Dir:         *dir,
		Excludes:    *excludes,
	}

	if err := syncer.Run(cfg); err != nil {
		log.Fatalf("sync failed: %v", err)
	}
}
