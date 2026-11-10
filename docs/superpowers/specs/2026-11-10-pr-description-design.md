# PR Description Pane

## Intent

PR descriptions often carry the most useful context for triage, but zappr did
not show them. The description should be available on demand without changing
the default diff-centred flow.

## Behaviour

- `d` (`toggle-description`, configurable) switches the large pane between its
  normal content (diff, commit diff, comment, or plan) and **Description** for
  the selected PR. Normal mode is the default.
- The toggle works from any pane. Focus changes keep the current mode, so the
  description can be scrolled and wrapped (`w`) after focusing the large pane.
- Selecting another PR, switching Summary tabs (`1`–`4`), or selecting another
  Summary item (arrows, Shift+Up/Down, mouse) returns the large pane to its
  normal content.
- Toggling back with `d` restores the diff scroll position.

## Decisions

- The description replaces Comments and Plan readers too while active.
- `toggle-diff` (`v`) affects only the diff; the description displays even with
  the diff disabled.
- Narrow layout (< 76 columns) has no large pane beside the Inbox and is
  unchanged.
- Empty description renders muted "no description".
- `model.PullRequest` gains `Body`; `body` is requested in both `gh pr list`
  and `gh pr view`, so the list refresh populates it and old cache entries need
  no migration.
- The body renders through the existing comment markdown renderer.

## History

A first iteration showed the description automatically whenever the Inbox
gained focus. It was replaced by the explicit toggle because the implicit
switching did not feel intuitive.

## Non-goals

No description editing.
