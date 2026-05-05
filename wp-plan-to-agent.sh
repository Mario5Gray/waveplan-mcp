#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  ./wp-plan-to-agent.sh --mode implement --target <codex|claude|opencode> --plan <plan.json> --agent <agent> [--dry-run]
  ./wp-plan-to-agent.sh --mode review --target <codex|claude|opencode> --plan <plan.json> --agent <owner_agent> --reviewer <reviewer_agent> [--dry-run]
  ./wp-plan-to-agent.sh --mode review_end --plan <plan.json> --task-id <Tn.m> [--review-note <text>] [--dry-run]
  ./wp-plan-to-agent.sh --mode fin --plan <plan.json> --task-id <Tn.m> [--git-sha <sha|DEFERRED>] [--dry-run]

Description:
  Unified wrapper for plan/task workflow actions.
  - implement/review delegate to ./wp-task-to-agent.sh
  - review_end/fin call waveplan-cli write actions directly
USAGE
}

MODE=""
TARGET=""
PLAN=""
AGENT=""
REVIEWER=""
TASK_ID=""
REVIEW_NOTE=""
GIT_SHA=""
DRY_RUN="0"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode) MODE="${2:-}"; shift 2 ;;
    --target) TARGET="${2:-}"; shift 2 ;;
    --plan) PLAN="${2:-}"; shift 2 ;;
    --agent) AGENT="${2:-}"; shift 2 ;;
    --reviewer) REVIEWER="${2:-}"; shift 2 ;;
    --task-id) TASK_ID="${2:-}"; shift 2 ;;
    --review-note) REVIEW_NOTE="${2:-}"; shift 2 ;;
    --git-sha) GIT_SHA="${2:-}"; shift 2 ;;
    --dry-run) DRY_RUN="1"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; usage; exit 2 ;;
  esac
done

if [[ -z "$MODE" || -z "$PLAN" ]]; then
  echo "Missing required args: --mode and --plan" >&2
  usage
  exit 2
fi

if [[ ! -f "$PLAN" ]]; then
  echo "Plan file not found: $PLAN" >&2
  exit 2
fi

if [[ "$MODE" == "implement" || "$MODE" == "review" ]]; then
  if [[ -z "$TARGET" || -z "$AGENT" ]]; then
    echo "Mode $MODE requires --target and --agent" >&2
    exit 2
  fi

  args=("./wp-task-to-agent.sh" "--target" "$TARGET" "--plan" "$PLAN" "--agent" "$AGENT" "--mode" "$MODE")
  if [[ "$MODE" == "review" ]]; then
    if [[ -n "$REVIEWER" ]]; then
      args+=("--reviewer" "$REVIEWER")
    fi
  fi
  if [[ "$DRY_RUN" == "1" ]]; then
    args+=("--dry-run")
  fi

  exec "${args[@]}"
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 not found" >&2
  exit 2
fi

if [[ ! -f "./waveplan-cli" ]]; then
  echo "waveplan-cli not found in current directory" >&2
  exit 2
fi

if [[ -z "$TASK_ID" ]]; then
  echo "Mode $MODE requires --task-id" >&2
  exit 2
fi

if [[ "$MODE" == "review_end" ]]; then
  cmd=("python3" "./waveplan-cli" "--plan" "$PLAN" "end_review" "$TASK_ID")
  if [[ -n "$REVIEW_NOTE" ]]; then
    cmd+=("$REVIEW_NOTE")
  fi
elif [[ "$MODE" == "fin" ]]; then
  cmd=("python3" "./waveplan-cli" "--plan" "$PLAN" "fin" "$TASK_ID")
  if [[ -n "$GIT_SHA" ]]; then
    cmd+=("$GIT_SHA")
  fi
else
  echo "Invalid --mode: $MODE" >&2
  usage
  exit 2
fi

if [[ "$DRY_RUN" == "1" ]]; then
  printf '%q ' "${cmd[@]}"
  printf '\n'
  exit 0
fi

"${cmd[@]}"
