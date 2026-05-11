package swim

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type dispatchReceipt struct {
	OK              bool   `json:"ok"`
	InquiryRequired bool   `json:"inquiry_required,omitempty"`
	InquirySource   string `json:"inquiry_source,omitempty"`
	InquiryHint     string `json:"inquiry_hint,omitempty"`
}

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

func loadDispatchReceipt(schedulePath, stepID string, attempt int) (*dispatchReceipt, error) {
	body, err := os.ReadFile(dispatchReceiptAbsPath(schedulePath, stepID, attempt))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var receipt dispatchReceipt
	if err := json.Unmarshal(body, &receipt); err != nil {
		return nil, err
	}
	return &receipt, nil
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
