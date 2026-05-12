# waveplan-ps

`waveplan-ps` is a local observer for waveplan execution. It loads
execution-wave plans, waveplan state sidecars, SWIM journals, txtstore notes,
and SWIM step logs, then renders a live terminal view or a deterministic
one-shot text snapshot.

## Build

```bash
go build -o waveplan-ps ./cmd/waveplan-ps
```

## Usage

Render one text snapshot and exit:

```bash
./waveplan-ps \
  --once \
  --plan-dir ../docs/plans \
  --state-dir ../docs/plans \
  --journal-dir ../docs/plans \
  --note-dir ../docs/agent_notes \
  --log-dir ../docs/plans/.waveplan
```

Run the live terminal observer:

```bash
./waveplan-ps \
  --config docs/waveplan-ps-config-example.yaml \
  --interval 2s
```

Limit the view to one plan by absolute path, relative path, or basename. When a
plan is selected, matching state sidecars are filtered to the selected plan.

```bash
./waveplan-ps --once \
  --plan-dir ../docs/plans \
  --state-dir ../docs/plans \
  --plan 2026-05-12-waveplan-ps-execution-waves.json
```

## Flags

| flag | default | description |
|---|---:|---|
| `--config` | empty | YAML config file. CLI directory flags are appended to config directories. |
| `--once` | `false` | Render one text snapshot and exit. Without this flag, the live TUI runs until interrupted. |
| `--plan` | empty | Plan path or basename to display. Repeat the flag to select multiple plans. |
| `--plan-dir` | empty | Directory root to recursively scan for `*-execution-waves.json` plans. Repeatable. |
| `--state-dir` | empty | Directory root to recursively scan for `*.state.json` sidecars. Repeatable. |
| `--journal-dir` | empty | Directory root to recursively scan for `*.journal.json` SWIM journals. Repeatable. |
| `--note-dir` | empty | Directory root to recursively scan for Markdown txtstore notes. Repeatable. |
| `--log-dir` | empty | Directory root to recursively scan for SWIM log files. Repeatable. This is not a glob pattern. |
| `--interval` | `1s` | Live refresh interval, parsed as a Go duration such as `500ms`, `2s`, or `1m`. |
| `--tail-limit` | `10` | Maximum waveplan tail rows to render. Values less than or equal to zero render all rows. |
| `--journal-limit` | `10` | Maximum SWIM journal events to render. Values less than or equal to zero render all events. |
| `--expand-first-wave` | `true` | Initial display setting for the first wave. Passing the flag overrides config. |

## Config

All directory lists are recursive scan roots. They are directories that
`waveplan-ps` walks with `filepath.WalkDir`; they are not shell globs.

```yaml
plan_dirs:
  - ../docs/plans
state_dirs:
  - ../docs/plans
journal_dirs:
  - ../docs/plans
note_dirs:
  - ../docs/agent_notes
log_dirs:
  - ../docs/plans/.waveplan
display:
  expand_first_wave: true
```

`display.expand_first_wave` defaults to `true` when omitted. Set it to `false`
in YAML when the first wave should start collapsed.

## Discovered Files

`waveplan-ps` recognizes files by filename:

| source | recognized names |
|---|---|
| plans | `*-execution-waves.json` |
| states | `*.state.json` |
| SWIM journals | `*.journal.json` |
| txtstore notes | `*.md` |
| SWIM logs | `S<seq>_<task-id>_<action>.<attempt>.stdout.log` and `.stderr.log` |

Valid SWIM log actions are `implement`, `review`, `review_r<N>`, `fix`,
`fix_r<N>`, `end_review`, and `finish`. Log files are correlated to units by
the task-id segment embedded in `step_id`, so `T1.1` does not match `T11.1`.

Malformed log filenames under `log_dirs` fail the snapshot or live poll with a
parse error. Put unrelated logs outside configured log roots.
