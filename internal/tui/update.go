package tui

import tea "github.com/charmbracelet/bubbletea"

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case refreshMsg:
		m.loading = false
		if msg.err != nil {
			m.error = msg.err.Error()
			m.prs = m.app.Inbox()
			return m, nil
		}
		m.error = ""
		m.message = "refreshed"
		m.prs = msg.prs
		m.lastFetch = timeNow()
		if m.selected >= len(m.prs) {
			m.selected = len(m.prs) - 1
		}
		if m.selected < 0 {
			m.selected = 0
		}
		return m, nil

	case diffMsg:
		m.loading = false
		if msg.err != nil {
			m.error = msg.err.Error()
			return m, nil
		}
		m.diff = trimDiff(msg.diff)
		m.mode = ModeDiff
		return m, nil

	case actionMsg:
		m.loading = false
		if msg.err != nil {
			m.error = msg.err.Error()
			return m, nil
		}
		m.error = ""
		m.message = msg.message
		m.mode = ModeInbox
		return m, refreshCmd(m.app, m.repo)

	case tickMsg:
		if m.loading {
			return m, tickCmd(m.pollEvery)
		}
		m.loading = true
		m.message = "polling"
		return m, tea.Batch(refreshCmd(m.app, m.repo), tickCmd(m.pollEvery))

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "q" || key == "ctrl+c" {
		return m, tea.Quit
	}

	switch m.mode {
	case ModeComment:
		return m.handleCommentKey(key)
	case ModeConfirmApprove:
		return m.handleApproveKey(key)
	case ModeDiff:
		if key == "esc" || key == "enter" || key == "v" {
			m.mode = ModeDetail
		}
		return m, nil
	}

	switch key {
	case "j", "down":
		if m.selected < len(m.prs)-1 {
			m.selected++
		}
	case "k", "up":
		if m.selected > 0 {
			m.selected--
		}
	case "enter":
		if _, ok := m.current(); ok {
			m.mode = ModeDetail
		}
	case "esc":
		m.mode = ModeInbox
	case "r":
		m.loading = true
		m.message = "refreshing"
		return m, refreshCmd(m.app, m.repo)
	case "v":
		pr, ok := m.current()
		if !ok {
			return m, nil
		}
		m.loading = true
		m.message = "fetching diff"
		return m, diffCmd(m.app, pr)
	case "a":
		if pr, ok := m.current(); ok {
			if pr.Triage.Risk == "block" {
				m.error = "blocked PR requires manual inspection before approval"
				return m, nil
			}
			m.mode = ModeConfirmApprove
		}
	case "c":
		if _, ok := m.current(); ok {
			m.comment = ""
			m.mode = ModeComment
		}
	case "s":
		if pr, ok := m.current(); ok {
			if err := m.app.Skip(pr.Repo, pr.Number); err != nil {
				m.error = err.Error()
				return m, nil
			}
			m.prs = m.app.Inbox()
			m.message = "skipped"
		}
	}
	return m, nil
}

func (m Model) handleApproveKey(key string) (tea.Model, tea.Cmd) {
	if key == "esc" || key == "n" {
		m.mode = ModeDetail
		return m, nil
	}
	if key == "y" {
		pr, ok := m.current()
		if !ok {
			m.mode = ModeInbox
			return m, nil
		}
		m.loading = true
		return m, approveCmd(m.app, pr)
	}
	return m, nil
}

func (m Model) handleCommentKey(key string) (tea.Model, tea.Cmd) {
	if key == "esc" {
		m.mode = ModeDetail
		return m, nil
	}
	if key == "enter" {
		pr, ok := m.current()
		if !ok || m.comment == "" {
			return m, nil
		}
		m.loading = true
		return m, commentCmd(m.app, pr, m.comment)
	}
	if key == "backspace" || key == "ctrl+h" {
		if len(m.comment) > 0 {
			m.comment = m.comment[:len(m.comment)-1]
		}
		return m, nil
	}
	if len(key) == 1 {
		m.comment += key
	}
	return m, nil
}
