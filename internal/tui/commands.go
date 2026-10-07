package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/senilio/zappr/internal/config"
)

type actionContext string

const (
	actionContextNormal              actionContext = "normal"
	actionContextHelp                actionContext = "help"
	actionContextDiff                actionContext = "diff"
	actionContextApprove             actionContext = "approve"
	actionContextSettings            actionContext = "settings"
	actionContextColorscheme         actionContext = "colorscheme"
	actionContextComment             actionContext = "comment"
	actionContextCommand             actionContext = "command"
	actionContextCommandConfirmation actionContext = "command-confirmation"
)

type confirmationPolicy int

const (
	confirmationNone confirmationPolicy = iota
	confirmationOptional
	confirmationMandatory
)

type tuiAction struct {
	ID           string
	Label        string
	Context      actionContext
	Keys         []string
	Commands     []string
	RequiresPR   bool
	Confirmation confirmationPolicy
	Operation    string
	Comment      string
}

type actionRegistry struct {
	actions  []tuiAction
	keys     map[actionContext]map[string]tuiAction
	commands map[string]tuiAction
}

var actionDescriptors = []tuiAction{
	{"quit", "quit", actionContextNormal, nil, nil, false, confirmationNone, "quit", ""},
	{"help", "show help", actionContextNormal, nil, nil, false, confirmationNone, "help", ""},
	{"summary-files", "show files", actionContextNormal, nil, nil, true, confirmationNone, "summary-files", ""},
	{"summary-commits", "show commits", actionContextNormal, nil, nil, true, confirmationNone, "summary-commits", ""},
	{"summary-comments", "show comments", actionContextNormal, nil, nil, true, confirmationNone, "summary-comments", ""},
	{"summary-atlantis", "show Atlantis", actionContextNormal, nil, nil, true, confirmationNone, "summary-atlantis", ""},
	{"focus-next", "focus next pane", actionContextNormal, nil, nil, false, confirmationNone, "focus-next", ""},
	{"focus-prev", "focus previous pane", actionContextNormal, nil, nil, false, confirmationNone, "focus-prev", ""},
	{"go-top", "go to top", actionContextNormal, nil, nil, false, confirmationNone, "go-top", ""},
	{"go-bottom", "go to bottom", actionContextNormal, nil, nil, false, confirmationNone, "go-bottom", ""},
	{"open-pr", "open pull request", actionContextNormal, nil, nil, true, confirmationNone, "open-pr", ""},
	{"copy-pr-url", "copy pull request URL", actionContextNormal, nil, nil, true, confirmationNone, "copy-pr-url", ""},
	{"settings", "open settings", actionContextNormal, nil, nil, false, confirmationNone, "settings", ""},
	{"move-down", "move down", actionContextNormal, nil, nil, false, confirmationNone, "move-down", ""},
	{"move-up", "move up", actionContextNormal, nil, nil, false, confirmationNone, "move-up", ""},
	{"pan-left", "pan left", actionContextNormal, nil, nil, false, confirmationNone, "pan-left", ""},
	{"pan-right", "pan right", actionContextNormal, nil, nil, false, confirmationNone, "pan-right", ""},
	{"page-down", "page down", actionContextNormal, nil, nil, false, confirmationNone, "page-down", ""},
	{"page-up", "page up", actionContextNormal, nil, nil, false, confirmationNone, "page-up", ""},
	{"next-item", "next item", actionContextNormal, nil, nil, false, confirmationNone, "next-item", ""},
	{"prev-item", "previous item", actionContextNormal, nil, nil, false, confirmationNone, "prev-item", ""},
	{"inspect-pr", "inspect pull request", actionContextNormal, nil, nil, true, confirmationNone, "inspect-pr", ""},
	{"back-to-inbox", "back to inbox", actionContextNormal, nil, nil, false, confirmationNone, "back-to-inbox", ""},
	{"refresh-pr", "refresh pull request", actionContextNormal, nil, nil, true, confirmationNone, "refresh-pr", ""},
	{"refresh-inbox", "refresh full inbox", actionContextNormal, nil, nil, false, confirmationNone, "refresh-inbox", ""},
	{"toggle-diff", "toggle diff", actionContextNormal, nil, nil, true, confirmationNone, "toggle-diff", ""},
	{"open-diff-pager", "open diff pager", actionContextNormal, nil, nil, true, confirmationNone, "open-diff-pager", ""},
	{"toggle-side-by-side", "toggle side-by-side", actionContextNormal, nil, nil, true, confirmationNone, "toggle-side-by-side", ""},
	{"toggle-file-content", "toggle file content", actionContextNormal, nil, nil, true, confirmationNone, "toggle-file-content", ""},
	{"command-palette", "open command palette", actionContextNormal, nil, nil, false, confirmationNone, "command-palette", ""},
	{"help-close", "close help", actionContextHelp, nil, nil, false, confirmationNone, "help-close", ""},
	{"diff-close", "close diff", actionContextDiff, nil, nil, false, confirmationNone, "diff-close", ""},
	{"approve-confirm", "confirm approval", actionContextApprove, nil, nil, true, confirmationNone, "approve-confirm", ""},
	{"approve-cancel", "cancel approval", actionContextApprove, nil, nil, false, confirmationNone, "approve-cancel", ""},
	{"settings-close", "close settings", actionContextSettings, nil, nil, false, confirmationNone, "settings-close", ""},
	{"settings-next", "next setting", actionContextSettings, nil, nil, false, confirmationNone, "settings-next", ""},
	{"settings-prev", "previous setting", actionContextSettings, nil, nil, false, confirmationNone, "settings-prev", ""},
	{"settings-toggle", "toggle setting", actionContextSettings, nil, nil, false, confirmationNone, "settings-toggle", ""},
	{"colorscheme-cancel", "cancel colorscheme", actionContextColorscheme, nil, nil, false, confirmationNone, "colorscheme-cancel", ""},
	{"colorscheme-next", "next colorscheme", actionContextColorscheme, nil, nil, false, confirmationNone, "colorscheme-next", ""},
	{"colorscheme-prev", "previous colorscheme", actionContextColorscheme, nil, nil, false, confirmationNone, "colorscheme-prev", ""},
	{"colorscheme-confirm", "confirm colorscheme", actionContextColorscheme, nil, nil, false, confirmationNone, "colorscheme-confirm", ""},
	{"comment-cancel", "cancel comment", actionContextComment, nil, nil, false, confirmationNone, "comment-cancel", ""},
	{"comment-submit", "submit comment", actionContextComment, nil, nil, true, confirmationNone, "comment-submit", ""},
	{"comment-backspace", "delete comment character", actionContextComment, nil, nil, false, confirmationNone, "comment-backspace", ""},
	{"command-cancel", "cancel command", actionContextCommand, nil, nil, false, confirmationNone, "command-cancel", ""},
	{"command-submit", "submit command", actionContextCommand, nil, nil, false, confirmationNone, "command-submit", ""},
	{"command-next", "next command", actionContextCommand, nil, nil, false, confirmationNone, "command-next", ""},
	{"command-prev", "previous command", actionContextCommand, nil, nil, false, confirmationNone, "command-prev", ""},
	{"command-backspace", "delete command character", actionContextCommand, nil, nil, false, confirmationNone, "command-backspace", ""},
	{"command-confirm", "confirm command", actionContextCommandConfirmation, nil, nil, false, confirmationNone, "command-confirm", ""},
	{"command-cancel-confirmation", "cancel command", actionContextCommandConfirmation, nil, nil, false, confirmationNone, "command-cancel-confirmation", ""},
	{"toggle-draft", "toggle PR draft status", actionContextNormal, nil, nil, true, confirmationNone, "toggle-draft", ""},
	{"toggle-automerge", "toggle auto-merge", actionContextNormal, nil, nil, true, confirmationMandatory, "toggle-automerge", ""},
	{"close-pr", "close PR", actionContextNormal, nil, nil, true, confirmationMandatory, "close-pr", ""},
	{"update-branch", "update branch", actionContextNormal, nil, nil, true, confirmationMandatory, "update-branch", ""},
	{"request-changes", "block PR merge", actionContextNormal, nil, nil, true, confirmationMandatory, "request-changes", ""},
	{"merge-pr", "merge active PR (squash)", actionContextNormal, nil, nil, true, confirmationMandatory, "merge-pr", ""},
	{"colorscheme", "pick color scheme", actionContextNormal, nil, nil, false, confirmationNone, "colorscheme", ""},
}

