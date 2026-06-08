package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/senilio/zappr/internal/model"
)

type ShellProvider struct {
	Command string
	Prompt  string
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
	if p.Prompt != "" {
		cmd.Env = append(os.Environ(), "ZAPPR_AI_PROMPT="+p.Prompt)
	}
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

	// Extract the first JSON object from the output, tolerating prose or
	// markdown-fenced responses from LLM CLIs (e.g. ```json ... ```).
	jsonBytes := extractJSON(out)
	if len(jsonBytes) == 0 {
		snippet := strings.TrimSpace(string(out))
		if len(snippet) > 120 {
			snippet = snippet[:120] + "…"
		}
		if snippet == "" {
			return nil, fmt.Errorf("ai command produced no output")
		}
		return nil, fmt.Errorf("parse ai output: no JSON object found in output: %s", snippet)
	}

	var triage model.AITriage
	if err := json.Unmarshal(jsonBytes, &triage); err != nil {
		snippet := strings.TrimSpace(string(jsonBytes))
		if len(snippet) > 120 {
			snippet = snippet[:120] + "…"
		}
		return nil, fmt.Errorf("parse ai output: %w (got: %s)", err, snippet)
	}
	return &triage, nil
}

// extractJSON returns the bytes of the first complete JSON object ({...})
// found in src, or nil if none is found.
func extractJSON(src []byte) []byte {
	start := bytes.IndexByte(src, '{')
	if start < 0 {
		return nil
	}
	end := bytes.LastIndexByte(src, '}')
	if end < start {
		return nil
	}
	return src[start : end+1]
}
