# Changelog

## Unreleased

## v0.5.0

### Added

- `:confirm-action` reviews a pending guarded action on the selected pull request.
- The Inbox header shows `refreshing` during a full refresh and the age of the last successful full refresh.

### Changed

- Selected-PR refreshes (`r`) and Atlantis `plan`, `apply`, `cancel`, and `unlock` comments queue per pull request and run in order on the original PR. Inbox rows show progress such as `refreshing`, `plan queued`, or `checking plan`.
- Guard warnings no longer interrupt work on other pull requests; the row shows `confirm plan` (or another command) until confirmed.
- zappr verifies that the target pull request is open before sending, and checks the head and guard again after confirmation.

## v0.4.0

### Added

- `C` copies a link for the focused pane: the pull request, selected file or commit, visible diff file, or Activity entry.
- The Activity tab shows who approved or requested changes and when, alongside comments. GitHub-confirmed stale approvals have a separate dismissal entry.
- Configure Activity display names with `author_aliases`.
- Watch the config file for changes and configure the reload action's bindings.
- Press `d` to toggle the pull request description in the large pane.

### Changed

- Renamed the Comments tab to Activity; its existing shortcut and action ID still work.

## v0.3.0

### Added

- Atlantis plan reader with project navigation and folding for supported plan reports.
- Manual configuration reload through `:reload`, with validation before applying settings.

### Changed

- Inbox items use two content rows, with compact Atlantis and review status pills in the metadata row.
- Comments use a shared list and Comment pane; reviewer details remain available in Comments rather than additional Inbox rows.
- Status pills use normal-weight text and consistent background intensity.

### Fixed

- Show `plan discarded` after a loaded Atlantis unlock confirmation instead of retaining obsolete plan, policy, and apply statuses.
- Distinguish partial Atlantis applies from applies covering every known project's current plan.
- Ignore stale polling and refresh responses after relevant configuration changes.

## v0.2.0

### Added

- Atlantis plan, policy, and apply status in the inbox, with refreshed status checks and explicit send-anyway confirmation for guarded plan/apply comments.
- CI and Terraform status pills, merge-state indicators, and grouped human reviewer details with separate advisory bot counts.
- Configurable pill shape and background intensity through `pills` and `pill_opacity`.

### Changed

- Atlantis details use compact outcome, project-count, and age labels, omitting secondary details in narrow panes.

### Fixed

- Preserve pill backgrounds and selected inbox styling behind modals, including ANSI colors and Unicode grapheme clipping.
- Keep reviewer details fresh after approval and request-changes actions, and distinguish reviewer states without color.

## v0.1.0

### Added

- Terminal pull request triage with GitHub CLI integration and a local SQLite cache.

### Changed

- Minimum Go version is 1.26, required by the updated `golang.org/x/net` dependency.
- Summary pane now contains only Files, Commits, and Comments; integrations use personal configuration rather than dedicated tabs.
- CLI approval runs directly without fetching the PR or diff for analysis.

### Removed

- Legacy `zappr review` command, automated review providers, risk classification, and cached recommendation displays. Manual GitHub review actions remain available.
- Inaccurate preview image.

## v0.0.5

### Changed

- Inbox and Summary Files/Commits selection scrolling now keeps up to two surrounding items visible, stopping naturally at the start and end of each list.
- Scrolling context accounts for wrapped items, directory headers, and small panes while preserving manual paging.
