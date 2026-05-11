package swim

import (
	"os"
	"path/filepath"
)

func isDispatchAction(action string) bool {
	switch action {
	case "implement", "review", "fix":
		return true
	default:
		return false
	}
}

func dispatchReceiptExists(schedulePath, stepID string, attempt int) bool {
	_, err := os.Stat(dispatchReceiptAbsPath(schedulePath, stepID, attempt))
	return err == nil
}

func anyDispatchReceiptExists(schedulePath, stepID string) bool {
	pattern := filepath.Join(dispatchReceiptAbsDir(schedulePath), stepID+".*.dispatch.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return false
	}
	return len(matches) > 0
}

func dispatchReceiptAbsPath(schedulePath, stepID string, attempt int) string {
	return filepath.Join(filepath.Dir(schedulePath), deriveDispatchReceiptPath(schedulePath, stepID, attempt))
}

func dispatchReceiptAbsDir(schedulePath string) string {
	return filepath.Join(filepath.Dir(schedulePath), deriveDispatchReceiptDir(schedulePath))
}
