package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Refresh RefreshConfig `yaml:"refresh"`
	AI      AIConfig      `yaml:"ai"`
	Cache   CacheConfig   `yaml:"cache"`
	Trust   TrustConfig   `yaml:"trust"`
	Theme   string        `yaml:"theme,omitempty"`
	Sort    string        `yaml:"sort,omitempty"`
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
		return cfg, nil
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
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
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
