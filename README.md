# zappr

[![CI](https://github.com/senilio/zappr/actions/workflows/ci.yml/badge.svg)](https://github.com/senilio/zappr/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go)](https://go.dev/)

Terminal-first pull request inbox for fast, human-owned GitHub review triage.

zappr watches open pull requests, optionally runs shell-command AI triage on demand, and lets you inspect files, commits, PR conversation comments, diffs, comment, or approve from a keyboard-driven Bubble Tea UI.

## Principles

- Human approves; zappr only recommends.
- AI review is manual and advisory.
- Terminal-only, local-first, no SaaS.
- Fast triage over deep autonomous review.
- `gh` CLI is the initial GitHub provider.

## Install

### Released binary

```sh
go install github.com/senilio/zappr/cmd/zappr@latest
```

After the first public tag, `@latest` resolves to the newest released version.

### Source checkout

```sh
go run ./cmd/zappr --help
```

Build with version metadata:

```sh
go build -ldflags "-X github.com/senilio/zappr/internal/cli.Version=$(git describe --tags --always --dirty)" ./cmd/zappr
```

### Future package manager

Homebrew support not available yet; track releases or build from source.

## Preview

![zappr terminal mockup](docs/demo.svg)

## Requirements

- Go 1.25+
- GitHub CLI: `gh`
- Authenticated GitHub CLI session: `gh auth login`

Run checks:

```sh
zappr doctor
```

## Commands

```sh
zappr watch owner/repo        # open keyboard inbox
zappr inbox owner/repo        # refresh and print inbox
zappr review owner/repo#123   # analyze one PR
zappr approve owner/repo#123  # user-triggered approve via gh pr review
zappr config path             # print config path
zappr config init             # create default config
zappr cache status            # show local cache status
zappr cache clear             # clear cached PRs
zappr doctor                  # check gh, auth, cache
zappr version                 # print version (also --version)
```

## TUI Keys

```text
j/k          navigate inbox
enter        inspect selected PR
a            start approval; y confirms
A            approve immediately (no confirmation)
c            comment on selected PR
r            refresh (inbox); re-run AI analysis (detail/diff)
v            toggle diff pane
V            open full diff in external pager (delta or less)
s            toggle side-by-side diff
tab          cycle focus
g/G          top / bottom of list
shift-up/down previous / next file or commit
x            expand selected file / diff hunks
1/2/3/4      summary pane: files / commits / comments / Atlantis plan
n/p, [/]     previous / next file or commit in summary/diff
f            filters / settings
o            open PR in browser
C            copy PR URL
?            help
:            command palette (vim-style colon commands)
esc          back / cancel
q            quit
```

The summary pane starts on changed files and can switch to commits, PR
conversation comments, or Atlantis plan output with `1`, `2`, `3`, and `4`.
Long lines are hard-truncated in all panes; diffs use horizontal scrolling
instead of wrapping. Keyboard shortcuts are always available via `?`.

### Command palette

Press `:` to open the command palette. Type a command prefix to filter; press
`Enter` on an exact match to execute.

| Command  | Action |
|----------|--------|
| `:ap`    | Comment `atlantis plan` on selected PR |
| `:aa`    | Comment `atlantis apply` on selected PR (prompts for confirmation) |
| `:au`    | Comment `atlantis unlock` on selected PR |
| `:draft` | Toggle PR draft status (draft ↔ ready for review) |
| `:close` | Close PR (prompts for confirmation) |
| `:update`| Update PR branch with base branch (prompts for confirmation) |
| `:block` | Request changes on PR to block merge (prompts for confirmation) |
| `:merge` | Merge active PR via squash and delete branch (prompts for confirmation) |
| `:colorscheme` | Change color scheme |

### Mouse

Click an Inbox item to select it. Click Summary or Diff pane to move focus to
that pane; item selection stays Inbox-only.

## AI Review

AI review is opt-in and never runs automatically. Press `r` on a selected PR to
run the configured shell command against the full PR metadata and diff.

Sample config:

```yaml
ai:
  command: "llm"
  prompt: "Review this pull request for correctness, risk, and missing tests. Return concise bullets."
```

- AI output is advisory only.
- Approval remains a human decision.
- When no AI result exists, the summary pane shows `press r to run AI review`.
- When AI is not configured, the summary pane shows `AI review not configured`.
- AI results are persisted in SQLite; a subsequent `r` re-runs analysis.

## Safety

- Local-only app; data stays in `gh` CLI, local cache, and terminal.
- GitHub actions use `gh` CLI only.
- AI is opt-in, manual, advisory.
- Approval stays human-owned.
- Actions fire only from explicit key/command.

### Trusted authors

Authors listed under `trust.authors` are treated as trusted. For trusted-author
PRs the deterministic classifier softens a `Review` result to `Safe` and raises
confidence on already-`Safe` results. A `Block` result is never overridden.

```yaml
trust:
  authors:
    - alice
    - bob
```

## Config

Default path:

```text
$XDG_CONFIG_HOME/zappr/config.yaml
```

Fallback:

```text
~/.config/zappr/config.yaml
```

Example:

```yaml
refresh:
  interval: 2m0s
ai:
  command: ""
  prompt: ""
  timeout: 1m0s
cache:
  path: /Users/you/.cache/zappr/zappr.db
trust:
  authors: []
```

Environment overrides:

| Variable           | Config field  | Description                       |
|--------------------|---------------|-----------------------------------|
| `ZAPPR_AI_COMMAND` | `ai.command`  | Shell command for AI triage       |
| `ZAPPR_AI_PROMPT`  | `ai.prompt`   | Prompt passed to the AI command   |

## Release

- `go install github.com/senilio/zappr/cmd/zappr@latest` installs tagged release binary.
- Source install with `go run ./cmd/zappr` works from checkout.
- Release builds should set `internal/cli.Version` with `-ldflags`.

## Cache

zappr stores PR metadata and AI triage output in SQLite.

Default path:

```text
$XDG_CACHE_HOME/zappr/zappr.db
```

Fallback:

```text
~/.cache/zappr/zappr.db
```

## Development

```sh
go mod tidy
gofmt -w cmd internal
go test ./...
```

## Non-Goals

- Autonomous approval
- Web dashboard
- GitHub App auth
- Webhooks
- Team management
- Analytics
- Distributed services