func newActionRegistry(configured map[string]config.ActionConfig) (actionRegistry, error) {
	registry := actionRegistry{keys: make(map[actionContext]map[string]tuiAction), commands: make(map[string]tuiAction)}
	descriptors := make(map[string]tuiAction, len(actionDescriptors))
	for _, descriptor := range actionDescriptors {
		descriptors[descriptor.ID] = descriptor
	}
	fixedActions := config.FixedActions()
	for _, descriptor := range actionDescriptors {
		fixedAction, ok := fixedActions[descriptor.ID]
		if !ok {
			continue
		}
		descriptor.Keys = append([]string(nil), fixedAction.Keys...)
		descriptor.Commands = append([]string(nil), fixedAction.Commands...)
		if err := registry.add(descriptor); err != nil {
			return actionRegistry{}, err
		}
	}
	ids := make([]string, 0, len(configured))
	for id := range configured {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		configuredAction := configured[id]
		if _, fixed := fixedActions[id]; fixed {
			continue
		}
		if configuredAction.Operation != "" {
			action := tuiAction{ID: id, Context: actionContextNormal, Keys: append([]string(nil), configuredAction.Keys...), Commands: append([]string(nil), configuredAction.Commands...), RequiresPR: true, Confirmation: confirmationMandatory, Operation: configuredAction.Operation, Comment: configuredAction.Comment, Label: configuredAction.Description}
			if configuredAction.Operation == "comment" && configuredAction.Comment == "" {
				action.Confirmation = confirmationNone
			}
			if configuredAction.Confirm != nil && !*configuredAction.Confirm {
				action.Confirmation = confirmationNone
			}
			if err := registry.add(action); err != nil {
				return actionRegistry{}, err
			}
			continue
		}
		action, builtin := descriptors[id]
		if !builtin {
			return actionRegistry{}, fmt.Errorf("action %q is not predefined", id)
		}
		if configuredAction.Comment != "" {
			return actionRegistry{}, fmt.Errorf("action %q: built-in action comment is not allowed", id)
		}
		action.Keys = append([]string(nil), configuredAction.Keys...)
		action.Commands = append([]string(nil), configuredAction.Commands...)
		if err := registry.add(action); err != nil {
			return actionRegistry{}, err
		}
	}
	for _, descriptor := range actionDescriptors {
		if _, ok := registry.actionByID(descriptor.ID); !ok {
			return actionRegistry{}, fmt.Errorf("action %q is missing", descriptor.ID)
		}
	}
	for _, id := range essentialActionIDs {
		action, _ := registry.actionByID(id)
		if len(action.Keys) == 0 {
			return actionRegistry{}, fmt.Errorf("action %q requires a key binding", id)
		}
	}
	registry.bindDiffFallbacks()
	return registry, nil
}

