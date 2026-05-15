package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/config"
	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/ui"
	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/watch"
	"github.com/rivo/tview"
)

const livePageName = "snapshot"

type cliOptions struct {
	configPath          string
	once                bool
	planPaths           []string
	statePaths          []string
	journalPaths        []string
	reviewSchedulePaths []string
	notePaths           []string
	logDirs             []string
	interval            time.Duration
	tailLimit           int
	journalLimit        int
	logTailLines        int
	unitLimit           int
	expandFirstWave     bool
}

type rootCommand struct {
	out     io.Writer
	err     io.Writer
	args    []string
	argsSet bool
}

// NewRootCommand constructs the waveplan-ps CLI.
func NewRootCommand() *rootCommand {
	return &rootCommand{
		out: os.Stdout,
		err: os.Stderr,
	}
}

func (c *rootCommand) SetOut(out io.Writer) {
	c.out = out
}

func (c *rootCommand) SetErr(err io.Writer) {
	c.err = err
}

func (c *rootCommand) SetArgs(args []string) {
	c.args = append([]string(nil), args...)
	c.argsSet = true
}

func (c *rootCommand) Execute() error {
	return c.ExecuteContext(context.Background())
}

func (c *rootCommand) ExecuteContext(ctx context.Context) error {
	opts := cliOptions{
		interval:        time.Second,
		tailLimit:       10,
		journalLimit:    10,
		logTailLines:    8,
		unitLimit:       10,
		expandFirstWave: true,
	}

	flags := newFlagSet(&opts, c.err)
	args := c.args
	if !c.argsSet {
		args = os.Args[1:]
	}
	if err := flags.Parse(args); err != nil {
		return err
	}

	cfg := config.Default()
	if opts.configPath != "" {
		loaded, err := config.Load(opts.configPath)
		if err != nil {
			return err
		}
		cfg = *loaded
	}
	if flagWasPassed(flags, "expand-first-wave") {
		cfg.Display.ExpandFirstWave = opts.expandFirstWave
	}

	watchOptions, err := buildWatchOptions(cfg, opts)
	if err != nil {
		return err
	}
	if err := validateWatchOptions(watchOptions); err != nil {
		return err
	}
	renderOptions := ui.Options{
		ExpandFirstWave:  cfg.Display.ExpandFirstWave,
		TailLimit:        opts.tailLimit,
		JournalLimit:     opts.journalLimit,
		LogTailLines:     opts.logTailLines,
		TableVisibleRows: opts.unitLimit,
	}
	if opts.once {
		return runOnce(c.out, watchOptions, renderOptions)
	}
	return runLive(ctx, watchOptions, renderOptions, opts.interval)
}

func main() {
	if err := NewRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newFlagSet(opts *cliOptions, errOut io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet("waveplan-ps", flag.ContinueOnError)
	flags.SetOutput(errOut)
	flags.StringVar(&opts.configPath, "config", "", "YAML config file")
	flags.BoolVar(&opts.once, "once", false, "render one snapshot and exit")
	flags.Var((*stringArray)(&opts.planPaths), "plan", "execution-waves plan path (repeatable)")
	flags.Var((*stringArray)(&opts.statePaths), "state", "waveplan state sidecar path (repeatable)")
	flags.Var((*stringArray)(&opts.journalPaths), "journal", "SWIM journal sidecar path (repeatable)")
	flags.Var((*stringArray)(&opts.reviewSchedulePaths), "review-schedule", "SWIM review schedule sidecar path (repeatable)")
	flags.Var((*stringArray)(&opts.notePaths), "note", "txtstore note path (repeatable)")
	flags.Var((*stringArray)(&opts.logDirs), "log-dir", "directory to recursively scan for SWIM logs")
	flags.DurationVar(&opts.interval, "interval", time.Second, "live refresh interval")
	flags.IntVar(&opts.tailLimit, "tail-limit", 10, "maximum tail rows to render")
	flags.IntVar(&opts.journalLimit, "journal-limit", 10, "maximum journal events to render")
	flags.IntVar(&opts.logTailLines, "log-lines", 8, "lines of stdio log to show in active log panel")
	flags.IntVar(&opts.unitLimit, "unit-limit", 10, "visible rows in the wave/unit table before scrolling")
	flags.BoolVar(&opts.expandFirstWave, "expand-first-wave", true, "expand the first wave initially")
	return flags
}

func buildWatchOptions(cfg config.Config, opts cliOptions) (watch.Options, error) {
	planPaths := appendPaths(cfg.PlanPaths, opts.planPaths)
	statePaths := appendPaths(cfg.StatePaths, opts.statePaths)
	journalPaths := appendPaths(cfg.JournalPaths, opts.journalPaths)
	reviewSchedulePaths := appendPaths(cfg.ReviewSchedulePaths, opts.reviewSchedulePaths)
	notePaths := appendPaths(cfg.NotePaths, opts.notePaths)
	logDirs := appendPaths(cfg.LogDirs, opts.logDirs)
	applyEnvFallbacks(&planPaths, &statePaths, &journalPaths, &reviewSchedulePaths)

	return watch.Options{
		PlanPaths:           planPaths,
		StatePaths:          statePaths,
		JournalPaths:        journalPaths,
		ReviewSchedulePaths: reviewSchedulePaths,
		NotePaths:           notePaths,
		LogDirs:             logDirs,
	}, nil
}

func validateWatchOptions(options watch.Options) error {
	if len(options.PlanPaths) == 0 &&
		len(options.StatePaths) == 0 &&
		len(options.JournalPaths) == 0 &&
		len(options.ReviewSchedulePaths) == 0 &&
		len(options.NotePaths) == 0 &&
		len(options.LogDirs) == 0 {
		return fmt.Errorf("no observer inputs configured; pass --config, explicit --plan/--state/--journal/--review-schedule/--note flags, WAVEPLAN_PLAN/WAVEPLAN_STATE/WAVEPLAN_JOURNAL/WAVEPLAN_SCHED_REVIEW env vars, or --log-dir")
	}
	return nil
}

func runOnce(out io.Writer, options watch.Options, renderOptions ui.Options) error {
	snapshot, err := watch.PollOnce(options)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, ui.RenderText(snapshot, renderOptions))
	return err
}

