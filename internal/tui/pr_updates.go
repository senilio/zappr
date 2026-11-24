package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/senilio/zappr/internal/app"
	"github.com/senilio/zappr/internal/model"
)

type prVersion struct {
	revision, closedRevision uint64
}

func keyForPR(pr model.PullRequest) app.PRKey {
	return app.PRKey{Repo: pr.Repo, Number: pr.Number}
}

// applyPRUpdate uses the same accepted revisions for Inbox, targeted reads,
// and local writes. Keep the revision even after the row is removed.
func (m *Model) applyPRUpdate(update app.PRUpdate) bool {
	key := update.Key
	if key.Number <= 0 || (m.repo != "" && key.Repo != m.repo) {
		return false
	}
	previous := m.prVersions[key]
	if update.Revision <= previous.revision && (previous.revision != 0 || update.Closed) {
		return false
	}
	if m.allPRs == nil {
		m.allPRs = append([]model.PullRequest{}, m.prs...)
	}
	index := -1
	for i, pr := range m.allPRs {
		if keyForPR(pr) == key {
			index = i
			if update.Revision == 0 && pr.UpdatedAt.After(update.PR.UpdatedAt) {
				return false
			}
			break
		}
	}
	if m.prVersions == nil {
		m.prVersions = make(map[app.PRKey]prVersion)
	}
	m.prVersions[key] = prVersion{revision: update.Revision, closedRevision: update.ClosedRevision}
	if update.Closed || previous.closedRevision != update.ClosedRevision {
		delete(m.prActions, key)
		for recent := range m.recentPlans {
			if recent.repo == key.Repo && recent.number == key.Number {
				delete(m.recentPlans, recent)
			}
		}
	}
	if update.Closed {
		if index >= 0 {
			m.allPRs = append(m.allPRs[:index], m.allPRs[index+1:]...)
		}
	} else {
		if pr, ok := m.current(); ok && keyForPR(pr) == key &&
			(pr.HeadRefOID != update.PR.HeadRefOID || previous.closedRevision != update.ClosedRevision) {
			m.clearLoadedDiffState()
		}
		if index >= 0 {
			m.allPRs[index] = update.PR
		} else {
			m.allPRs = append(m.allPRs, update.PR)
		}
	}
	return true
}

func (m *Model) reconcilePRUpdates() tea.Cmd {
	m.applyPRSettings()
	return m.selectionCmd()
}

type prIntent struct {
	key            app.PRKey
	closedRevision uint64
}

func (m Model) intentForPR(pr model.PullRequest) prIntent {
	key := keyForPR(pr)
	return prIntent{key: key, closedRevision: m.prVersions[key].closedRevision}
}

func (m Model) visiblePR(intent prIntent) (model.PullRequest, bool) {
	if intent.key.Number != 0 && m.prVersions[intent.key].closedRevision == intent.closedRevision {
		for _, pr := range m.prs {
			if keyForPR(pr) == intent.key {
				return pr, true
			}
		}
	}
	return model.PullRequest{}, false
}

func (m Model) actionPR() (model.PullRequest, bool) {
	switch m.mode {
	case ModeCommand, ModeComment, ModeConfirmApprove, ModeConfirmCommand, ModeConfirmAtlantis:
		return m.visiblePR(m.modalTarget)
	default:
		return m.current()
	}
}

func (m *Model) cancelPRModal() {
	m.mode = m.prevMode
	m.modalTarget = prIntent{}
	m.prConfirmationToken = 0
	m.prConfirmationPage = 0
	m.pendingAction, m.pendingPR = tuiAction{}, model.PullRequest{}
	m.commandInput, m.comment = "", ""
}

func (m *Model) invalidatePRModal() {
	m.invalidatedPRModal = m.mode
	m.cancelPRModal()
	m.message = "action cancelled: original PR is no longer available"
}

// Do not let a late answer to an invalidated prompt become a shortcut on
// the new selection. Repeated confirmation keys are still the same answer.
func (m *Model) consumeInvalidatedPRInput(key string) bool {
	var context actionContext
	var submit, cancel string
	switch m.invalidatedPRModal {
	case ModeComment:
		context, submit, cancel = actionContextComment, "comment-submit", "comment-cancel"
	case ModeCommand:
		context, submit, cancel = actionContextCommand, "command-submit", "command-cancel"
	case ModeConfirmApprove:
		context, submit, cancel = actionContextApprove, "approve-confirm", "approve-cancel"
	case ModeConfirmCommand, ModeConfirmAtlantis:
		context, submit, cancel = actionContextCommandConfirmation, "command-confirm", "command-cancel-confirmation"
	default:
		return false
	}
	action, _ := m.actions.actionForKey(context, key)
	if action.ID == submit {
		return true
	}
	m.invalidatedPRModal = ModeInbox
	return action.ID == cancel
}

func (m *Model) reconcilePRModal() {
	switch m.mode {
	case ModeConfirmAtlantis:
		if _, ok := m.actionPR(); !ok || m.pendingPRAction() == nil {
			m.invalidatePRModal()
		}
	case ModeComment, ModeConfirmApprove, ModeConfirmCommand:
		if _, ok := m.actionPR(); !ok {
			m.invalidatePRModal()
		}
	}
	if len(m.prs) == 0 && (m.mode == ModeDetail || m.mode == ModeDiff) {
		m.mode, m.focus = ModeInbox, FocusInbox
	}
}
