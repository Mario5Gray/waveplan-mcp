module github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps

go 1.26.1

require (
	github.com/rivo/tview v0.42.0
	github.com/spf13/cobra v1.9.1
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/rivo/tview => ./internal/tviewshim