func runLive(ctx context.Context, options watch.Options, renderOptions ui.Options, interval time.Duration) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	app := tview.NewApplication()
	pages := tview.NewPages()
	app.SetRoot(pages, true)

	var root *ui.Root
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch {
		case event.Key() == tcell.KeyEscape || (event.Key() == tcell.KeyRune && event.Rune() == 'q'):
			app.Stop()
			return nil
		case event.Key() == tcell.KeyTab && root != nil:
			focused := app.GetFocus() == root.Table()
			root.SetTableFocus(focused)

			if focused {
				app.SetFocus(root.Details())
			} else {
				app.SetFocus(root.Table())
			}
			return nil
		case event.Key() == tcell.KeyRune && event.Rune() == 'l' && root != nil:
			root.CycleLogMode()
			return nil
		}
		return event
	})

	errs := make(chan error, 1)
	go func() {
		watcher := watch.New(options, interval)
		errs <- watcher.Run(ctx, func(snapshot watch.Snapshot) error {
			app.QueueUpdateDraw(func() {
				if root == nil {
					prim := ui.BuildPrimitive(snapshot, renderOptions)
					root = prim.(*ui.Root)
					pages.AddAndSwitchToPage(livePageName, root, true)
				} else {
					root.Update(snapshot, renderOptions)
				}
			})
			return nil
		})
	}()

	if err := app.Run(); err != nil {
		cancel()
		return err
	}
	cancel()
	return <-errs
}

func appendPaths(base, extra []string) []string {
	combined := append([]string(nil), base...)
	combined = append(combined, extra...)
	return combined
}

func applyEnvFallbacks(planPaths, statePaths, journalPaths, reviewSchedulePaths *[]string) {
	if len(*planPaths) == 0 {
		appendEnvPath(planPaths, "WAVEPLAN_PLAN")
	}
	if len(*statePaths) == 0 {
		appendEnvPath(statePaths, "WAVEPLAN_STATE")
	}
	if len(*journalPaths) == 0 {
		appendEnvPath(journalPaths, "WAVEPLAN_JOURNAL")
	}
	if len(*reviewSchedulePaths) == 0 {
		appendEnvPath(reviewSchedulePaths, "WAVEPLAN_SCHED_REVIEW")
	}
}

func appendEnvPath(paths *[]string, envVar string) {
	if value := os.Getenv(envVar); value != "" {
		*paths = append(*paths, value)
	}
}

type stringArray []string

func (a *stringArray) String() string {
	return fmt.Sprint([]string(*a))
}

func (a *stringArray) Set(value string) error {
	*a = append(*a, value)
	return nil
}

func flagWasPassed(flags *flag.FlagSet, name string) bool {
	changed := false
	flags.Visit(func(flag *flag.Flag) {
		if flag.Name == name {
			changed = true
		}
	})
	return changed
}
