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
4            summary pane: plan (only when a supported report is found)
f            filters / settings
?            help
:            command palette (vim-style colon commands)
esc          back / cancel
q            quit
```

The following PR/view actions use their default aliases and may be configured:
`a` approves after confirmation, `A` approves immediately, `c` opens the
comment editor, `r` refreshes the selected PR, `R` refreshes the inbox, `v`
toggles the diff pane (shown by default), `d` toggles the PR description in the large pane, `V` opens the full diff pager, `s` toggles side-by-side
diff, `w` toggles wrapping in the focused pane, `x` expands file content,
`n`/`p` and `[`/`]` move between files or
commits, `o` opens the PR, and `C` copies its URL. The [Plan reader](#plan-reader)
uses the basic navigation keys. Command palette
operations below are also configurable where listed.

The summary pane starts on changed files and can switch to commits, PR
comments, or an Atlantis plan with `1`, `2`, `3`, and `4`. Every available
destination is named in the Summary title. Files show the selected file diff in
the large pane; ordinary modified labels and zero counts are omitted so added,
removed, and renamed files stand out. Commits show the selected commit diff.
Comments are listed newest first, one row per comment with timestamp, author,
and kind. The selected row is bold; other rows are muted. There is no marker,
indentation, or body preview. Comments include conversation comments, review
bodies, and inline review comments. The selected comment appears in the large
Comment pane, with author, kind, time, and file location in its content.
The Comment pane is available even when the diff is off.
Wrapping is off by default. Press `w` to toggle it independently in Inbox,
Summary, or Diff; each preference is saved across restarts. Inbox wrapping
expands every PR title while keeping metadata on one row. Diff wrapping keeps
line-number gutters and side-by-side columns intact. With wrapping off, long
lines are hard-truncated and diffs support horizontal scrolling. Horizontal
panning is disabled while wrapping is on. A focused Inbox takes about two
thirds of the left column. When Summary or Diff has focus, the Inbox shrinks to
fit its content, keeping at least eight rows and at most a third of the column.
Keyboard shortcuts are available via `?`.

### Inbox status

Each inbox item has two rows: its title, then `#number by author` with status
pills aligned to the right. On the selected PR, the title is bold and the
number and author use plain text:

- `ci`: green when checks passed, yellow while running, red when failed, grey
  when unknown.
- Review pill: green `✓` approved, red `✗` changes requested, yellow `…` review
  required, grey `–` for no review decision or a draft. Each symbol sits in a
  compact three-cell pill using the same shape and opacity as CI. The symbols
  themselves do not require a Nerd Font; rounded pill ends do.
- Occasional pills: `draft`, `auto` (auto-merge enabled), `behind`, `conflict`,
  and `blocked`. `blocked` appears only when CI passed and the PR is approved
  but GitHub still blocks the merge. On narrow panes these drop first.

