package tui

import (
	"context"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/senilio/zappr/internal/app"
	"github.com/senilio/zappr/internal/model"
	uitheme "github.com/senilio/zappr/internal/tui/theme"
)

type prActionPhase uint8

const (
	prActionQueued prActionPhase = iota
	prActionRefreshing
	prActionChecking
	prActionConfirming
	prActionRechecking
	prActionSending
)

type queuedPRAction struct {
	command        atlantisCommand // none is a PR refresh
	body           string
	guarded        bool
	confirm        bool
	closedRevision uint64
	phase          prActionPhase
	token          uint64
	checkedHead    string
	reasons        []guardReason
}

type prActionQueue struct {
	jobs        []queuedPRAction
	notice      rowPill
	noticeUntil time.Time
	followUp    bool // one targeted read after the last successful post
}

type prActionResult struct {
	key    app.PRKey
	token  uint64
	update app.PRUpdate
	proof  app.PRVerification
	err    error
}

func (m *Model) enqueueAtlantisComment(pr model.PullRequest, command atlantisCommand, body string, confirm bool) tea.Cmd {
	return m.enqueuePRAction(pr, queuedPRAction{
		command: command, body: body, guarded: command.guarded() && m.atlantisEnabled(), confirm: confirm,
	})
}

func (m *Model) enqueuePRAction(pr model.PullRequest, job queuedPRAction) tea.Cmd {
	key := keyForPR(pr)
	if m.prActions == nil {
		m.prActions = make(map[app.PRKey]*prActionQueue)
	}
	q := m.prActions[key]
	if q == nil {
		q = &prActionQueue{}
		m.prActions[key] = q
	}
	job.closedRevision = m.prVersions[key].closedRevision
	q.notice, q.noticeUntil = rowPill{}, time.Time{}
	q.jobs = append(q.jobs, job)
	return m.startNextPRAction(key)
}

func (m *Model) startNextPRAction(key app.PRKey) tea.Cmd {
	q := m.prActions[key]
	if q == nil || len(q.jobs) == 0 || q.jobs[0].phase != prActionQueued {
		return nil
	}
	job := &q.jobs[0]
	if job.command == atlantisCommandNone {
		return m.dispatchPRAction(key, job, prActionRefreshing, app.PRVerification{})
	}
	return m.dispatchPRAction(key, job, prActionChecking, app.PRVerification{})
}

func (m *Model) dispatchPRAction(key app.PRKey, job *queuedPRAction, phase prActionPhase, proof app.PRVerification) tea.Cmd {
	m.prActionToken++
	job.phase, job.token = phase, m.prActionToken
	if phase == prActionRefreshing {
		if pr, ok := m.current(); ok && keyForPR(pr) == key && m.planState.pendingOpen != nil {
			request := *m.planState.pendingOpen
			request.refresh = job.token
			m.planState.pendingOpen = &request
		}
	}
	a, dispatched := m.app, *job
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		result := prActionResult{key: key, token: dispatched.token}
		switch phase {
		case prActionRefreshing:
			result.update, result.err = a.RefreshPRUpdate(ctx, key)
		case prActionChecking, prActionRechecking:
			result.proof, result.err = a.VerifyPRAction(ctx, key, dispatched.guarded)
			result.update = result.proof.Update
		case prActionSending:
			result.proof = proof
			result.err = a.PostPRAction(ctx, key, dispatched.body, proof, dispatched.closedRevision)
		}
		return result
	}
}

