# Agent Instructions

## Project

zappr is a Go terminal app for pull request triage. It uses Bubble Tea for TUI, `gh` CLI for GitHub operations, deterministic risk classification, optional shell-command AI triage, and local SQLite cache.

## Core Rules

- Keep changes minimal and incremental.
- Preserve package boundaries under `internal/`.
- Do not add web UI, SaaS behavior, GitHub App auth, webhooks, analytics, or distributed services.
- Human approval stays mandatory; AI output is advisory only.
- Deterministic risk rules remain the source of truth for **classification** but do not gate approval. BLOCK risk is shown as a warning; the user decides whether to approve.
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
- `internal/app`: orchestration between GitHub, AI, cache, and risk.
- `internal/model`: shared PR and triage data structures.
- `internal/github`: provider interface and `gh` implementation.
- `internal/risk`: deterministic risk classifier.
- `internal/ai`: optional shell-command AI provider.
- `internal/cache`: memory and SQLite stores.
- `internal/config`: XDG config/cache paths and YAML config.
- `internal/tui`: Bubble Tea UI state, update, and view.

## Coding Guidance

- Keep interfaces small.
- Return errors from I/O boundaries.
- Avoid broad abstractions until a second implementation exists.
- Keep TUI state transitions explicit.
- Do not let AI triage change final risk by itself.
- Add tests for deterministic risk, config/cache behavior, and parser logic when changed.

## GitHub Actions

- Approval/comment actions must use `gh pr review`.
- Approval is permitted for all risk levels; BLOCK is advisory only.
- Network/auth/rate-limit failures should surface as user-readable errors.