Atlantis status appears as an additional metadata pill; see
[Plan reader](#plan-reader). Reviewer names are not shown in the Inbox; review
details and bot reports remain available in Comments.

Pills use Nerd Font rounded ends by default. Set `pills: square` to avoid Nerd
Font glyphs. Terminals without color show symbols instead: `ci✓`, `ci…`,
`ci✗`, `ci–`. Colored pills use normal-weight status-colored text over a tinted
background; `pill_opacity` changes the background without fading the text.
The ANSI theme uses foreground colors only.

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
| `:reload` | Reload configuration from disk |

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
pill_opacity: 0.7
wrap:
  inbox: false
  summary: false
  diff: false
atlantis:
  enabled: true
```

`pills: rounded` (the default) needs a Nerd Font; `pills: square` avoids Nerd
Font glyphs. `pill_opacity` controls background intensity from `0` (panel
background) to `1` (original intensity), defaulting to `0.7`. Labels keep their
brightness. This blends colors rather than using terminal transparency and has
no effect in no-color mode or the `ansi` palette.

Config edits are checked once per second and applied without restarting,
independently of PR refresh. Use `:reload` to reload immediately. Invalid,
unreadable, or missing configuration leaves the last working settings active
and shows an error once per distinct failed change; valid edits recover
automatically. Reload never recreates a missing file. Changes to `cache.path`
require a restart; the current database stays open.

### TUI actions

`zappr config init` creates `$XDG_CONFIG_HOME/zappr/config.yaml` (or
`~/.config/zappr/config.yaml`). The generated file includes only configurable
PR/view actions, reload, and generic approval and comment actions. Fixed controls are
built in and cannot be configured. Add only the fields to change; each action
overlays its built-in `keys` and `commands` independently. An empty list clears
that field.

```yaml
actions:
  reload: { keys: [ctrl+r] }
  refresh-inbox: { keys: [R], commands: [refresh-all] }
  approve: { operation: approve, keys: [a], confirm: true }
  approve-now: { operation: approve, keys: [A], confirm: false }
  comment: { operation: comment, keys: [c] }
```

Configurable built-in IDs: `open-pr`, `copy-pr-url`, `refresh-pr`,
`refresh-inbox`, `reload`, `toggle-diff`, `toggle-description`, `toggle-wrap`, `open-diff-pager`, `next-file`, `prev-file`,
`toggle-side-by-side`, `toggle-file-content`, `toggle-draft`, `toggle-automerge`, `close-pr`,
`update-branch`, `request-changes`, and `merge-pr`. Generic action
IDs include the default `approve`, `approve-now`, and `comment` entries.

`reload` has no default key and retains `:reload` when only `keys` is changed.
It is a built-in action, not a generic `operation: reload`. Command-modified
shortcuts work only if the terminal forwards the corresponding key events;
zappr does not change the terminal's shortcut configuration or keyboard protocol.

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
remain available in the Comments summary (`3`). Key `4` is reserved for the
Plan tab; binding a custom action to it is reported as a key collision.

### Plan reader

When the selected PR has a supported Atlantis plan report in its conversation
comments, the Summary title adds `[4]` (`[4 plan]` when active). Press `4` to open it; zappr never
switches to it on its own. The Plan tab starts at the overall summary, then
lists projects with changes, failures, unknown results, and no changes. The
large pane shows the original plan text for the selected project or resource,
not the PR's source diff, and works with the diff off.

Supported reports use the standard Atlantis comment layout: a `Ran Plan for
project:` or `Ran Plan for N projects:` header, output in a collapsible
`diff` block, and `Continued plan output from previous comment.` for reports
split across several comments. Split comments are joined only when they are
adjacent and explicitly marked as continuations; otherwise the report is shown
as incomplete. Unrecognized comments stay available in Comments.

Each report states whether its source is complete. Counts come from the report
itself, not checks. Multi-project reports support numbered or unnumbered
`### project:` sections. When the output sections account for all changed and
failed projects and the complete summary counts reconcile, projects without
output are marked as having no changes. Otherwise their outcomes remain unknown.
Successful checks or missing apply statuses are never treated as "no changes".

Use the basic navigation keys: arrows to select an item, `g`/`gg` and `G`
for the first/last Plan item, and Page Up/Page Down to move by a page.
Projects list their resources directly, and changed values are shown in full.
Source rows show the original comment; select a report row and press Enter
to switch reports. The right-pane header is simply `Plan`; evidence quality
and source warnings appear in the content.

The former reader shortcuts and `:plan-*` commands have been removed.
Their entries in older generated configs are ignored without rewriting the
file, so their keys can be reused for other actions. Comments use ordinary
navigation without reader search or author filters.

The Inbox Atlantis pill shows the plan result or check progress, for example:

```text
[plan ok]
```

Without a loaded report, checks can still establish `plan ok`, `plan failed`,
or `planning`, but not whether there are changes. The Inbox omits `changes`,
`no changes`, and `incomplete` pills; full report details remain in the Plan pane.
Partial applies show `partly applied`; `applied` requires a successful apply at
least as recent as each known project's plan. The pills
omit commit verification, project counts, and timestamps. PRs with neither
Atlantis checks nor a loaded report do not show these pills. Optional pills
drop when the metadata row is too narrow.

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

Set `atlantis.enabled: false` to disable the Plan tab, Atlantis pills, and
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

## Deferred work

### JSON plan input

JSON plan parsing is deferred because producing and publishing JSON artifacts
would require changes to the live Atlantis server. The existing artifact
workflow produces plain-text `tofu show -no-color` output. Further plan-reader
work should use existing output without requiring changes to Atlantis.

If revisited, JSON input should be optional and storage-provider-neutral, with
comments and text still usable without AWS access or organization-specific
conventions. Artifacts need reliable association with the PR commit, project,
workspace, and plan run. Raw plan JSON can expose sensitive values; storage
access, retention, logging, and display masking need review before adoption.

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
