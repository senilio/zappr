package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	actionIDPattern     = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	commandAliasPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)
)

type Config struct {
	Refresh    RefreshConfig           `yaml:"refresh"`
	Cache      CacheConfig             `yaml:"cache"`
	Theme      string                  `yaml:"theme,omitempty"`
	Sort       string                  `yaml:"sort,omitempty"`
	HideDrafts bool                    `yaml:"hide_drafts,omitempty"`
	Actions    map[string]ActionConfig `yaml:"actions"`
}

type ActionConfig struct {
	Operation   string   `yaml:"operation,omitempty"`
	Keys        []string `yaml:"keys,omitempty"`
	Commands    []string `yaml:"commands,omitempty"`
	Comment     string   `yaml:"comment,omitempty"`
	Confirm     *bool    `yaml:"confirm,omitempty"`
	Description string   `yaml:"description,omitempty"`
}

func (a ActionConfig) MarshalYAML() (any, error) {
	node := yaml.Node{Kind: yaml.MappingNode}
	if a.Operation != "" {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "operation"},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: a.Operation},
		)
	}
	if a.Keys != nil {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "keys"},
			&yaml.Node{Kind: yaml.SequenceNode, Content: stringNodes(a.Keys)},
		)
	}
	if a.Commands != nil {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "commands"},
			&yaml.Node{Kind: yaml.SequenceNode, Content: stringNodes(a.Commands)},
		)
	}
	if a.Comment != "" {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "comment"},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: a.Comment},
		)
	}
	if a.Confirm != nil {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "confirm"},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(*a.Confirm)},
		)
	}
	if a.Description != "" {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "description"},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: a.Description},
		)
	}
	return &node, nil
}

func stringNodes(values []string) []*yaml.Node {
	nodes := make([]*yaml.Node, len(values))
	for i, value := range values {
		nodes[i] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	}
	return nodes
}

type actionOverride struct {
	Operation   *string   `yaml:"operation"`
	Keys        *[]string `yaml:"keys"`
	Commands    *[]string `yaml:"commands"`
	Comment     *string   `yaml:"comment"`
	Confirm     *bool     `yaml:"confirm"`
	Description *string   `yaml:"description"`
	supplied    map[string]bool
}

func (a *actionOverride) UnmarshalYAML(value *yaml.Node) error {
	type rawActionOverride actionOverride
	var raw rawActionOverride
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*a = actionOverride(raw)
	a.supplied = make(map[string]bool)
	for i := 0; i < len(value.Content); i += 2 {
		a.supplied[value.Content[i].Value] = true
	}
	return nil
}

type RefreshConfig struct {
	Interval Duration `yaml:"interval"`
}

type CacheConfig struct {
	Path string `yaml:"path"`
}

func Default() Config {
	cachePath, _ := DefaultCachePath()
	return Config{
		Refresh: RefreshConfig{Interval: Duration(2 * time.Minute)},
		Cache:   CacheConfig{Path: cachePath},
		Actions: DefaultActions(),
	}
}

func DefaultActions() map[string]ActionConfig {
	confirmed, immediate := true, false
	return map[string]ActionConfig{
		"open-pr":             {Keys: []string{"o"}},
		"copy-pr-url":         {Keys: []string{"C"}},
		"refresh-pr":          {Keys: []string{"r"}},
		"refresh-inbox":       {Keys: []string{"R"}, Commands: []string{"refresh-all"}},
		"toggle-diff":         {Keys: []string{"v"}},
		"open-diff-pager":     {Keys: []string{"V"}},
		"toggle-side-by-side": {Keys: []string{"s"}},
		"toggle-file-content": {Keys: []string{"x"}},
		"approve":             {Operation: "approve", Keys: []string{"a"}, Confirm: &confirmed},
		"approve-now":         {Operation: "approve", Keys: []string{"A"}, Confirm: &immediate},
		"comment":             {Operation: "comment", Keys: []string{"c"}},
		"toggle-draft":        {Commands: []string{"draft"}},
		"close-pr":            {Commands: []string{"close"}},
		"update-branch":       {Commands: []string{"update"}},
		"request-changes":     {Commands: []string{"block"}},
		"merge-pr":            {Commands: []string{"merge"}},
	}
}

