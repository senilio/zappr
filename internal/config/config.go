package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	actionIDPattern     = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	commandAliasPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)
)

type Config struct {
	Refresh RefreshConfig           `yaml:"refresh"`
	AI      AIConfig                `yaml:"ai"`
	Cache   CacheConfig             `yaml:"cache"`
	Trust   TrustConfig             `yaml:"trust"`
	Theme   string                  `yaml:"theme,omitempty"`
	Sort    string                  `yaml:"sort,omitempty"`
	Actions map[string]ActionConfig `yaml:"actions"`
}

type ActionConfig struct {
	Keys     []string `yaml:"keys,omitempty"`
	Commands []string `yaml:"commands,omitempty"`
	Comment  string   `yaml:"comment,omitempty"`
	Confirm  *bool    `yaml:"confirm,omitempty"`
}

type actionOverride struct {
	Keys     *[]string `yaml:"keys"`
	Commands *[]string `yaml:"commands"`
	Comment  *string   `yaml:"comment"`
	Confirm  *bool     `yaml:"confirm"`
}

type TrustConfig struct {
	// Authors lists GitHub usernames whose PRs are biased toward safe.
	// Trust never overrides a block verdict.
	Authors []string `yaml:"authors"`
}

type RefreshConfig struct {
	Interval Duration `yaml:"interval"`
}

type AIConfig struct {
	Command string   `yaml:"command"`
	Prompt  string   `yaml:"prompt"`
	Timeout Duration `yaml:"timeout"`
}

type CacheConfig struct {
	Path string `yaml:"path"`
}

func Default() Config {
	cachePath, _ := DefaultCachePath()
	return Config{
		Refresh: RefreshConfig{Interval: Duration(2 * time.Minute)},
		AI:      AIConfig{Timeout: Duration(60 * time.Second)},
		Cache:   CacheConfig{Path: cachePath},
		Actions: DefaultActions(),
	}
}

