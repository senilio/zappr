# PR Description Pane

## Intent

PR descriptions often carry the most useful context for triage, but zappr does
not show them. The description should be visible without adding keys, tabs, or
steps to the existing triage flow.

## Behaviour

- When the Inbox gains focus (Tab/Shift+Tab, Esc back to inbox, mouse click,
  and at startup) the right pane shows **Description** for the selected PR.
- Moving up/down in the Inbox keeps the current right-pane content. When it is
  the description, it follows the newly selected PR.
- Shift+Up/Down (`next-item`/`prev-item`) while the Inbox is focused performs
  its existing action and switches the right pane to that content (diff,
  commit diff, comment, or plan). It stays so until the Inbox gains focus again.
- When the Summary pane gains focus the right pane shows its normal content.
- Focusing the right pane itself keeps the description, so it can be scrolled
  (j/k, page keys, g/G, mouse wheel) and wrapped with `w`.

## Decisions

- The description replaces Comments and Plan readers too while active.
- `toggle-diff` (`v`) affects only the diff; the description displays even with
  the diff disabled.
- Narrow layout (< 76 columns) has no right pane in inbox view and is unchanged.
- Empty description renders muted "no description".
- `model.PullRequest` gains `Body`; `body` is requested in both `gh pr list`
  and `gh pr view`, so the list refresh populates it and old cache entries need
  no migration.
- The body renders through the existing comment markdown renderer.

## Approaches considered

- **Focus-driven Description (chosen):** no new keys; follows the user's
  natural flow between Inbox and Summary.
- **Pinned entry in Comments tab:** reuses the reader, but mixes the
  description with comments and requires a tab switch.
- **Dedicated Description tab or popup:** adds a key or an open/close step,
  and a popup covers the diff.

## Non-goals

No new key bindings, config options, or description editing.
