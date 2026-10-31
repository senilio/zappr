# Changelog

## Unreleased

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