func DefaultActions() map[string]ActionConfig {
	return map[string]ActionConfig{
		"quit":                        {Keys: []string{"q", "ctrl+c"}},
		"help":                        {Keys: []string{"?"}},
		"summary-files":               {Keys: []string{"1"}},
		"summary-commits":             {Keys: []string{"2"}},
		"summary-comments":            {Keys: []string{"3"}},
		"summary-atlantis":            {Keys: []string{"4"}},
		"focus-next":                  {Keys: []string{"tab"}},
		"focus-prev":                  {Keys: []string{"shift+tab"}},
		"go-top":                      {Keys: []string{"g"}},
		"go-bottom":                   {Keys: []string{"G"}},
		"open-pr":                     {Keys: []string{"o"}},
		"copy-pr-url":                 {Keys: []string{"C"}},
		"settings":                    {Keys: []string{"f"}},
		"move-down":                   {Keys: []string{"j", "down"}},
		"move-up":                     {Keys: []string{"k", "up"}},
		"pan-left":                    {Keys: []string{"left"}},
		"pan-right":                   {Keys: []string{"right"}},
		"page-down":                   {Keys: []string{"pgdown", " "}},
		"page-up":                     {Keys: []string{"pgup"}},
		"next-item":                   {Keys: []string{"shift+down"}},
		"prev-item":                   {Keys: []string{"shift+up"}},
		"inspect-pr":                  {Keys: []string{"enter"}},
		"back-to-inbox":               {Keys: []string{"esc"}},
		"refresh-pr":                  {Keys: []string{"r"}},
		"refresh-inbox":               {Keys: []string{"R"}, Commands: []string{"refresh-all"}},
		"toggle-diff":                 {Keys: []string{"v"}},
		"open-diff-pager":             {Keys: []string{"V"}},
		"next-file":                   {Keys: []string{"n", "]"}},
		"prev-file":                   {Keys: []string{"p", "["}},
		"toggle-side-by-side":         {Keys: []string{"s"}},
		"toggle-file-content":         {Keys: []string{"x"}},
		"approve-with-confirmation":   {Keys: []string{"a"}},
		"approve-immediately":         {Keys: []string{"A"}},
		"write-comment":               {Keys: []string{"c"}},
		"command-palette":             {Keys: []string{":"}},
		"help-close":                  {Keys: []string{"esc", "?", "enter"}},
		"diff-close":                  {Keys: []string{"esc", "enter", "v"}},
		"approve-confirm":             {Keys: []string{"y"}},
		"approve-cancel":              {Keys: []string{"esc", "n"}},
		"settings-close":              {Keys: []string{"esc", "f"}},
		"settings-next":               {Keys: []string{"j", "down"}},
		"settings-prev":               {Keys: []string{"k", "up"}},
		"settings-toggle":             {Keys: []string{" ", "enter", "right", "l", "left", "h"}},
		"colorscheme-cancel":          {Keys: []string{"esc", "q"}},
		"colorscheme-next":            {Keys: []string{"j", "down"}},
		"colorscheme-prev":            {Keys: []string{"k", "up"}},
		"colorscheme-confirm":         {Keys: []string{"enter", " "}},
		"comment-cancel":              {Keys: []string{"esc"}},
		"comment-submit":              {Keys: []string{"enter"}},
		"comment-backspace":           {Keys: []string{"backspace", "ctrl+h"}},
		"command-cancel":              {Keys: []string{"esc"}},
		"command-submit":              {Keys: []string{"enter"}},
		"command-next":                {Keys: []string{"down", "ctrl+j"}},
		"command-prev":                {Keys: []string{"up", "ctrl+k"}},
		"command-backspace":           {Keys: []string{"backspace", "ctrl+h"}},
		"command-confirm":             {Keys: []string{"y", "enter"}},
		"command-cancel-confirmation": {Keys: []string{"esc", "n"}},
		"atlantis-plan":               {Commands: []string{"ap"}},
		"atlantis-apply":              {Commands: []string{"aa"}},
		"atlantis-unlock":             {Commands: []string{"au"}},
		"toggle-draft":                {Commands: []string{"draft"}},
		"close-pr":                    {Commands: []string{"close"}},
		"update-branch":               {Commands: []string{"update"}},
		"request-changes":             {Commands: []string{"block"}},
		"merge-pr":                    {Commands: []string{"merge"}},
		"ai-review":                   {Commands: []string{"ai"}},
		"colorscheme":                 {Commands: []string{"colorscheme"}},
	}
}

func Load() (Config, error) {
	cfg := Default()
	path, err := DefaultConfigPath()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, Save(cfg)
	}
	if err != nil {
		return cfg, err
	}
	if err := privateDir(filepath.Dir(path)); err != nil {
		return cfg, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return cfg, err
	}
	file := struct {
		Refresh RefreshConfig             `yaml:"refresh"`
		AI      AIConfig                  `yaml:"ai"`
		Cache   CacheConfig               `yaml:"cache"`
		Trust   TrustConfig               `yaml:"trust"`
		Theme   string                    `yaml:"theme,omitempty"`
		Sort    string                    `yaml:"sort,omitempty"`
		Actions map[string]actionOverride `yaml:"actions"`
	}{
		Refresh: cfg.Refresh,
		AI:      cfg.AI,
		Cache:   cfg.Cache,
		Trust:   cfg.Trust,
		Theme:   cfg.Theme,
		Sort:    cfg.Sort,
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	cfg.Refresh = file.Refresh
	cfg.AI = file.AI
	cfg.Cache = file.Cache
	cfg.Trust = file.Trust
	cfg.Theme = file.Theme
	cfg.Sort = file.Sort
	for id, override := range file.Actions {
		action, builtin := cfg.Actions[id]
		if !actionIDPattern.MatchString(id) {
			return cfg, fmt.Errorf("actions.%s: invalid action ID", id)
		}
		if builtin && override.Comment != nil {
			return cfg, fmt.Errorf("actions.%s.comment: built-in action behavior cannot be changed", id)
		}
		if builtin && override.Confirm != nil {
			return cfg, fmt.Errorf("actions.%s.confirm: built-in action behavior cannot be changed", id)
		}
		if !builtin {
			if override.Comment == nil || strings.TrimSpace(*override.Comment) == "" {
				return cfg, fmt.Errorf("actions.%s.comment: custom action comment is required", id)
			}
			confirmed := true
			action = ActionConfig{Comment: *override.Comment, Confirm: &confirmed}
		}
		if override.Keys != nil {
			action.Keys = append([]string(nil), (*override.Keys)...)
		}
		if override.Commands != nil {
			action.Commands = append([]string(nil), (*override.Commands)...)
		}
		if !builtin && override.Confirm != nil {
			confirmed := *override.Confirm
			action.Confirm = &confirmed
		}
		cfg.Actions[id] = action
	}
	if err := validateActions(cfg.Actions); err != nil {
		return cfg, err
	}
	if cfg.Refresh.Interval == 0 {
		cfg.Refresh.Interval = Duration(2 * time.Minute)
	}
	if cfg.AI.Timeout == 0 {
		cfg.AI.Timeout = Duration(60 * time.Second)
	}
	if cfg.Cache.Path == "" {
		cfg.Cache.Path, _ = DefaultCachePath()
	}
	return cfg, nil
}

