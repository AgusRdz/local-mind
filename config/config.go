// Package config loads and scaffolds local-mind's on-disk configuration,
// stored under ~/.local-mind/.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Bands holds the confidence thresholds that decide how a matched note is
// injected. Scores are normalized to 0..1 (higher = stronger match).
type Bands struct {
	// VeryHigh and above: inject the note body.
	VeryHigh float64 `yaml:"very_high"`
	// High (up to VeryHigh): inject description + path only.
	High float64 `yaml:"high"`
	// Below High: candidate-only (traced, never injected).
}

// Budget caps how much a single hook invocation may inject.
type Budget struct {
	MaxNotes  int `yaml:"max_notes"`
	MaxTokens int `yaml:"max_tokens"` // approximate; ~4 chars/token
}

// Config is the full on-disk configuration.
type Config struct {
	// Sources are absolute paths to note roots (vault, memory dirs).
	Sources []string `yaml:"sources"`
	// Ignore are glob patterns (matched against the path) to skip.
	Ignore []string `yaml:"ignore"`
	Bands  Bands    `yaml:"bands"`
	Budget Budget   `yaml:"budget"`
}

// Defaults returns a config with seeded thresholds and detected sources.
func Defaults() Config {
	return Config{
		Sources: detectSources(),
		Ignore: []string{
			"**/node_modules/**", "**/.git/**", "**/templates/**",
			"**/.obsidian/**", "**/.trash/**",
			"**/logseq/bak/**", "**/logseq/.recycle/**",
		},
		Bands:   Bands{VeryHigh: 0.60, High: 0.35},
		Budget:  Budget{MaxNotes: 3, MaxTokens: 1200},
	}
}

// BaseDir is ~/.local-mind.
func BaseDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local-mind"), nil
}

// Path is ~/.local-mind/config.yml.
func Path() (string, error) {
	base, err := BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "config.yml"), nil
}

// DBPath is ~/.local-mind/index.db.
func DBPath() (string, error) {
	base, err := BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "index.db"), nil
}

// TracePath is ~/.local-mind/trace.log.
func TracePath() (string, error) {
	base, err := BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "trace.log"), nil
}

// Load reads config.yml, scaffolding it with defaults if absent.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		cfg := Defaults()
		if werr := Save(cfg); werr != nil {
			return cfg, werr
		}
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	applyDefaults(&cfg)
	return cfg, nil
}

// Save writes config.yml, creating the base dir if needed.
func Save(cfg Config) error {
	base, err := BaseDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(base, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", base, err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	path, _ := Path()
	return os.WriteFile(path, data, 0o600)
}

// applyDefaults fills zero-valued knobs so an old/partial config still works.
func applyDefaults(cfg *Config) {
	d := Defaults()
	if cfg.Bands.VeryHigh == 0 {
		cfg.Bands.VeryHigh = d.Bands.VeryHigh
	}
	if cfg.Bands.High == 0 {
		cfg.Bands.High = d.Bands.High
	}
	if cfg.Budget.MaxNotes == 0 {
		cfg.Budget.MaxNotes = d.Budget.MaxNotes
	}
	if cfg.Budget.MaxTokens == 0 {
		cfg.Budget.MaxTokens = d.Budget.MaxTokens
	}
}

// Set updates one scalar knob by name. Returns an error for unknown keys or
// unparseable values.
func (c *Config) Set(key, value string) error {
	switch key {
	case "very_high":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("very_high must be a number: %w", err)
		}
		c.Bands.VeryHigh = f
	case "high":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("high must be a number: %w", err)
		}
		c.Bands.High = f
	case "max_notes":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("max_notes must be an integer: %w", err)
		}
		c.Budget.MaxNotes = n
	case "max_tokens":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("max_tokens must be an integer: %w", err)
		}
		c.Budget.MaxTokens = n
	default:
		return fmt.Errorf("unknown key %q (settable: very_high, high, max_notes, max_tokens)", key)
	}
	return nil
}

// AddSource appends a note root if it exists and isn't already present.
// Returns false (no error) if it was already there.
func (c *Config) AddSource(path string) (bool, error) {
	for _, s := range c.Sources {
		if s == path {
			return false, nil
		}
	}
	fi, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	if !fi.IsDir() {
		return false, fmt.Errorf("%s: not a directory", path)
	}
	c.Sources = append(c.Sources, path)
	return true, nil
}

// AddIgnore appends an ignore glob if not already present.
func (c *Config) AddIgnore(glob string) bool {
	for _, g := range c.Ignore {
		if g == glob {
			return false
		}
	}
	c.Ignore = append(c.Ignore, glob)
	return true
}

// detectSources probes well-known note roots and returns those that exist.
func detectSources() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	candidates := []string{
		filepath.Join(home, ".claude", "memory"),
		filepath.Join(home, "dev", "second-brain"),
	}
	var found []string
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.IsDir() {
			found = append(found, c)
		}
	}
	return found
}
