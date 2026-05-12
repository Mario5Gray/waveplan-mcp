package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the normalized waveplan-ps YAML configuration.
type Config struct {
	PlanDirs    []string      `yaml:"plan_dirs"`
	StateDirs   []string      `yaml:"state_dirs"`
	JournalDirs []string      `yaml:"journal_dirs"`
	NoteDirs    []string      `yaml:"note_dirs"`
	LogDirs     []string      `yaml:"log_dirs"`
	Display     DisplayConfig `yaml:"display"`
}

// DisplayConfig controls initial observer rendering behavior.
type DisplayConfig struct {
	ExpandFirstWave bool `yaml:"expand_first_wave"`
}

type rawConfig struct {
	PlanDirs    []string         `yaml:"plan_dirs"`
	StateDirs   []string         `yaml:"state_dirs"`
	JournalDirs []string         `yaml:"journal_dirs"`
	NoteDirs    []string         `yaml:"note_dirs"`
	LogDirs     []string         `yaml:"log_dirs"`
	Display     rawDisplayConfig `yaml:"display"`
}

type rawDisplayConfig struct {
	ExpandFirstWave *bool `yaml:"expand_first_wave"`
}

// Default returns the config values used when a YAML file omits a field.
func Default() Config {
	return Config{
		Display: DisplayConfig{
			ExpandFirstWave: true,
		},
	}
}

// Load reads and normalizes a YAML configuration file from path.
func Load(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config %q: %w", path, err)
	}
	defer file.Close()

	cfg, err := Decode(file)
	if err != nil {
		return nil, fmt.Errorf("decode config %q: %w", path, err)
	}
	baseDir := filepath.Dir(path)
	cfg.PlanDirs = resolvePaths(baseDir, cfg.PlanDirs)
	cfg.StateDirs = resolvePaths(baseDir, cfg.StateDirs)
	cfg.JournalDirs = resolvePaths(baseDir, cfg.JournalDirs)
	cfg.NoteDirs = resolvePaths(baseDir, cfg.NoteDirs)
	cfg.LogDirs = resolvePaths(baseDir, cfg.LogDirs)
	return cfg, nil
}

// Decode reads and normalizes YAML configuration from r.
func Decode(r io.Reader) (*Config, error) {
	var raw rawConfig
	decoder := yaml.NewDecoder(r)
	decoder.KnownFields(true)
	if err := decoder.Decode(&raw); err != nil {
		return nil, err
	}

	cfg := Default()
	cfg.PlanDirs = raw.PlanDirs
	cfg.StateDirs = raw.StateDirs
	cfg.JournalDirs = raw.JournalDirs
	cfg.NoteDirs = raw.NoteDirs
	cfg.LogDirs = raw.LogDirs
	if raw.Display.ExpandFirstWave != nil {
		cfg.Display.ExpandFirstWave = *raw.Display.ExpandFirstWave
	}
	return &cfg, nil
}

func resolvePaths(baseDir string, paths []string) []string {
	resolved := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" || filepath.IsAbs(path) {
			resolved = append(resolved, path)
			continue
		}
		resolved = append(resolved, filepath.Clean(filepath.Join(baseDir, path)))
	}
	return resolved
}