func validateActions(actions map[string]ActionConfig) error {
	builtins := DefaultActions()
	ids := make([]string, 0, len(actions))
	for id := range actions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	commands := make(map[string]string)
	for _, id := range ids {
		action := actions[id]
		if !actionIDPattern.MatchString(id) {
			return fmt.Errorf("actions.%s: invalid action ID", id)
		}
		if _, builtin := builtins[id]; !builtin {
			if strings.TrimSpace(action.Comment) == "" {
				return fmt.Errorf("actions.%s.comment: custom action comment is required", id)
			}
			if len(action.Keys) == 0 && len(action.Commands) == 0 {
				return fmt.Errorf("actions.%s: custom action requires a key or command", id)
			}
		}
		for _, command := range action.Commands {
			if !commandAliasPattern.MatchString(command) {
				return fmt.Errorf("actions.%s.commands: invalid command alias %q", id, command)
			}
			alias := strings.ToLower(command)
			if other, exists := commands[alias]; exists {
				return fmt.Errorf("actions.%s.commands: command alias %q conflicts with %s", id, command, other)
			}
			commands[alias] = id
		}
	}
	return nil
}

type Duration time.Duration

func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

func (d Duration) MarshalYAML() (any, error) {
	return time.Duration(d).String(), nil
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		parsed, err := time.ParseDuration(value.Value)
		if err == nil {
			*d = Duration(parsed)
			return nil
		}
	}
	var ns int64
	if err := value.Decode(&ns); err != nil {
		return err
	}
	*d = Duration(time.Duration(ns))
	return nil
}

// Save writes cfg back to the default config path. Overwrites the whole file.
func Save(cfg Config) error {
	path, err := DefaultConfigPath()
	if err != nil {
		return err
	}
	if err := privateDir(filepath.Dir(path)); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func Init(force bool) (string, error) {
	path, err := DefaultConfigPath()
	if err != nil {
		return "", err
	}
	if err := privateDir(filepath.Dir(path)); err != nil {
		return path, err
	}
	if !force {
		if _, err := os.Stat(path); err == nil {
			if err := os.Chmod(path, 0o600); err != nil {
				return path, err
			}
			return path, fmt.Errorf("config already exists: %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return path, err
		}
	}
	data, err := yaml.Marshal(Default())
	if err != nil {
		return path, err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return path, err
	}
	return path, os.Chmod(path, 0o600)
}

func privateDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

func DefaultConfigPath() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "zappr", "config.yaml"), nil
}

func DefaultCachePath() (string, error) {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "zappr", "zappr.db"), nil
}
