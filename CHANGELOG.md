# Changelog

## Unreleased

### Added

- Terminal pull request triage with GitHub CLI integration and a local SQLite cache.

### Changed

- Minimum Go version is 1.26, required by the updated `golang.org/x/net` dependency.
- Summary pane now contains only Files, Commits, and Comments; integrations use personal configuration rather than dedicated tabs.
- CLI approval runs directly without fetching the PR or diff for analysis.

### Removed

- Legacy `zappr review` command, automated review providers, risk classification, and cached recommendation displays. Manual GitHub review actions remain available.
- Inaccurate preview image.
