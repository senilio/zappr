package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/senilio/zappr/internal/model"
)

type ShellProvider struct {
	Command string
	Timeout time.Duration
}

func (p ShellProvider) Triage(ctx context.Context, pr model.PullRequest) (*model.AITriage, error) {
	if strings.TrimSpace(p.Command) == "" {
		return nil, nil
	}
	timeout := p.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	input, err := json.Marshal(pr)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", p.Command)
	cmd.Stdin = bytes.NewReader(input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("ai command: %s", msg)
	}

	var triage model.AITriage
	if err := json.Unmarshal(out, &triage); err != nil {
		return nil, fmt.Errorf("parse ai output: %w", err)
	}
	return &triage, nil
}