func genericActionLabel(action tuiAction) string {
	if action.Operation == "comment" {
		if action.Comment == "" {
			return "Write a comment on PR."
		}
		return fmt.Sprintf("comment %q", action.Comment)
	}
	label := "Approve PR."
	if action.Confirmation == confirmationNone {
		return label + " No confirmation."
	}
	return label + " Confirmation required."
}

// selectedPRAction describes the next toggle without changing its bindings.
func (m Model) selectedPRAction(action tuiAction) tuiAction {
	pr, ok := m.current()
	if !ok {
		return action
	}
	switch action.ID {
	case "toggle-draft":
		action.Label = "Mark as draft"
		if pr.Draft {
			action.Label = "Mark ready for review"
		}
	case "toggle-automerge":
		action.Label = "Enable auto-merge"
		if pr.AutoMerge {
			action.Label = "Disable auto-merge"
			action.Confirmation = confirmationNone
		}
	}
	return action
}

// diffFallbackActionIDs are normal-context actions that stay reachable while the
// full-screen diff is open. They only scroll, navigate, or change how the diff is
// rendered, so none of them mutate a pull request. Actions that write to GitHub
// (approvals, comments, merges, custom comment actions) are deliberately absent:
// a key press meant for the diff viewer must never trigger a remote change.
var diffFallbackActionIDs = []string{
	"quit", "help",
	"go-top", "go-bottom",
	"move-down", "move-up",
	"pan-left", "pan-right",
	"page-down", "page-up",
	"next-item", "prev-item",
	"toggle-side-by-side", "toggle-file-content",
}