func FixedActions() map[string]ActionConfig {
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
		"settings":                    {Keys: []string{"f"}},
		"move-down":                   {Keys: []string{"down"}},
		"move-up":                     {Keys: []string{"up"}},
		"pan-left":                    {Keys: []string{"left"}},
		"pan-right":                   {Keys: []string{"right"}},
		"page-down":                   {Keys: []string{"pgdown", " "}},
		"page-up":                     {Keys: []string{"pgup"}},
		"next-item":                   {Keys: []string{"shift+down"}},
		"prev-item":                   {Keys: []string{"shift+up"}},
		"inspect-pr":                  {Keys: []string{"enter"}},
		"back-to-inbox":               {Keys: []string{"esc"}},
		"command-palette":             {Keys: []string{":"}},
		"help-close":                  {Keys: []string{"esc"}},
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
		Refresh    RefreshConfig             `yaml:"refresh"`
		Cache      CacheConfig               `yaml:"cache"`
		Theme      string                    `yaml:"theme,omitempty"`
		Sort       string                    `yaml:"sort,omitempty"`
		HideDrafts bool                      `yaml:"hide_drafts,omitempty"`
		Actions    map[string]actionOverride `yaml:"actions"`
	}{
		Refresh:    cfg.Refresh,
		Cache:      cfg.Cache,
		Theme:      cfg.Theme,
		Sort:       cfg.Sort,
		HideDrafts: cfg.HideDrafts,
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	cfg.Refresh = file.Refresh
	cfg.Cache = file.Cache
	cfg.Theme = file.Theme
	cfg.Sort = file.Sort
	cfg.HideDrafts = file.HideDrafts
	for id, override := range file.Actions {
		if !actionIDPattern.MatchString(id) {
			return cfg, fmt.Errorf("actions.%s: invalid action ID", id)
		}
		if isRemovedAction(id) {
			return cfg, fmt.Errorf("actions.%s is no longer predefined; use operation: approve or operation: comment", id)
		}
		if fixed, ok := FixedActions()[id]; ok {
			if override.supplied["operation"] || override.supplied["comment"] || override.supplied["confirm"] || override.supplied["description"] ||
				(override.supplied["keys"] && (override.Keys == nil || !reflect.DeepEqual(*override.Keys, fixed.Keys))) ||
				(override.supplied["commands"] && (override.Commands == nil || !reflect.DeepEqual(*override.Commands, fixed.Commands))) {
				return cfg, fmt.Errorf("actions.%s is now built in; remove it from config", id)
			}
			continue
		}
		action, builtin := cfg.Actions[id]
		generic := action.Operation != "" || (!builtin && override.Operation != nil)
		if builtin && !generic {
			if override.Operation != nil {
				return cfg, fmt.Errorf("actions.%s.operation: built-in action behavior cannot be changed", id)
			}
			if override.Comment != nil {
				return cfg, fmt.Errorf("actions.%s.comment: built-in action behavior cannot be changed", id)
			}
			if override.Confirm != nil {
				return cfg, fmt.Errorf("actions.%s.confirm: built-in action behavior cannot be changed", id)
			}
			if override.Description != nil {
				return cfg, fmt.Errorf("actions.%s.description: built-in action behavior cannot be changed", id)
			}
		}
		if generic {
			if override.Operation != nil {
				action.Operation = *override.Operation
			}
			if action.Operation == "" {
				return cfg, fmt.Errorf("actions.%s requires operation", id)
			}
			if action.Operation != "approve" && action.Operation != "comment" {
				return cfg, fmt.Errorf("actions.%s has unknown operation %q", id, action.Operation)
			}
			if override.supplied["comment"] {
				if action.Operation == "approve" {
					return cfg, fmt.Errorf("actions.%s approve actions cannot set comment", id)
				}
				if override.Comment == nil || strings.TrimSpace(*override.Comment) == "" {
					return cfg, fmt.Errorf("actions.%s static comment cannot be blank", id)
				}
				action.Comment = *override.Comment
			}
			if override.supplied["confirm"] && action.Operation == "comment" && action.Comment == "" {
				return cfg, fmt.Errorf("actions.%s editor comments cannot set confirm", id)
			}
			if override.Confirm != nil {
				confirmed := *override.Confirm
				action.Confirm = &confirmed
			}
			if action.Operation == "approve" || action.Comment != "" {
				if action.Confirm == nil {
					confirmed := true
					action.Confirm = &confirmed
				}
			} else {
				action.Confirm = nil
			}
			if override.Description != nil {
				action.Description = *override.Description
			}
		} else if !builtin {
			return cfg, fmt.Errorf("actions.%s requires operation", id)
		}
		if override.Keys != nil {
			action.Keys = append([]string{}, (*override.Keys)...)
		}
		if override.Commands != nil {
			action.Commands = append([]string{}, (*override.Commands)...)
		}
		cfg.Actions[id] = action
	}
	if err := validateActions(cfg.Actions); err != nil {
		return cfg, err
	}
	if cfg.Refresh.Interval == 0 {
		cfg.Refresh.Interval = Duration(2 * time.Minute)
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
		if action.Operation != "" {
			if action.Operation != "approve" && action.Operation != "comment" {
				return fmt.Errorf("actions.%s has unknown operation %q", id, action.Operation)
			}
			if action.Operation == "approve" && action.Comment != "" {
				return fmt.Errorf("actions.%s approve actions cannot set comment", id)
			}
			if len(action.Keys) == 0 && len(action.Commands) == 0 {
				return fmt.Errorf("actions.%s: custom action requires a key or command", id)
			}
		} else if _, builtin := builtins[id]; !builtin {
			return fmt.Errorf("actions.%s requires operation", id)
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

func isRemovedAction(id string) bool {
	switch id {
	case "approve-with-confirmation", "approve-immediately", "write-comment":
		return true
	}
	return false
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
