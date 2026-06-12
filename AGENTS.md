# Agent Instructions

## Project

zappr is a Go terminal app for pull request triage. It uses Bubble Tea for TUI, `gh` CLI for GitHub operations, on-demand shell-command AI triage, and local SQLite cache.

## Core Rules

- Keep changes minimal and incremental.
- Preserve package boundaries under `internal/`.
- Do not add web UI, SaaS behavior, GitHub App auth, webhooks, analytics, or distributed services.
- Human approval stays mandatory; AI output is advisory only.
- AI review must remain manual; do not auto-run it during refresh or selection changes.
- TUI panes must hard-truncate long lines; no soft wrapping in Inbox, Summary, or Diff panes. Diff uses horizontal scrolling.
- Prefer `gh` CLI integration until a concrete reason exists to replace it.

## Development Commands

```sh
go mod tidy
gofmt -w cmd internal
go test ./...
go run ./cmd/zappr --help
```

Use `rtk go test ./...` if available in the environment.

## Architecture

- `cmd/zappr`: binary entrypoint.
- `internal/cli`: command dispatch and process wiring.
- `internal/app`: orchestration between GitHub, AI, and cache.
- `internal/model`: shared PR and triage data structures.
- `internal/github`: provider interface and `gh` implementation.
- `internal/ai`: optional shell-command AI provider.
- `internal/cache`: memory and SQLite stores.
- `internal/config`: XDG config/cache paths and YAML config.
- `internal/risk`: deterministic PR risk classification with trusted-author bias.
- `internal/tui`: Bubble Tea UI state, update, view, and vim-style command palette.

## Coding Guidance

- Keep interfaces small.
- Return errors from I/O boundaries.
- Avoid broad abstractions until a second implementation exists.
- Keep TUI state transitions explicit.
- Add tests for AI triage flow, config/cache behavior, parser logic, and TUI layout/rendering when changed.

## GitHub Actions

- Approval/comment actions must use `gh pr review`.
- Approval is permitted whenever the user chooses; zappr does not auto-gate by risk.
- Network/auth/rate-limit failures should surface as user-readable errors.