func (m *Model) handlePRActionResult(msg prActionResult) tea.Cmd {
	if msg.update.Closed && m.applyPRUpdate(msg.update) {
		// An accepted removal is valid regardless of which job discovered it.
		return m.reconcilePRUpdates()
	}
	q := m.prActions[msg.key]
	if q == nil || len(q.jobs) == 0 || q.jobs[0].token != msg.token {
		return nil
	}
	job := &q.jobs[0]
	if job.phase == prActionConfirming {
		return nil // duplicate completion of the check that parked this job
	}
	if msg.err != nil {
		if job.phase == prActionRefreshing {
			if pr, ok := m.current(); ok {
				m.failPlanOpen(msg.key, pr.HeadRefOID, msg.token, msg.err)
			}
		}
		label := job.command.label()
		switch job.phase {
		case prActionSending:
			label += " send"
		case prActionChecking, prActionRechecking:
			label += " check"
		}
		label += " failed: " + strings.Join(strings.Fields(readerInput(msg.err.Error())), " ")
		return m.finishPRAction(msg.key, rowPill{truncateANSIWidth(label, 72), uitheme.PillFailed})
	}
	before, _ := m.current()
	if msg.update.Revision != 0 && m.applyPRUpdate(msg.update) {
		m.applyPRSettings()
		if m.prActions[msg.key] == nil {
			// A reopened row cannot revive actions from the previous open period.
			return m.selectionCmd()
		}
	}
	switch job.phase {
	case prActionRefreshing:
		next := m.finishPRAction(msg.key, rowPill{"refreshed", uitheme.PillPassed})
		// The next job already reads the target; update the selected view
		// without racing that read with a redundant details fetch.
		m.reconcilePlan()
		return tea.Batch(next, m.autoDiffCmd(), m.selectedCommitDiffCmd())
	case prActionSending:
		if job.command == atlantisCommandPlan {
			m.recordRecentPlan(msg.proof.Check.PR)
		}
		q.followUp = true
		return m.finishPRAction(msg.key, rowPill{job.command.label() + " sent", uitheme.PillPassed})
	case prActionChecking, prActionRechecking:
		next := m.usePRActionProof(msg.key, job, msg.proof)
		after, ok := m.current()
		if ok && prRefreshKey(before) != prRefreshKey(after) {
			// The precheck cleared the old diff. Replace it without a
			// competing PR details read that could supersede the proof.
			return tea.Batch(next, m.autoDiffCmd(), m.selectedCommitDiffCmd())
		}
		return next
	}
	return nil
}

func (m *Model) usePRActionProof(key app.PRKey, job *queuedPRAction, proof app.PRVerification) tea.Cmd {
	version := m.prVersions[key]
	if proof.Update.ClosedRevision != job.closedRevision || version.revision != proof.Update.Revision {
		return m.finishPRAction(key, rowPill{job.command.label() + " check failed: PR changed", uitheme.PillFailed})
	}
	pr := proof.Check.PR // accepted cache display data is not guard evidence
	var reasons []guardReason
	if job.guarded {
		complete := proof.Check.ChecksKnown && proof.Check.EvidenceErr == nil
		recent := recentPlanKey{repo: key.Repo, number: key.Number, head: pr.HeadRefOID}
		reasons = guardReasons(job.command, pr, m.recentPlans[recent], timeNow(), complete)
		if !complete {
			reason := guardReason{finding: atlantisEvidenceFailed}
			if proof.Check.EvidenceErr != nil {
				reason.detail = proof.Check.EvidenceErr.Error()
			}
			reasons = append(reasons, reason)
		}
	}
	shouldAsk := job.confirm || len(reasons) > 0
	if job.phase == prActionRechecking {
		shouldAsk = job.checkedHead != pr.HeadRefOID || slices.ContainsFunc(reasons, func(reason guardReason) bool {
			return !slices.Contains(job.reasons, reason)
		})
	}
	if shouldAsk {
		job.checkedHead, job.reasons, job.phase = pr.HeadRefOID, reasons, prActionConfirming
		return nil
	}
	return m.dispatchPRAction(key, job, prActionSending, proof)
}

