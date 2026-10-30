# zappr

[![CI](https://github.com/senilio/zappr/actions/workflows/ci.yml/badge.svg)](https://github.com/senilio/zappr/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27.3%2B-00ADD8?logo=go)](https://go.dev/)

Terminal-first pull request inbox for fast, human-owned GitHub review triage.

zappr watches open pull requests and lets you inspect files, commits, PR comments and reviews, diffs, comment, or approve from a keyboard-driven Bubble Tea UI.

## Principles

- Humans review and approve; GitHub review actions are explicit.
- Terminal-only, local-first, no SaaS.
- Fast triage, fully manual review.
- `gh` CLI is the initial GitHub provider.

## Install

### Prebuilt binaries

Download the Darwin or Linux archive and `checksums.txt` from the GitHub
Release, then verify the downloaded archive before extracting it:

```sh
shasum -a 256 -c checksums.txt --ignore-missing
tar -xzf zappr_VERSION_OS_ARCH.tar.gz
```

### Tagged source install

```sh
go install github.com/senilio/zappr/cmd/zappr@latest
```

This builds the latest tagged source version; it does not download a GitHub
Release binary.

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

## Requirements

- Go 1.27.3+
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
zappr approve owner/repo#123  # user-triggered approve via gh pr review
zappr config path             # print config path
zappr config init             # create default config
zappr cache status            # show local cache status
zappr cache clear             # clear cached PRs
zappr doctor                  # check gh, auth, cache
zappr version                 # print version (also --version)
```

Review PRs manually with `zappr watch`. The legacy `zappr review` command and
automated recommendations have been removed. Approvals, comments, and
request-changes remain explicit user actions.

## TUI Keys

These standard navigation, pane switching, selection, confirmation, help, and
quit controls are built in and cannot be configured:

```text
j/k          navigate inbox
enter        inspect selected PR
y            confirm
tab          cycle focus
g/G          top / bottom of list
shift-up/down previous / next file, commit, or comment
1/2/3        summary pane: files / commits / comments
f            filters / settings
?            help
:            command palette (vim-style colon commands)
esc          back / cancel
q            quit
```

The following PR/view actions use their default aliases and may be configured:
`a` approves after confirmation, `A` approves immediately, `c` opens the
comment editor, `r` refreshes the selected PR, `R` refreshes the inbox, `v`
toggles the diff pane (shown by default), `V` opens the full diff pager, `s` toggles side-by-side
diff, `w` toggles wrapping in the focused pane, `x` expands file content,
`n`/`p` and `[`/`]` move between files or
commits, `o` opens the PR, and `C` copies its URL. Command palette operations
below are also configurable where listed.

The summary pane starts on changed files and can switch to commits or PR
comments with `1`, `2`, and `3`. Files show the selected file diff in the
large pane, commits show the selected commit diff, and comments list each
comment on one row as timestamp · author, newest first, with the selected comment shown
in the large Comment pane. Comments include conversation comments, review
bodies, and inline review comments; the Comment pane shows their type and file
and line context. The Comment pane is available even when the diff is off.
Wrapping is off by default. Press `w` to toggle it independently in Inbox,
Summary, or Diff; each preference is saved across restarts. Inbox wrapping
expands every PR title while keeping metadata on one row. Diff wrapping keeps
line-number gutters and side-by-side columns intact. With wrapping off, long
lines are hard-truncated and diffs support horizontal scrolling. Horizontal
panning is disabled while wrapping is on. Keyboard shortcuts are available via `?`.

### Inbox status

Each inbox item shows its title, then `#number · author` with status pills
aligned to the right:

- `CI`: green when checks passed, yellow while running, red when failed, grey
  when unknown.
- Review icon: green approved, red changes requested, yellow review required,
  grey for no review decision or a draft.
- `TF`, in Atlantis repositories: the plan and policy state.
- Occasional pills: `draft`, `auto` (auto-merge enabled), `behind`, `conflict`,
  and `blocked`. `blocked` appears only when CI passed and the PR is approved
  but GitHub still blocks the merge. On narrow panes these drop first.

The selected PR adds a `reviews` row listing reviewers colored by state:
approvals green, requested changes red, pending requests yellow, comments
muted. Bots are muted, listed last, and never shown as an approval; a login is
treated as a bot when it ends in `[bot]`, `-bot`, or `-reviewer`. In Atlantis
repositories an `atlantis` row with a verdict pill follows.

Pills use Nerd Font rounded ends by default. Set `pills: square` to avoid Nerd
Font glyphs. Terminals without color show symbols instead: `CI✓`, `CI…`,
`CI✗`, `CI–`. Reviewer names use `✓` for approval, `✗` for changes requested,
and `●` for pending requests; comments, dismissed reviews, and bots have
explicit labels.

### Command palette

Press `:` to open the command palette. Type a command prefix to filter; press
`Enter` on an exact match to execute.

| Command | Action |
|---------|--------|
| `:draft` | Toggle PR draft status (draft ↔ ready for review) |
| `:automerge` | Toggle auto-merge; enabling prompts for confirmation because GitHub may merge immediately |
| `:close` | Close PR (prompts for confirmation) |
| `:update`| Update PR branch with base branch (prompts for confirmation) |
| `:block` | Request changes on PR to block merge (prompts for confirmation) |
| `:merge` | Merge active PR via squash and delete branch (prompts for confirmation) |
| `:refresh-all` | Refresh full inbox |
| `:colorscheme` | Change color scheme |

### Mouse

Click an Inbox item to select it. Click Summary or Diff pane to move focus to
that pane; item selection stays Inbox-only.

## Safety

- Local-only app; data stays in `gh` CLI, local cache, and terminal.
- GitHub actions use `gh` CLI only.
- Approval stays human-owned.
- Actions fire only from explicit key/command.

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
  interval: 30s
cache:
  path: /Users/you/.cache/zappr/zappr.db
pills: rounded
wrap:
  inbox: false
  summary: false
  diff: false
atlantis:
  enabled: true
```

`pills: rounded` (the default) needs a Nerd Font; `pills: square` avoids Nerd
Font glyphs.

### TUI actions

`zappr config init` creates `$XDG_CONFIG_HOME/zappr/config.yaml` (or
`~/.config/zappr/config.yaml`). The generated file includes only configurable
PR/view actions plus generic approval and comment actions. Fixed controls are
built in and cannot be configured. Add only the fields to change; each action
overlays its built-in `keys` and `commands` independently. An empty list clears
that field.

```yaml
actions:
  refresh-inbox: { keys: [R], commands: [refresh-all] }
  approve: { operation: approve, keys: [a], confirm: true }
  approve-now: { operation: approve, keys: [A], confirm: false }
  comment: { operation: comment, keys: [c] }
```

Configurable built-in IDs: `open-pr`, `copy-pr-url`, `refresh-pr`,
`refresh-inbox`, `toggle-diff`, `toggle-wrap`, `open-diff-pager`, `next-file`, `prev-file`,
`toggle-side-by-side`, `toggle-file-content`, `toggle-draft`, `toggle-automerge`, `close-pr`,
`update-branch`, `request-changes`, and `merge-pr`. Generic action
IDs include the default `approve`, `approve-now`, and `comment` entries.

Custom generic actions require `operation: approve` or `operation: comment`
and at least one `keys` or `commands` entry. `description` is optional and
replaces the generated help label. An `approve` action cannot set `comment` and
confirms by default. A `comment` action without `comment` opens the editor and
cannot set `confirm`; with a non-empty `comment`, it posts that static text and
confirms by default. Set `confirm: false` for immediate approval or static
comment actions.

Custom IDs use lowercase letters, digits, and hyphens. Removed named action IDs
`approve-with-confirmation`, `approve-immediately`, and `write-comment` fail
with a migration error: use `operation: approve` or `operation: comment`
instead. Saving rewrites the YAML file, so comments and formatting are not
preserved.

Keys conflict only within the same UI context. Command aliases are global,
case-insensitive, and must be unique. Commands use letters, digits, and
hyphens, and run from the palette as `:alias` (enter a prefix and select a
match if needed).

zappr rewrites the entire YAML file when saving theme or sort settings. YAML
comments, formatting, and field ordering are not preserved.

### Optional configuration recipes

These are personal configuration examples, not application defaults. Merge the
fields you want into your config; combine action entries under a single
`actions` mapping rather than adding duplicate mappings.

#### Refresh, drafts, and shortcuts

Refresh every minute, include draft PRs, and add `M` to merge and `U` to update
the branch. The built-in `:merge` and `:update` commands and their confirmations
remain unchanged. The custom `:review` command posts the literal comment
`review` after confirmation; it does not submit a GitHub review.

```yaml
refresh:
  interval: 1m
hide_drafts: false
actions:
  merge-pr: { keys: [M] }
  update-branch: { keys: [U] }
  review:
    operation: comment
    commands: [review]
    comment: review
```

#### Atlantis comment commands

For repositories that already use Atlantis, generic comment actions can post
its commands. The action IDs and aliases below are user-defined. Bot replies
remain available in the Comments summary (`3`).

When any open PR in the current inbox has `atlantis/*` commit statuses, the
inbox shows a `TF` pill on every item and the selected item adds an `atlantis`
row with a verdict read from those statuses. zappr does not parse comments for
this. The row is a single line and is never wrapped. Example:

```text
atlantis  PLAN OK  ready to apply · 12 projects · 6m ago
```

The verdict is the first that applies: `APPLY FAILED`, `PLAN FAILED` (naming
the failed project, or noting a failed policy check), `APPLYING`, `PLANNING`,
`PLAN OK`, or `NO PLAN`. For `PLAN OK` the note says `ready to apply`,
`applied`, or `nothing to apply`, followed by the project (or project count) and how long ago the
plan reported.

Comments whose first two words are `atlantis plan` or `atlantis apply` (case
insensitive) are guarded, whether they come from an action or from free text.
zappr refreshes the PR first, then decides:

- `atlantis plan` is held while any Atlantis status is running, or while a plan
  posted by zappr on the same PR and head commit within the last 2 minutes has
  not reported yet.
- `atlantis apply` is held when any Atlantis status is running, when a plan was
  just requested, when a plan or policy check failed, or when there is no plan
  for the latest commit. A missing plan is not held if apply already
  succeeded; the other reasons still are.

If the refresh fails, the dialog says zappr could not verify Atlantis status.
A held comment opens a dialog that lists the reasons and offers to send anyway
or cancel. When the guard passes, the action's normal confirmation (if any)
still applies; a blocked dialog replaces that confirmation, so you are asked
once.

Set `atlantis.enabled: false` to disable the `TF` pill, the `atlantis` row, and
the guards.
It defaults to `true`.

```yaml
actions:
  atlantis-plan:
    operation: comment
    keys: [P]
    commands: [ap]
    comment: atlantis plan
    confirm: false
  atlantis-apply:
    operation: comment
    commands: [aa]
    comment: atlantis apply
  atlantis-cancel:
    operation: comment
    commands: [ac]
    comment: atlantis cancel
  atlantis-unlock:
    operation: comment
    commands: [au]
    comment: atlantis unlock
```

In this example, `P` or `:ap` posts the plan command immediately because
`confirm: false` is explicit. Omit that field to require confirmation. Apply,
cancel, and unlock confirm by default; posting those comments may trigger
infrastructure changes or affect Atlantis state.

## Release

Tagged releases are configured to publish prebuilt Darwin and Linux archives,
`checksums.txt`, and SBOM documents to GitHub Releases, and generate GitHub
build-provenance attestations. Download the archive for your platform and
verify it against `checksums.txt` before extracting it.

Maintainers update `CHANGELOG.md`, create an annotated `vX.Y.Z` tag, and push
the tag to publish release artifacts:

```sh
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin vX.Y.Z
```

The release workflow runs the existing CI checks against the tagged commit
before publishing. Private vulnerability reporting must be enabled and
verified when making the repository public; see [SECURITY.md](SECURITY.md).

## Cache

zappr stores PR metadata in SQLite.

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
