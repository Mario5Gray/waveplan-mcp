package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// logEvent is the top-level envelope for opencode --format json JSONL lines.
type logEvent struct {
	Type      string          `json:"type"`
	Timestamp int64           `json:"timestamp"`
	SessionID string          `json:"sessionID"`
	Part      json.RawMessage `json:"part"`
}

type stepStartPart struct {
	Snapshot string `json:"snapshot"`
	Type     string `json:"type"`
}

type textPartTime struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

type textPart struct {
	Text string       `json:"text"`
	Time textPartTime `json:"time"`
}

type cacheTokens struct {
	Write int `json:"write"`
	Read  int `json:"read"`
}

type tokenUsage struct {
	Total     int         `json:"total"`
	Input     int         `json:"input"`
	Output    int         `json:"output"`
	Reasoning int         `json:"reasoning"`
	Cache     cacheTokens `json:"cache"`
}

type stepFinishPart struct {
	Tokens tokenUsage `json:"tokens"`
}

type toolStateTime struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

type toolState struct {
	Status string        `json:"status"`
	Title  string        `json:"title"`
	Time   toolStateTime `json:"time"`
}

type toolUsePart struct {
	Tool  string    `json:"tool"`
	State toolState `json:"state"`
}

const logTimeFmt = "20060102-15:04:05"
const maxTextChars = 255

// renderLogEvent converts one JSONL line into display lines.
// Non-JSON or unrecognised lines are returned as-is.
func renderLogEvent(line string) []string {
	line = strings.TrimSpace(line)
	if line == "" || line[0] != '{' {
		return []string{line}
	}
	var ev logEvent
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		return []string{line}
	}

	ts := time.UnixMilli(ev.Timestamp).UTC()
	tStr := ts.Format(logTimeFmt)
	sess := shortSessionID(ev.SessionID)

	switch ev.Type {
	case "step_start":
		var p stepStartPart
		if err := json.Unmarshal(ev.Part, &p); err != nil {
			break
		}
		return []string{fmt.Sprintf("%s  %s  %s  %s", tStr, sess, shortHash(p.Snapshot), p.Type)}

	case "text":
		var p textPart
		if err := json.Unmarshal(ev.Part, &p); err != nil {
			break
		}
		text := p.Text
		if len(text) > maxTextChars {
			text = text[:maxTextChars] + "…"
		}
		dur := formatLogDuration(p.Time.End - p.Time.Start)
		header := fmt.Sprintf("%s  %s  (%s)", tStr, sess, dur)
		out := []string{header}
		for _, tl := range strings.Split(text, "\n") {
			out = append(out, tl)
		}
		return out

	case "step_finish":
		var p stepFinishPart
		if err := json.Unmarshal(ev.Part, &p); err != nil {
			break
		}
		t := p.Tokens
		return []string{fmt.Sprintf("%s  tot:%d in:%d out:%d rsn:%d cw:%d cr:%d",
			tStr, t.Total, t.Input, t.Output, t.Reasoning, t.Cache.Write, t.Cache.Read)}

	case "tool_use":
		var p toolUsePart
		if err := json.Unmarshal(ev.Part, &p); err != nil {
			break
		}
		start := fmtMilliTime(p.State.Time.Start)
		end := fmtMilliTime(p.State.Time.End)
		title := p.State.Title
		return []string{fmt.Sprintf("%s  %s  tool_use  %s  %s  %s  %s→%s",
			tStr, sess, p.Tool, p.State.Status, title, start, end)}
	}
	return []string{line}
}

// fmtMilliTime formats a millisecond unix timestamp as HH:MM:ss.
func fmtMilliTime(ms int64) string {
	if ms == 0 {
		return "-"
	}
	return time.UnixMilli(ms).UTC().Format("15:04:05")
}

// readRenderedLogLines reads up to n display lines from path.
// JSONL files (first non-empty line starts with '{') are rendered via renderLogEvent;
// plain text files fall back to the same raw tail behaviour as before.
func readRenderedLogLines(path string, n int) []string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil
	}
	raw := strings.TrimRight(string(data), "\n")
	allRaw := strings.Split(raw, "\n")

	isJSONL := false
	for _, l := range allRaw {
		if t := strings.TrimSpace(l); t != "" {
			isJSONL = t[0] == '{'
			break
		}
	}

	if !isJSONL {
		if len(allRaw) > n {
			allRaw = allRaw[len(allRaw)-n:]
		}
		return allRaw
	}

	var rendered []string
	for _, l := range allRaw {
		rendered = append(rendered, renderLogEvent(l)...)
	}
	if len(rendered) > n {
		rendered = rendered[len(rendered)-n:]
	}
	return rendered
}

func shortSessionID(id string) string {
	if len(id) > 8 {
		return id[len(id)-8:]
	}
	return id
}

func shortHash(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	return h
}

func formatLogDuration(ms int64) string {
	if ms <= 0 {
		return "0s"
	}
	total := ms / 1000
	m := total / 60
	s := total % 60
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
