package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeAppliesDisplayDefaultsWhenUnset(t *testing.T) {
	cfg, err := Decode(strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if !cfg.Display.ExpandFirstWave {
		t.Fatal("Display.ExpandFirstWave = false, want default true")
	}
}

func TestDecodePreservesExplicitFalseExpandFirstWave(t *testing.T) {
	cfg, err := Decode(strings.NewReader("display:\n  expand_first_wave: false\n"))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if cfg.Display.ExpandFirstWave {
		t.Fatal("Display.ExpandFirstWave = true, want explicit false")
	}
}

func TestDecodeLoadsConfiguredDiscoveryDirectories(t *testing.T) {
	cfg, err := Decode(strings.NewReader(`
plan_dirs:
  - plans
state_dirs:
  - states
journal_dirs:
  - journals
note_dirs:
  - notes
log_dirs:
  - logs
`))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	assertStrings(t, cfg.PlanDirs, []string{"plans"})
	assertStrings(t, cfg.StateDirs, []string{"states"})
	assertStrings(t, cfg.JournalDirs, []string{"journals"})
	assertStrings(t, cfg.NoteDirs, []string{"notes"})
	assertStrings(t, cfg.LogDirs, []string{"logs"})
}

func TestLoadReadsYAMLConfigFromPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "waveplan-ps.yaml")
	writeTestFile(t, path, "display:\n  expand_first_wave: false\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Display.ExpandFirstWave {
		t.Fatal("Display.ExpandFirstWave = true, want explicit false")
	}
}

func TestLoadResolvesRelativeDiscoveryDirectoriesAgainstConfigPath(t *testing.T) {
	root := t.TempDir()
	cfgDir := filepath.Join(root, "observer")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(%q): %v", cfgDir, err)
	}
	path := filepath.Join(cfgDir, "waveplan-ps.yaml")
	writeTestFile(t, path, `
plan_dirs:
  - ../plans
state_dirs:
  - ../state
journal_dirs:
  - ../journals
note_dirs:
  - ../notes
log_dirs:
  - ../logs
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	assertStrings(t, cfg.PlanDirs, []string{filepath.Join(root, "plans")})
	assertStrings(t, cfg.StateDirs, []string{filepath.Join(root, "state")})
	assertStrings(t, cfg.JournalDirs, []string{filepath.Join(root, "journals")})
	assertStrings(t, cfg.NoteDirs, []string{filepath.Join(root, "notes")})
	assertStrings(t, cfg.LogDirs, []string{filepath.Join(root, "logs")})
}

func assertStrings(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len(%#v) = %d, want %d", got, len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("value[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}
