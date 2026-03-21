# bookstack-sync

Go tool to sync a local directory of Markdown files to [BookStack](https://www.bookstackapp.com/).

## Features

- Reads a directory tree and finds all `.md` files.
- Creates a **book** in BookStack named after the root folder.
- Root-level `.md` files become **pages** directly in the book.
- Sub-directories that contain `.md` files become **chapters**; their `.md` files become pages inside those chapters.
- If a Markdown file references a local image (`![alt](./path/to/image.png)`), the image is uploaded as a BookStack file attachment and the reference is replaced with a link to the attachment.
- Optionally assigns the new book to an existing **shelf** by name.
- Configurable **exclude list** to skip specific files or directories (e.g. `AGENTS.md`).

## Installation

```
go install github.com/dimahum/bookstack-sync/cmd/bookstack-sync@latest
```

## Usage

```
bookstack-sync \
  -url         https://bookstack.example.com \
  -token-id    <API token ID> \
  -token-secret <API token secret> \
  -dir         ./docs \
  -shelf       "Engineering" \
  -exclude     "AGENTS.md,drafts"
```

### Flags

| Flag | Required | Description |
|------|----------|-------------|
| `-url` | ✓ | BookStack base URL |
| `-token-id` | ✓ | BookStack API token ID |
| `-token-secret` | ✓ | BookStack API token secret |
| `-dir` | | Local directory to sync (default: `.`) |
| `-shelf` | | Shelf name to add the book to |
| `-exclude` | | Comma-separated list of file/directory names to skip |

### Obtaining API credentials

In your BookStack instance go to **Profile → API Tokens** and create a new token.

## Project layout

```
bookstack-sync/
├── cmd/
│   └── bookstack-sync/   # CLI entry point
│       └── main.go
└── internal/
    └── syncer/           # Core sync logic
        ├── syncer.go
        └── syncer_test.go
```