func (m *Model) finishPRAction(key app.PRKey, notice rowPill) tea.Cmd {
	q := m.prActions[key]
	q.jobs[0] = queuedPRAction{}
	q.jobs = q.jobs[1:]
	q.notice, q.noticeUntil = notice, time.Time{}
	if notice.state == uitheme.PillPassed {
		q.noticeUntil = timeNow().Add(3 * time.Second)
	}
	if len(q.jobs) > 0 {
		return m.startNextPRAction(key)
	}
	if notice.label == "" {
		delete(m.prActions, key)
	}
	if q.followUp {
		q.followUp = false
		return refreshPRCmd(m.app, model.PullRequest{Repo: key.Repo, Number: key.Number})
	}
	return nil
}

func (m *Model) expirePRActionNotices(now time.Time) {
	for key, q := range m.prActions {
		if !q.noticeUntil.IsZero() && !now.Before(q.noticeUntil) {
			q.notice, q.noticeUntil = rowPill{}, time.Time{}
			if len(q.jobs) == 0 {
				delete(m.prActions, key)
			}
		}
	}
}

func (m *Model) openPRActionConfirmation() tea.Cmd {
	pr, ok := m.actionPR()
	if m.mode == ModeCommand {
		m.cancelPRModal()
	}
	if !ok {
		return nil
	}
	q := m.prActions[keyForPR(pr)]
	if q == nil || len(q.jobs) == 0 || q.jobs[0].phase != prActionConfirming {
		return nil
	}
	m.cancelPlanOpen()
	m.prevMode, m.mode = m.mode, ModeConfirmAtlantis
	m.modalTarget = m.intentForPR(pr)
	m.prConfirmationToken = q.jobs[0].token
	return nil
}

func (m Model) pendingPRAction() *queuedPRAction {
	q := m.prActions[m.modalTarget.key]
	if q != nil && len(q.jobs) != 0 {
		job := &q.jobs[0]
		if job.phase == prActionConfirming && job.token == m.prConfirmationToken {
			return job
		}
	}
	return nil
}

func (m Model) handleConfirmAtlantisKey(key string) (tea.Model, tea.Cmd) {
	if key == "pgup" || key == "pgdown" {
		_, last, _ := m.atlantisConfirmation()
		if key == "pgup" {
			m.prConfirmationPage = max(0, m.prConfirmationPage-1)
		} else {
			m.prConfirmationPage = min(last, m.prConfirmationPage+1)
		}
		return m, nil
	}
	action, ok := m.actions.actionForKey(actionContextCommandConfirmation, key)
	if !ok {
		return m, nil
	}
	intent, job := m.modalTarget, m.pendingPRAction()
	_, visible := m.visiblePR(intent)
	if visible && job != nil && action.ID == "command-confirm" {
		if _, _, allowed := m.atlantisConfirmation(); !allowed {
			return m, nil
		}
	}
	m.cancelPRModal()
	if !visible || job == nil {
		return m, nil
	}
	switch action.ID {
	case "command-cancel-confirmation":
		cmd := m.finishPRAction(intent.key, rowPill{})
		return m, cmd
	case "command-confirm":
		cmd := m.dispatchPRAction(intent.key, job, prActionRechecking, app.PRVerification{})
		return m, cmd
	}
	return m, nil
}

func (m Model) prActionPills(pr model.PullRequest) [2]rowPill {
	q := m.prActions[keyForPR(pr)]
	if q == nil {
		return [2]rowPill{}
	}
	if len(q.jobs) == 0 {
		return [2]rowPill{q.notice}
	}
	job := q.jobs[0]
	label := job.command.label()
	switch job.phase {
	case prActionRefreshing:
		label = "refreshing"
	case prActionChecking, prActionRechecking:
		label = "checking " + label
	case prActionConfirming:
		label = "confirm " + label
	case prActionSending:
		label = "sending " + label
	default:
		label += " queued"
	}
	var secondary rowPill
	if len(q.jobs) > 1 {
		secondary = rowPill{q.jobs[1].command.label() + " queued", uitheme.PillWait}
	} else if q.notice.state == uitheme.PillFailed {
		secondary = q.notice
	}
	return [2]rowPill{{label, uitheme.PillWait}, secondary}
}
