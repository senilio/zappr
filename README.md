# zappr

Terminal-first pull request inbox for fast, human-owned GitHub review triage.

zappr watches open pull requests, classifies review risk with deterministic rules, optionally runs shell-command AI triage, and lets you inspect, comment, skip, or approve from a keyboard-driven Bubble Tea UI.

## Principles

- Human approves; zappr only recommends.
- Deterministic risk rules outrank AI output.
- Terminal-only, local-first, no SaaS.
- Fast triage over deep autonomous review.
- `gh` CLI is the initial GitHub provider.

## Install

```sh
go install github.com/senilio/zappr/cmd/zappr@latest
```

For local development:

```sh
go run ./cmd/zappr --help
```

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
zappr approve owner/repo#123  # approve non-block PR via gh pr review
zappr config path             # print config path
zappr config init             # create default config
zappr cache status            # show local cache status
zappr cache clear             # clear cached PRs
zappr doctor                  # check gh, auth, cache
```

## TUI Keys

```text
j/k        navigate
enter      inspect selected PR
a          start approval; y confirms
c          comment on selected PR
s          skip PR (inbox); toggle side-by-side (diff)
r          refresh (inbox); re-run analysis (detail)
v          view diff
?          help (keys + risk legend)
esc        back
q          quit
```

The header shows `AI: on|off`. Triage panels separate the deterministic
`Rule-based verdict (authoritative)` from `AI (advisory - does not affect risk)`.

## Risk Model

Risk is conservative and deterministic:

- `safe`: low-risk changes, usually docs-only or generated/test-only.
- `review`: production code, medium size, missing tests, or uncertain data.
- `block`: failing CI, drafts, auth/security/billing/migrations/infra/API changes, lockfiles, or very large diffs.

Blocked PRs cannot be quick-approved from zappr.

### Trusted authors

Configure authors whose PRs are biased toward `safe`:

```yaml
trust:
  authors:
    - alice
    - bob
```

Trust only softens a `review` verdict to `safe` (and raises confidence on
already-safe PRs). It never overrides `block`; failing CI, drafts, high-risk
paths, and very large diffs still block quick approval.

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
  timeout: 1m0s
cache:
  path: /Users/you/.cache/zappr/zappr.db
trust:
  authors: []
```

`ZAPPR_AI_COMMAND` overrides `ai.command` for optional shell-command AI triage.

## Cache

zappr stores PR metadata, triage output, and local skip state in SQLite.

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
