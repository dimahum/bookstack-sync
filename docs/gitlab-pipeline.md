# Using bookstack-sync in a GitLab CI/CD Pipeline

This guide shows how to use the `ghcr.io/dimahum/bookstack-sync` Docker image inside a GitLab CI/CD pipeline to automatically sync a directory of Markdown files to [BookStack](https://www.bookstackapp.com/) on every push.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Setting up CI/CD variables](#setting-up-cicd-variables)
- [Minimal pipeline example](#minimal-pipeline-example)
- [Sync only on changes to docs](#sync-only-on-changes-to-docs)
- [Sync to a shelf and exclude files](#sync-to-a-shelf-and-exclude-files)
- [Use a pinned image version](#use-a-pinned-image-version)
- [Run on a self-hosted runner](#run-on-a-self-hosted-runner)
- [Tips and best practices](#tips-and-best-practices)

---

## Prerequisites

- A running [BookStack](https://www.bookstackapp.com/) instance reachable from GitLab CI runners.
- A BookStack API token (see [Obtaining API credentials](usage.md#obtaining-api-credentials)).
- A GitLab project with a `.gitlab-ci.yml` file.

---

## Setting up CI/CD variables

Store your BookStack credentials as **masked, protected** CI/CD variables so they are never exposed in job logs.

1. Go to your GitLab project → **Settings → CI/CD → Variables**.
2. Add the following variables:

| Variable | Description |
|---|---|
| `BOOKSTACK_URL` | Full URL of your BookStack instance, e.g. `https://bookstack.example.com` |
| `BOOKSTACK_TOKEN_ID` | BookStack API token ID |
| `BOOKSTACK_TOKEN_SECRET` | BookStack API token secret (mark as **Masked**) |

---

## Minimal pipeline example

The simplest `.gitlab-ci.yml` that syncs the `docs/` folder to BookStack:

```yaml
# .gitlab-ci.yml

stages:
  - publish

bookstack-sync:
  stage: publish
  image: ghcr.io/dimahum/bookstack-sync:latest
  script:
    - |
      bookstack-sync \
        --url          "$BOOKSTACK_URL" \
        --token-id     "$BOOKSTACK_TOKEN_ID" \
        --token-secret "$BOOKSTACK_TOKEN_SECRET" \
        --dir          docs
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
```

This job runs only on commits to the default branch (e.g. `main`).

---

## Sync only on changes to docs

Use `changes` rules to skip the job when no documentation was modified:

```yaml
# .gitlab-ci.yml

stages:
  - publish

bookstack-sync:
  stage: publish
  image: ghcr.io/dimahum/bookstack-sync:latest
  script:
    - |
      bookstack-sync \
        --url          "$BOOKSTACK_URL" \
        --token-id     "$BOOKSTACK_TOKEN_ID" \
        --token-secret "$BOOKSTACK_TOKEN_SECRET" \
        --dir          docs
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
      changes:
        - docs/**/*
```

---

## Sync to a shelf and exclude files

Add `--shelf` to assign the book to an existing BookStack shelf, and `--exclude` to skip files or directories you don't want synced:

```yaml
# .gitlab-ci.yml

stages:
  - publish

bookstack-sync:
  stage: publish
  image: ghcr.io/dimahum/bookstack-sync:latest
  variables:
    BOOKSTACK_SHELF: "Engineering"
  script:
    - |
      bookstack-sync \
        --url          "$BOOKSTACK_URL" \
        --token-id     "$BOOKSTACK_TOKEN_ID" \
        --token-secret "$BOOKSTACK_TOKEN_SECRET" \
        --dir          docs \
        --shelf        "$BOOKSTACK_SHELF" \
        --exclude      AGENTS.md \
        --exclude      drafts
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
      changes:
        - docs/**/*
```

---

## Use a pinned image version

Using `latest` is convenient but can lead to unexpected behaviour if the image is updated. Pin to a specific release tag for reproducible pipelines:

```yaml
bookstack-sync:
  stage: publish
  image: ghcr.io/dimahum/bookstack-sync:0.0.1   # pin to a release tag
  script:
    - |
      bookstack-sync \
        --url          "$BOOKSTACK_URL" \
        --token-id     "$BOOKSTACK_TOKEN_ID" \
        --token-secret "$BOOKSTACK_TOKEN_SECRET" \
        --dir          docs
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
```

All available tags follow [semver](https://semver.org/) (`<major>.<minor>.<patch>`, `<major>.<minor>`, and `latest`). Check the [container registry](https://github.com/dimahum/bookstack-sync/pkgs/container/bookstack-sync) for the current list of published tags.

---

## Run on a self-hosted runner

If your BookStack instance is only accessible within your private network, make sure the job runs on a runner that has network access to it. Use the `tags` keyword to target the right runner:

```yaml
bookstack-sync:
  stage: publish
  image: ghcr.io/dimahum/bookstack-sync:latest
  tags:
    - internal          # runner tag that has access to BookStack
  script:
    - |
      bookstack-sync \
        --url          "$BOOKSTACK_URL" \
        --token-id     "$BOOKSTACK_TOKEN_ID" \
        --token-secret "$BOOKSTACK_TOKEN_SECRET" \
        --dir          docs
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
```

The runner must have Docker executor enabled to use the `image:` keyword.

---

## Tips and best practices

| Tip | Details |
|---|---|
| **Mask the token secret** | In GitLab variable settings, enable **Mask variable** for `BOOKSTACK_TOKEN_SECRET` so it never appears in job logs. |
| **Protect variables** | Enable **Protect variable** to restrict credentials to protected branches only. |
| **Pin image versions** | Use a specific semver tag instead of `latest` to keep pipelines deterministic. |
| **Limit trigger paths** | Use `changes:` rules to avoid unnecessary API calls when only non-docs files change. |
| **Cache nothing** | `bookstack-sync` is a stateless binary; no caching is needed for the tool itself. |
| **Review logs** | The tool logs each created/updated page to stdout, which appears in the GitLab job log for easy auditing. |
