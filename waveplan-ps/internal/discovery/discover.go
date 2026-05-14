package discovery

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/model"
)

// Inventory is the discovered set of files that later packages can load.
type Inventory struct {
	PlanPaths    []string
	StatePaths   []string
	JournalPaths []string
	NotePaths    []string
	Logs         []model.LogRef
}

// Discover recursively scans one root for known waveplan observer files.
func Discover(root string) (*Inventory, error) {
	return DiscoverAll([]string{root})
}

// DiscoverAll recursively scans roots and returns deterministic sorted results.
func DiscoverAll(roots []string) (*Inventory, error) {
	inventory := &Inventory{}
	for _, root := range roots {
		if root == "" {
			continue
		}
		if err := walkFiles(root, func(path string) error {
			// Classify specific sidecars before broader suffixes such as notes
			// or logs so a future case reorder does not change ownership.
			switch {
			case isPlanPath(path):
				inventory.PlanPaths = append(inventory.PlanPaths, path)
			case isStatePath(path):
				inventory.StatePaths = append(inventory.StatePaths, path)
			case isJournalPath(path):
				inventory.JournalPaths = append(inventory.JournalPaths, path)
			case isNotePath(path):
				inventory.NotePaths = append(inventory.NotePaths, path)
			case isLogLikePath(path):
				logRef, err := model.ParseLogPath(path)
				if err != nil {
					return nil
				}
				inventory.Logs = append(inventory.Logs, *logRef)
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	inventory.sort()
	return inventory, nil
}

// DiscoverPlans recursively scans root for execution-waves plan JSON files.
func DiscoverPlans(root string) ([]string, error) {
	return discoverPaths(root, isPlanPath)
}

// DiscoverStates recursively scans root for waveplan state sidecars.
func DiscoverStates(root string) ([]string, error) {
	return discoverPaths(root, isStatePath)
}

// DiscoverJournals recursively scans root for SWIM journal sidecars.
func DiscoverJournals(root string) ([]string, error) {
	return discoverPaths(root, isJournalPath)
}

// DiscoverNotes recursively scans root for markdown notes.
func DiscoverNotes(root string) ([]string, error) {
	return discoverPaths(root, isNotePath)
}

// DiscoverLogs recursively scans root for SWIM log files and validates names.
func DiscoverLogs(root string) ([]model.LogRef, error) {
	var logs []model.LogRef
	if err := walkFiles(root, func(path string) error {
		if !isLogLikePath(path) {
			return nil
		}
		logRef, err := model.ParseLogPath(path)
		if err != nil {
			return fmt.Errorf("parse log %q: %w", path, err)
		}
		logs = append(logs, *logRef)
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].Path < logs[j].Path
	})
	return logs, nil
}

func discoverPaths(root string, match func(string) bool) ([]string, error) {
	var paths []string
	if err := walkFiles(root, func(path string) error {
		if match(path) {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func walkFiles(root string, visit func(string) error) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %q: %w", path, err)
		}
		if entry.IsDir() {
			return nil
		}
		return visit(path)
	})
}

func (i *Inventory) sort() {
	sort.Strings(i.PlanPaths)
	sort.Strings(i.StatePaths)
	sort.Strings(i.JournalPaths)
	sort.Strings(i.NotePaths)
	sort.Slice(i.Logs, func(left, right int) bool {
		return i.Logs[left].Path < i.Logs[right].Path
	})
}

func isPlanPath(path string) bool {
	return strings.HasSuffix(filepath.Base(path), "-execution-waves.json")
}

func isStatePath(path string) bool {
	name := filepath.Base(path)
	return strings.HasSuffix(name, ".state.json") || strings.HasSuffix(name, "-execution-state.json")
}

func isJournalPath(path string) bool {
	name := filepath.Base(path)
	return strings.HasSuffix(name, ".journal.json") || strings.HasSuffix(name, "-execution-journal.json")
}

func isNotePath(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".md")
}

func isLogLikePath(path string) bool {
	name := filepath.Base(path)
	return strings.HasSuffix(name, ".stdout.log") || strings.HasSuffix(name, ".stderr.log")
}
