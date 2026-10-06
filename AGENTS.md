# Agent Instructions

## Project

zappr is a Go terminal app for pull request triage. It uses Bubble Tea for TUI, `gh` CLI for GitHub operations, and local SQLite cache.

## Core Rules

- Keep changes minimal and incremental.
- Preserve package boundaries under `internal/`.
- Do not add web UI, SaaS behavior, GitHub App auth, webhooks, analytics, or distributed services.
- Human approval stays mandatory.
- TUI panes must hard-truncate long lines; no soft wrapping in Inbox, Summary, or Diff panes. Diff uses horizontal scrolling.
- Prefer `gh` CLI integration until a concrete reason exists to replace it.

## Git Workflow

- Work on a normal branch in the main checkout; do not use git worktrees.
- When the work is done and validated, merge the branch into local `main` and delete the branch.

## Development Commands

```sh
go mod tidy
gofmt -w cmd internal
go test ./...
go run ./cmd/zappr --help
```

Run CI-equivalent validation:

```sh
test -z "$(gofmt -l .)"
go mod tidy -diff
go mod verify
go vet ./...
go test -race ./...
go build -trimpath -ldflags "-s -w -X github.com/senilio/zappr/internal/cli.Version=ci" -o /tmp/zappr ./cmd/zappr
/tmp/zappr version
```

Use `rtk go test ./...` if available in the environment.

## Architecture

- `cmd/zappr`: binary entrypoint.
- `internal/cli`: command dispatch and process wiring.
- `internal/app`: orchestration between GitHub and cache.
- `internal/model`: shared PR and triage data structures.
- `internal/github`: provider interface and `gh` implementation.
- `internal/cache`: memory and SQLite stores.
- `internal/config`: XDG config/cache paths and YAML config.
- `internal/risk`: deterministic PR risk classification.
- `internal/tui`: Bubble Tea UI state, update, view, and vim-style command palette.

## Coding Guidance

- Keep interfaces small.
- Return errors from I/O boundaries.
- Avoid broad abstractions until a second implementation exists.
- Keep TUI state transitions explicit.
- Add tests for config/cache behavior, parser logic, and TUI layout/rendering when changed.

## GitHub Actions

- Approvals and request changes use `gh pr review`; comments use `gh pr comment`.
- Approval is permitted whenever the user chooses; zappr does not auto-gate by risk.
- Network/auth/rate-limit failures should surface as user-readable errors.
- `ToggleDraft`: uses `gh pr edit --draft` (to draft) or `gh pr ready` (to undraft).
- `ClosePR`: uses `gh pr close`; optionally posts a comment first.
- `UpdateBranch`: uses `gh pr update-branch`.
- `RequestChanges`: uses `gh pr review --request-changes`.
- `MergePR`: uses `gh pr merge --squash --delete-branch`.

## TUI Layout

- No footer. Keyboard shortcuts are shown only in the `?` help screen.
- `chromeLines = 1` (header only); `footerLines = 0` in `resizeViewports`.
- Inbox items are separated by a thin muted `─` divider line (3-line stride: 2 content + 1 divider).
- All stride math (4 sites: `inboxPanel`, `inboxIndexAt`, `selectedInboxPreserveRanges`, `selectedInboxYOffset`) must stay in sync if item line count changes.
- Inbox items are always clickable regardless of current selection state.
- Clicking the Summary or Diff pane only changes focus to that pane; item selection stays Inbox-only.
