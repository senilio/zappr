package tui

import (
	"strings"
)

// viCommand describes a single vim-style colon command.
type viCommand struct {
	Name           string                        // e.g. "ap"
	Description    string                        // shown next to the command in the palette
	RequireConfirm bool                          // if true, prompt before executing
	Execute        func(m *Model) (bool, string) // returns (err, statusMsg)
}

// allCommands is the ordered list of all registered colon commands.
var allCommands = []viCommand{
	{
		Name:           "ap",
		Description:    "comment \"atlantis plan\" on selected PR",
		RequireConfirm: false,
		Execute: func(m *Model) (bool, string) {
			return executeAtlantisComment(m, "atlantis plan")
		},
	},
	{
		Name:           "aa",
		Description:    "comment \"atlantis apply\" on selected PR",
		RequireConfirm: true,
		Execute: func(m *Model) (bool, string) {
			return executeAtlantisComment(m, "atlantis apply")
		},
	},
}

func executeAtlantisComment(m *Model, body string) (bool, string) {
	// This is called from a tea.Cmd; execution is handled via commentCmd.
	// Returns false (no error) and a status message.
	return false, ""
}

// matchingCommands returns commands whose Name starts with prefix (case-insensitive).
func matchingCommands(prefix string) []viCommand {
	prefix = strings.ToLower(prefix)
	var out []viCommand
	for _, cmd := range allCommands {
		if strings.HasPrefix(strings.ToLower(cmd.Name), prefix) {
			out = append(out, cmd)
		}
	}
	return out
}

// exactCommand returns the single command whose Name exactly matches input, or false.
func exactCommand(input string) (viCommand, bool) {
	input = strings.ToLower(strings.TrimSpace(input))
	for _, cmd := range allCommands {
		if strings.ToLower(cmd.Name) == input {
			return cmd, true
		}
	}
	return viCommand{}, false
}

func isAtlantisApply(cmd viCommand) bool {
	return strings.EqualFold(cmd.Name, "aa")
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
