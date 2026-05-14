package swim

import (
	"fmt"
	"path/filepath"
	"strings"
)

func artifactBundleName(schedulePath string) string {
	base := filepath.Base(schedulePath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	if name == "" {
		name = base
	}
	if name == "" {
		name = "default"
	}
	return name
}

func defaultArtifactRootRel(schedulePath string) string {
	return filepath.Join(".waveplan", "swim", artifactBundleName(schedulePath))
}

// DefaultArtifactRoot resolves the default artifact root for a schedule.
// Example: docs/plans/x.json -> docs/plans/.waveplan/swim/x
func DefaultArtifactRoot(schedulePath string) string {
	return filepath.Join(filepath.Dir(schedulePath), defaultArtifactRootRel(schedulePath))
}

// ResolveArtifactRoot returns the operator-selected artifact root, or the
// default schedule-adjacent root when no override is provided.
func ResolveArtifactRoot(schedulePath, artifactRoot string) string {
	if artifactRoot != "" {
		return filepath.Clean(artifactRoot)
	}
	return DefaultArtifactRoot(schedulePath)
}

func DeriveLockPath(schedulePath, artifactRoot string) string {
	return filepath.Join(ResolveArtifactRoot(schedulePath, artifactRoot), "swim.lock")
}

func deriveLogPaths(schedulePath, artifactRoot, stepID string, attempt int) (string, string) {
	base := fmt.Sprintf("%s.%d", stepID, attempt)
	if artifactRoot != "" {
		root := ResolveArtifactRoot(schedulePath, artifactRoot)
		return filepath.Join(root, "logs", base+".stdout.log"), filepath.Join(root, "logs", base+".stderr.log")
	}
	root := defaultArtifactRootRel(schedulePath)
	return filepath.Join(root, "logs", base+".stdout.log"), filepath.Join(root, "logs", base+".stderr.log")
}

func deriveLogAbsPaths(schedulePath, artifactRoot, stepID string, attempt int) (string, string) {
	base := fmt.Sprintf("%s.%d", stepID, attempt)
	root := ResolveArtifactRoot(schedulePath, artifactRoot)
	return filepath.Join(root, "logs", base+".stdout.log"), filepath.Join(root, "logs", base+".stderr.log")
}

func deriveDispatchReceiptPath(schedulePath, artifactRoot, stepID string, attempt int) string {
	base := fmt.Sprintf("%s.%d", stepID, attempt)
	return filepath.Join(dispatchReceiptAbsDir(schedulePath, artifactRoot), base+".dispatch.json")
}

