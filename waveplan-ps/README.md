# waveplan-ps

`waveplan-ps` is a local observer for waveplan execution. It loads
explicitly-specified execution-wave plans, waveplan state sidecars, SWIM
journals, txtstore notes, and SWIM step logs, then renders a live terminal
view or a deterministic one-shot text snapshot.

## Build

```bash
go build -o waveplan-ps ./cmd/waveplan-ps
```

## Usage

Render one text snapshot and exit:

```bash
./waveplan-ps \
  --once \
  --plan ../docs/plans/2026-05-12-waveplan-ps-execution-waves.json \
  --state ../docs/plans/2026-05-12-waveplan-ps-execution-waves.json.state.json \
  --journal ../docs/plans/2026-05-12-waveplan-ps-execution-schedule.json.journal.json \
  --log-dir ../docs/plans/.waveplan
```

Run the live terminal observer:

```bash
./waveplan-ps \
  --config docs/waveplan-ps-config-example.yaml \
  --interval 2s
```

You can also supply the same explicit paths through environment variables:

```bash
export WAVEPLAN_PLAN=/abs/path/to/plan-execution-waves.json
export WAVEPLAN_STATE=/abs/path/to/plan-execution-state.json
export WAVEPLAN_JOURNAL=/abs/path/to/plan-execution-journal.json
./waveplan-ps --config docs/waveplan-ps-config-example.yaml --interval 2s
```

## Flags

| flag | default | description |
|---|---:|---|
| `--config` | empty | YAML config file. Explicit CLI paths are appended to config path lists. |
| `--once` | `false` | Render one text snapshot and exit. Without this flag, the live TUI runs until interrupted. |
| `--plan` | empty | Execution-waves plan path. Repeatable. Falls back to `WAVEPLAN_PLAN` when omitted. |
| `--state` | empty | Waveplan state sidecar path. Repeatable. Falls back to `WAVEPLAN_STATE` when omitted. |
| `--journal` | empty | SWIM journal sidecar path. Repeatable. Falls back to `WAVEPLAN_JOURNAL` when omitted. |
| `--note` | empty | Markdown txtstore note path. Repeatable. |
| `--log-dir` | empty | Directory root to recursively scan for SWIM log files. Repeatable. This is not a glob pattern. |
| `--interval` | `1s` | Live refresh interval, parsed as a Go duration such as `500ms`, `2s`, or `1m`. |
| `--tail-limit` | `10` | Maximum waveplan tail rows to render. Values less than or equal to zero render all rows. |
| `--journal-limit` | `10` | Maximum SWIM journal events to render. Values less than or equal to zero render all events. |
| `--expand-first-wave` | `true` | Initial display setting for the first wave. Passing the flag overrides config. |

## Config

Plan/state/journal/note lists are explicit file paths. Only `log_dirs` remains
directory-based because SWIM log discovery is fan-out over many step log files.

```yaml
log_dirs:
  - ../docs/plans/.waveplan
display:
  expand_first_wave: true
```

`display.expand_first_wave` defaults to `true` when omitted. Set it to `false`
in YAML when the first wave should start collapsed.

## Explicit Inputs

`waveplan-ps` no longer scans directories for plans, state files, journals, or
notes. Those are loaded only from explicit CLI flags, config path lists, or the
`WAVEPLAN_PLAN`, `WAVEPLAN_STATE`, and `WAVEPLAN_JOURNAL` environment
variables.

SWIM logs are still discovered under configured `log_dirs`. Valid SWIM log
filenames are:

Valid SWIM log actions are `implement`, `review`, `review_r<N>`, `fix`,
`fix_r<N>`, `end_review`, and `finish`. Log files are correlated to units by
the task-id segment embedded in `step_id`, so `T1.1` does not match `T11.1`.

Malformed log filenames under `log_dirs` fail the snapshot or live poll with a
parse error. Put unrelated logs outside configured log roots.
