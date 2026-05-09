package shlex

import "strings"

// Join returns a shell-escaped command line.
func Join(parts []string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, quote(part))
	}
	return strings.Join(out, " ")
}

func quote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n'\"\\$`!&*()[]{}<>?|;:#~") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
