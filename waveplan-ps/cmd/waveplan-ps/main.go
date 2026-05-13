package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/config"
	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/discovery"
	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/ui"
	"github.com/darkbit1001/Stability-Toys/waveplan-mcp/waveplan-ps/internal/watch"
	"github.com/rivo/tview"
)

const livePageName = "snapshot"

type cliOptions struct {
	configPath      string
	once            bool
	planFilters     []string
	planDirs        []string
	stateDirs       []string
	journalDirs     []string
	noteDirs        []string
	logDirs         []string
	interval        time.Duration
	tailLimit       int
	journalLimit    int
	logTailLines    int
	expandFirstWave bool
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
		expandFirstWave: true,
	}

	flags := flag.NewFlagSet("waveplan-ps", flag.ContinueOnError)
	flags.SetOutput(c.err)
	flags.StringVar(&opts.configPath, "config", "", "YAML config file")
	flags.BoolVar(&opts.once, "once", false, "render one snapshot and exit")
	flags.Var((*stringArray)(&opts.planFilters), "plan", "plan path or basename to display")
	flags.Var((*stringArray)(&opts.planDirs), "plan-dir", "directory to recursively scan for execution-waves plans")
	flags.Var((*stringArray)(&opts.stateDirs), "state-dir", "directory to recursively scan for waveplan state sidecars")
	flags.Var((*stringArray)(&opts.journalDirs), "journal-dir", "directory to recursively scan for SWIM journals")
	flags.Var((*stringArray)(&opts.noteDirs), "note-dir", "directory to recursively scan for txtstore notes")
	flags.Var((*stringArray)(&opts.logDirs), "log-dir", "directory to recursively scan for SWIM logs")
	flags.DurationVar(&opts.interval, "interval", time.Second, "live refresh interval")
	flags.IntVar(&opts.tailLimit, "tail-limit", 10, "maximum tail rows to render")
	flags.IntVar(&opts.journalLimit, "journal-limit", 10, "maximum journal events to render")
	flags.IntVar(&opts.logTailLines, "log-lines", 8, "lines of stdio log to show in active log panel")
	flags.BoolVar(&opts.expandFirstWave, "expand-first-wave", true, "expand the first wave initially")
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
		ExpandFirstWave: cfg.Display.ExpandFirstWave,
		TailLimit:       opts.tailLimit,
		JournalLimit:    opts.journalLimit,
		LogTailLines:    opts.logTailLines,
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

func buildWatchOptions(cfg config.Config, opts cliOptions) (watch.Options, error) {
	planDirs := appendPaths(cfg.PlanDirs, opts.planDirs)
	stateDirs := appendPaths(cfg.StateDirs, opts.stateDirs)
	journalDirs := appendPaths(cfg.JournalDirs, opts.journalDirs)
	noteDirs := appendPaths(cfg.NoteDirs, opts.noteDirs)
	logDirs := appendPaths(cfg.LogDirs, opts.logDirs)

	planPaths, err := discoverAllPaths(planDirs, discovery.DiscoverPlans)
	if err != nil {
		return watch.Options{}, err
	}
	statePaths, err := discoverAllPaths(stateDirs, discovery.DiscoverStates)
	if err != nil {
		return watch.Options{}, err
	}
	journalPaths, err := discoverAllPaths(journalDirs, discovery.DiscoverJournals)
	if err != nil {
		return watch.Options{}, err
	}
	notePaths, err := discoverAllPaths(noteDirs, discovery.DiscoverNotes)
	if err != nil {
		return watch.Options{}, err
	}

	if len(opts.planFilters) > 0 {
		planPaths = filterPlanPaths(planPaths, opts.planFilters)
		if len(planPaths) == 0 {
			planPaths = appendExplicitPlans(opts.planFilters)
		}
		statePaths = filterStatePaths(statePaths, planPaths)
		if len(statePaths) == 0 {
			statePaths = appendExistingStateSidecars(planPaths)
		}
	}

	return watch.Options{
		PlanPaths:    planPaths,
		StatePaths:   statePaths,
		JournalPaths: journalPaths,
		NotePaths:    notePaths,
		LogDirs:      logDirs,
	}, nil
}

func validateWatchOptions(options watch.Options) error {
	if len(options.PlanPaths) == 0 &&
		len(options.StatePaths) == 0 &&
		len(options.JournalPaths) == 0 &&
		len(options.NotePaths) == 0 &&
		len(options.LogDirs) == 0 {
		return fmt.Errorf("no discovery roots configured; pass --config or at least one of --plan-dir, --state-dir, --journal-dir, --note-dir, or --log-dir")
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
			if app.GetFocus() == root.Table() {
				app.SetFocus(root.Details())
			} else {
				app.SetFocus(root.Table())
			}
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

func discoverAllPaths(roots []string, discover func(string) ([]string, error)) ([]string, error) {
	var paths []string
	for _, root := range roots {
		if root == "" {
			continue
		}
		discovered, err := discover(root)
		if err != nil {
			return nil, err
		}
		paths = append(paths, discovered...)
	}
	sort.Strings(paths)
	return paths, nil
}

func filterPlanPaths(paths, filters []string) []string {
	filterSet := pathFilterSet(filters)
	var selected []string
	for _, path := range paths {
		if filterSet[filepath.Clean(path)] || filterSet[filepath.Base(path)] {
			selected = append(selected, path)
		}
	}
	return selected
}

func filterStatePaths(paths, planPaths []string) []string {
	wanted := map[string]bool{}
	for _, path := range planPaths {
		wanted[filepath.Base(path)+".state.json"] = true
	}
	var selected []string
	for _, path := range paths {
		if wanted[filepath.Base(path)] {
			selected = append(selected, path)
		}
	}
	return selected
}

func appendExplicitPlans(filters []string) []string {
	var paths []string
	for _, filter := range filters {
		info, err := os.Stat(filter)
		if err == nil && !info.IsDir() {
			paths = append(paths, filter)
		}
	}
	sort.Strings(paths)
	return paths
}

func appendExistingStateSidecars(planPaths []string) []string {
	var paths []string
	for _, planPath := range planPaths {
		statePath := planPath + ".state.json"
		info, err := os.Stat(statePath)
		if err == nil && !info.IsDir() {
			paths = append(paths, statePath)
		}
	}
	sort.Strings(paths)
	return paths
}

func pathFilterSet(filters []string) map[string]bool {
	set := map[string]bool{}
	for _, filter := range filters {
		if filter == "" {
			continue
		}
		set[filepath.Clean(filter)] = true
		set[filepath.Base(filter)] = true
	}
	return set
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
