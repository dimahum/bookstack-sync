# Using bookstack-sync

`bookstack-sync` is a command-line tool that mirrors a local directory of Markdown files into a [BookStack](https://www.bookstackapp.com/) instance through the BookStack API.

## Table of Contents

- [How it works](#how-it-works)
- [Installation](#installation)
  - [Pre-built binary via Go](#pre-built-binary-via-go)
  - [Docker](#docker)
- [Obtaining API credentials](#obtaining-api-credentials)
- [Flags](#flags)
- [Examples](#examples)
  - [Minimal sync](#minimal-sync)
  - [Sync to a shelf](#sync-to-a-shelf)
  - [Exclude files and directories](#exclude-files-and-directories)
  - [Run with Docker](#run-with-docker)
- [Directory structure conventions](#directory-structure-conventions)
- [Image attachments](#image-attachments)

---

## How it works

`bookstack-sync` walks a local directory tree and reflects its structure in BookStack:

| Local path | BookStack entity |
|---|---|
| Root folder | **Book** (named after the folder) |
| Root-level `.md` file | **Page** directly inside the book |
| Sub-directory containing `.md` files | **Chapter** inside the book |
| `.md` file inside a sub-directory | **Page** inside the chapter |

If a Markdown file references a local image (e.g. `![diagram](./images/arch.png)`), the image is uploaded as a BookStack file attachment and the reference is rewritten to the attachment URL automatically.

---

## Installation

### Pre-built binary via Go

```bash
go install github.com/dimahum/bookstack-sync/cmd/bookstack-sync@latest
```

This places the `bookstack-sync` binary in `$GOPATH/bin` (usually `~/go/bin`).

### Docker

A multi-platform image (linux/amd64, linux/arm64) is published to the GitHub Container Registry on every tagged release:

```bash
docker pull ghcr.io/dimahum/bookstack-sync:latest
```

Specific versions follow [semver](https://semver.org/):

```bash
docker pull ghcr.io/dimahum/bookstack-sync:0.1
docker pull ghcr.io/dimahum/bookstack-sync:0.1.2
```

---

## Obtaining API credentials

1. Log in to your BookStack instance.
2. Go to **Profile icon → Edit Profile**.
3. Scroll to the **API Tokens** section.
4. Click **Create Token**, give it a name, and save.
5. Copy the **Token ID** and **Token Secret** – the secret is only shown once.

Store these values as environment variables or CI/CD secrets rather than hard-coding them.

---

## Flags

| Flag | Required | Default | Description |
|---|---|---|---|
| `--url` | ✓ | — | BookStack base URL, e.g. `https://bookstack.example.com` |
| `--token-id` | ✓ | — | BookStack API token ID |
| `--token-secret` | ✓ | — | BookStack API token secret |
| `--dir` | | `.` | Local directory to sync |
| `--shelf` | | — | Name of an existing shelf to assign the new book to |
| `--exclude` | | — | File or directory name to skip (repeatable) |

---

## Examples

### Minimal sync

Sync the current directory to BookStack:

```bash
bookstack-sync \
  --url          https://bookstack.example.com \
  --token-id     MY_TOKEN_ID \
  --token-secret MY_TOKEN_SECRET
```

### Sync to a shelf

```bash
bookstack-sync \
  --url          https://bookstack.example.com \
  --token-id     MY_TOKEN_ID \
  --token-secret MY_TOKEN_SECRET \
  --dir          ./docs \
  --shelf        "Engineering"
```

### Exclude files and directories

Skip `AGENTS.md` and any directory named `drafts`:

```bash
bookstack-sync \
  --url          https://bookstack.example.com \
  --token-id     MY_TOKEN_ID \
  --token-secret MY_TOKEN_SECRET \
  --dir          ./docs \
  --exclude      AGENTS.md \
  --exclude      drafts
```

### Run with Docker

Mount your local docs directory into the container:

```bash
docker run --rm \
  -v "$(pwd)/docs:/docs" \
  ghcr.io/dimahum/bookstack-sync:latest \
    --url          https://bookstack.example.com \
    --token-id     MY_TOKEN_ID \
    --token-secret MY_TOKEN_SECRET \
    --dir          /docs \
    --shelf        "Engineering"
```

Pass credentials as environment variables to avoid them appearing in shell history:

```bash
docker run --rm \
  -e BOOKSTACK_URL \
  -e BOOKSTACK_TOKEN_ID \
  -e BOOKSTACK_TOKEN_SECRET \
  -v "$(pwd)/docs:/docs" \
  ghcr.io/dimahum/bookstack-sync:latest \
    --url          "$BOOKSTACK_URL" \
    --token-id     "$BOOKSTACK_TOKEN_ID" \
    --token-secret "$BOOKSTACK_TOKEN_SECRET" \
    --dir          /docs
```

---

## Directory structure conventions

Given the following local tree:

```
docs/
├── overview.md
├── architecture.md
├── setup/
│   ├── prerequisites.md
│   └── installation.md
└── api/
    ├── authentication.md
    └── endpoints.md
```

Running `bookstack-sync --dir ./docs` produces:

- **Book**: `docs`
  - **Page**: `overview`
  - **Page**: `architecture`
  - **Chapter**: `setup`
    - **Page**: `prerequisites`
    - **Page**: `installation`
  - **Chapter**: `api`
    - **Page**: `authentication`
    - **Page**: `endpoints`

---

## Image attachments

If a Markdown page contains a relative image reference such as:

```markdown
![System diagram](./diagrams/system.png)
```

`bookstack-sync` will:

1. Upload `./diagrams/system.png` as a BookStack **attachment** on the corresponding page.
2. Rewrite the Markdown reference to use the attachment's URL so the image renders correctly in BookStack.