// bindDiffFallbacks copies the allowlisted normal-context bindings into the diff
// context. Keys already claimed by a real diff-context action (such as
// diff-close) keep that binding, preserving the previous lookup precedence.
func (r *actionRegistry) bindDiffFallbacks() {
	for _, id := range diffFallbackActionIDs {
		action, ok := r.actionByID(id)
		if !ok {
			continue
		}
		for _, key := range action.Keys {
			keys := r.keys[actionContextDiff]
			if keys == nil {
				keys = make(map[string]tuiAction)
				r.keys[actionContextDiff] = keys
			}
			if _, taken := keys[key]; taken {
				continue
			}
			keys[key] = action
		}
	}
}

var essentialActionIDs = []string{
	"quit", "help-close", "diff-close", "approve-confirm", "approve-cancel", "settings-close", "colorscheme-cancel", "colorscheme-confirm", "comment-cancel", "comment-submit", "command-cancel", "command-submit", "command-confirm", "command-cancel-confirmation",
}

func (r *actionRegistry) add(action tuiAction) error {
	for _, key := range action.Keys {
		keys := r.keys[action.Context]
		if keys == nil {
			keys = make(map[string]tuiAction)
			r.keys[action.Context] = keys
		}
		if existing, ok := keys[key]; ok {
			return fmt.Errorf("key %q conflicts between actions %q and %q in %s context", key, existing.ID, action.ID, action.Context)
		}
		keys[key] = action
	}
	for _, command := range action.Commands {
		key := strings.ToLower(command)
		if existing, ok := r.commands[key]; ok {
			return fmt.Errorf("command %q conflicts between actions %q and %q", command, existing.ID, action.ID)
		}
		r.commands[key] = action
	}
	r.actions = append(r.actions, action)
	return nil
}

func (r actionRegistry) actionByID(id string) (tuiAction, bool) {
	for _, action := range r.actions {
		if action.ID == id {
			return action, true
		}
	}
	return tuiAction{}, false
}

func (r actionRegistry) actionForKey(context actionContext, key string) (tuiAction, bool) {
	action, ok := r.keys[context][key]
	return action, ok
}

// commandMatch pairs a matched action with the specific command alias that
// matched the typed input, so the palette can display and confirm that alias
// instead of always falling back to the action's first alias.
type commandMatch struct {
	action tuiAction
	alias  string
}

func (r actionRegistry) matchingCommands(prefix string) []commandMatch {
	prefix = strings.ToLower(prefix)
	var matches []commandMatch
	for _, action := range r.actions {
		for _, command := range action.Commands {
			if strings.HasPrefix(strings.ToLower(command), prefix) {
				matches = append(matches, commandMatch{action: action, alias: command})
				break
			}
		}
	}
	return matches
}

func (r actionRegistry) exactCommand(input string) (tuiAction, bool) {
	action, ok := r.commands[strings.ToLower(strings.TrimSpace(input))]
	return action, ok
}

func ciPassing(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "passing", "success":
		return true
	default:
		return false
	}
}

func reviewApproved(decision string) bool {
	return strings.EqualFold(strings.TrimSpace(decision), "APPROVED")
}
