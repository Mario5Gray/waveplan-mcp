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

func TestDecodeLoadsConfiguredExplicitPaths(t *testing.T) {
	cfg, err := Decode(strings.NewReader(`
plan_paths:
  - plans/demo-execution-waves.json
state_paths:
  - state/demo-execution-state.json
journal_paths:
  - journals/demo-execution-journal.json
note_paths:
  - notes/demo.md
log_dirs:
  - logs
`))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	assertStrings(t, cfg.PlanPaths, []string{"plans/demo-execution-waves.json"})
	assertStrings(t, cfg.StatePaths, []string{"state/demo-execution-state.json"})
	assertStrings(t, cfg.JournalPaths, []string{"journals/demo-execution-journal.json"})
	assertStrings(t, cfg.NotePaths, []string{"notes/demo.md"})
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

func TestLoadResolvesRelativeExplicitPathsAgainstConfigPath(t *testing.T) {
	root := t.TempDir()
	cfgDir := filepath.Join(root, "observer")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(%q): %v", cfgDir, err)
	}
	path := filepath.Join(cfgDir, "waveplan-ps.yaml")
	writeTestFile(t, path, `
plan_paths:
  - ../plans/demo-execution-waves.json
state_paths:
  - ../state/demo-execution-state.json
journal_paths:
  - ../journals/demo-execution-journal.json
note_paths:
  - ../notes/demo.md
log_dirs:
  - ../logs
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	assertStrings(t, cfg.PlanPaths, []string{filepath.Join(root, "plans", "demo-execution-waves.json")})
	assertStrings(t, cfg.StatePaths, []string{filepath.Join(root, "state", "demo-execution-state.json")})
	assertStrings(t, cfg.JournalPaths, []string{filepath.Join(root, "journals", "demo-execution-journal.json")})
	assertStrings(t, cfg.NotePaths, []string{filepath.Join(root, "notes", "demo.md")})
	assertStrings(t, cfg.LogDirs, []string{filepath.Join(root, "logs")})
}

func TestDecodeRejectsLegacyDiscoveryDirectories(t *testing.T) {
	_, err := Decode(strings.NewReader("plan_dirs:\n  - plans\n"))
	if err == nil {
		t.Fatal("Decode() error = nil, want legacy scan root rejection")
	}
	if !strings.Contains(err.Error(), "directory scan roots are no longer supported") {
		t.Fatalf("error = %v", err)
	}
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
