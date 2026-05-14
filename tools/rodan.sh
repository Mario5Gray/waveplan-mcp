#!/usr/bin/env bash
#
# opencode-jang.sh — Wrapper for opencode with configurable model
#
# Designed for use with waveplan-mcp's wp-plan-to-agent.sh and wp-task-to-agent.sh
# Default model: vmlx/Qwen3.6-27B-JANG_4M-CRACK (served at http://localhost:8000/v1)
#
# Usage:
#   ./opencode-jang.sh [--model <provider/model>] [--serve|--run|--attach] [message...]
#   ./opencode-jang.sh wp-plan <plan.json> --agent <name> [--model <provider/model>]
#   ./opencode-jang.sh wp-task <plan.json> --agent <name> --mode implement|review|fix [--model <provider/model>]
#

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
export OPENCODE_MODEL="${OPENCODE_MODEL:-vmlx/Qwen3.6-27B-JANG_4M-CRACK}"
export OPENCODE_SERVER_HOST="${OPENCODE_SERVER_HOST:-127.0.0.1}"
export OPENCODE_SERVER_PORT="${OPENCODE_SERVER_PORT:-4096}"
export OPENCODE_ATTACH_URL="${OPENCODE_ATTACH_URL:-http://127.0.0.1:4096}"
export OPENCODE_AUTO_SERVE="${OPENCODE_AUTO_SERVE:-1}"

# Optimal generation parameters — tuned for code generation, planning, and structured output
# Override per-model via environment: JANG_TEMPERATURE, JANG_TOP_K, JANG_TOP_P, etc.
readonly JANG_TEMPERATURE="${JANG_TEMPERATURE:-1.0}"
readonly JANG_TOP_K="${JANG_TOP_K:-20}"
readonly JANG_TOP_P="${JANG_TOP_P:-0.0}"
readonly JANG_MIN_P="${JANG_MIN_P:-0.0}"
readonly JANG_MAX_TOKENS="${JANG_MAX_TOKENS:-16384}"
readonly JANG_REPETITION_PENALTY="${JANG_REPETITION_PENALTY:-1.0}"
readonly JANG_FREQUENCY_PENALTY=0.0
readonly JANG_PRESENCE_PENALTY=0.0

# Model card (default: vmlx/Qwen3.6-27B-JANG_4M-CRACK):
#   - temperature=1.0: Balanced creativity for code generation
#   - top_k=20: Sharp token selection, reduces incoherent outputs
#   - top_p=0.0: Disabled (top_k dominates sampling)
#   - max_tokens=16384: Full context window for complex tasks
#   - repetition_penalty=1.0: No penalty (code benefits from repetition)
#   - frequency_penalty=0.0: Neutral (prevents over-penalizing common patterns)

usage() {
  cat <<'USAGE'
Usage:
  opencode-jang.sh [--model <provider/model>] [--serve|--run|--attach] [message...]
  opencode-jang.sh wp-plan <plan.json> --agent <name> [--model <provider/model>] [--dry-run]
  opencode-jang.sh wp-task <plan.json> --agent <name> [--mode implement|review|fix] [--model <provider/model>] [--dry-run]

Description:
  Wrapper for opencode with configurable model.
  Optimized for waveplan-mcp task dispatch via wp-plan-to-agent.sh and wp-task-to-agent.sh.

Options:
  --model <provider/model>  Model to use (default: vmlx/Qwen3.6-27B-JANG_4M-CRACK)
  --serve                   Start opencode server with model (default)
  --run                     Run opencode with a message and exit
  --attach                  Attach to running opencode server
  wp-plan                   Dispatch plan task via wp-plan-to-agent.sh
  wp-task                   Dispatch task via wp-task-to-agent.sh

Environment:
  OPENCODE_MODEL            Model to use (default: vmlx/Qwen3.6-27B-JANG_4M-CRACK)
  OPENCODE_SERVER_HOST      Server host (default: 127.0.0.1)
  OPENCODE_SERVER_PORT      Server port (default: 4096)
  OPENCODE_AUTO_SERVE       Auto-start server (default: 1)
  JANG_TEMPERATURE          Generation temperature (default: 1.0)
  JANG_TOP_K                Top-k sampling (default: 20)
  JANG_TOP_P                Top-p sampling (default: 0.0)
  JANG_MAX_TOKENS           Max output tokens (default: 16384)
  WAVEPLAN_CLI_BIN          Path to waveplan-cli (optional)
  WP_TASK_TO_AGENT_BIN      Path to wp-task-to-agent.sh (optional)

Examples:
  # Start server with default JANG model
  ./opencode-jang.sh --serve

  # Start server with a different model
  ./opencode-jang.sh --model tabby/Qwen3.6-35B-A3B-EXL3-4.5bpw --serve

  # Run a one-shot task with custom model
  ./opencode-jang.sh --model vmlx/Qwen3.6-27B-JANG_4M-CRACK --run "Implement Task 1.1"

  # Dispatch plan task to agent
  ./opencode-jang.sh wp-plan ./plans/2026-05-13-cloudflare-ai-worker.json --agent Theta

  # Dispatch task in review mode with custom model
  ./opencode-jang.sh wp-task ./plans/2026-05-13-cloudflare-ai-worker.json --agent Sigma --mode review --model lmstudio/qwen/qwen3-30b-a3b-2507
USAGE
}

opencode_server_healthy() {
  local base_url="$1"
  python3 - "$base_url" <<'PY'
import json
import sys
import urllib.request

base = sys.argv[1].rstrip("/")
url = f"{base}/global/health"
req = urllib.request.Request(url, method="GET")
try:
    with urllib.request.urlopen(req, timeout=2.0) as r:
        body = r.read().decode("utf-8", "replace")
    data = json.loads(body)
    ok = bool(data.get("healthy")) if isinstance(data, dict) else False
    sys.exit(0 if ok else 1)
except Exception:
    sys.exit(1)
PY
}

maybe_start_opencode_server() {
  if [[ "${OPENCODE_AUTO_SERVE:-0}" != "1" ]]; then
    return 1
  fi

  local host="${OPENCODE_SERVER_HOST}"
  local port="${OPENCODE_SERVER_PORT}"
  local url="${OPENCODE_ATTACH_URL}"

  # Check if already running
  if opencode_server_healthy "$url" 2>/dev/null; then
    return 0
  fi

  echo "→ Starting opencode server with ${OPENCODE_MODEL}..."
  echo "  Model: ${OPENCODE_MODEL}"
  echo "  Parameters: temp=${JANG_TEMPERATURE}, top_k=${JANG_TOP_K}, top_p=${JANG_TOP_P}, max_tokens=${JANG_MAX_TOKENS}"
  echo "  Server: ${host}:${port}"

  nohup opencode serve \
    --hostname "$host" \
    --port "$port" \
    --model "$OPENCODE_MODEL" \
    >/tmp/opencode-jang-serve.log 2>&1 &

  local serve_pid=$!
  echo "  PID: ${serve_pid}"

  # Wait for server to be healthy
  for i in $(seq 1 40); do
    sleep 0.25
    if opencode_server_healthy "$url" 2>/dev/null; then
      echo "→ Server healthy (${i} iterations)"
      return 0
    fi
  done

  echo "ERROR: Server failed to start. Check /tmp/opencode-jang-serve.log" >&2
  return 1
}

cmd_serve() {
  local model="${1:-}"
  [[ -z "$model" ]] && model="$OPENCODE_MODEL"

  echo "Starting opencode server with ${model}"
  echo "========================================================="
  echo "Model:      ${model}"
  echo "Parameters: temp=${JANG_TEMPERATURE}, top_k=${JANG_TOP_K}, top_p=${JANG_TOP_P}"
  echo "           max_tokens=${JANG_MAX_TOKENS}, rep_penalty=${JANG_REPETITION_PENALTY}"
  echo "Server:     ${OPENCODE_SERVER_HOST}:${OPENCODE_SERVER_PORT}"
  echo "Attach URL: ${OPENCODE_ATTACH_URL}"
  echo ""
  echo "Usage with waveplan-mcp:"
  echo "  ./opencode-jang.sh wp-plan <plan.json> --agent <name>"
  echo "  ./opencode-jang.sh wp-task <plan.json> --agent <name> --mode implement"
  echo ""

  maybe_start_opencode_server

  # Run interactive
  opencode run \
    --attach "${OPENCODE_ATTACH_URL}" \
    --model "${model}" \
    --format default
}

cmd_run() {
  local model="${1:-}"
  shift
  local message="$*"
  if [[ -z "$message" ]]; then
    echo "Error: --run requires a message" >&2
    usage
    exit 2
  fi
  [[ -z "$model" ]] && model="$OPENCODE_MODEL"

  maybe_start_opencode_server

  opencode run \
    --attach "${OPENCODE_ATTACH_URL}" \
    --model "${model}" \
    --format json \
    "$message"
}

cmd_attach() {
  local model="${1:-}"
  [[ -z "$model" ]] && model="$OPENCODE_MODEL"

  maybe_start_opencode_server || true

  opencode run \
    --attach "${OPENCODE_ATTACH_URL}" \
    --model "${model}" \
    --format default
}

cmd_wp_plan() {
  local model="${1:-}"
  local plan="${2:-}"
  local agent="${3:-}"
  local dry_run="${4:-0}"

  if [[ ! -f "$plan" ]]; then
    echo "Plan file not found: $plan" >&2
    exit 2
  fi

  [[ -z "$agent" ]] && { echo "Error: --agent is required" >&2; exit 2; }

  maybe_start_opencode_server || true

  local target="opencode"
  local env_vars=(
    "OPENCODE_MODEL=${model}"
    "OPENCODE_ATTACH_URL=${OPENCODE_ATTACH_URL}"
    "OPENCODE_AUTO_SERVE=0"
  )

  local cmd=(
    "env"
    "${env_vars[@]}"
    "bash"
    "${SCRIPT_DIR}/wp-plan-to-agent.sh"
    "--mode" "implement"
    "--target" "$target"
    "--plan" "$plan"
    "--agent" "$agent"
  )

  if [[ "$dry_run" == "1" ]]; then
    cmd+=("--dry-run")
  fi

  echo "→ Dispatching plan task via wp-plan-to-agent.sh"
  echo "  Model: ${model}"
  echo "  Plan:  ${plan}"
  echo "  Agent: ${agent}"
  echo "  Command: ${cmd[*]}"
  echo ""

  "${cmd[@]}"
}

cmd_wp_task() {
  local model="${1:-}"
  local plan="${2:-}"
  local agent="${3:-}"
  local task_mode="${4:-implement}"
  local dry_run="${5:-0}"

  if [[ ! -f "$plan" ]]; then
    echo "Plan file not found: $plan" >&2
    exit 2
  fi

  [[ -z "$agent" ]] && { echo "Error: --agent is required" >&2; exit 2; }

  case "$task_mode" in
    implement|review|fix) ;;
    *)
      echo "Invalid mode: $task_mode (must be implement|review|fix)" >&2
      exit 2
      ;;
  esac

  maybe_start_opencode_server || true

  local target="opencode"
  local env_vars=(
    "OPENCODE_MODEL=${model}"
    "OPENCODE_ATTACH_URL=${OPENCODE_ATTACH_URL}"
    "OPENCODE_AUTO_SERVE=0"
  )

  local cmd=(
    "env"
    "${env_vars[@]}"
    "bash"
    "${SCRIPT_DIR}/wp-task-to-agent.sh"
    "--target" "$target"
    "--plan" "$plan"
    "--agent" "$agent"
    "--mode" "$task_mode"
  )

  if [[ "$dry_run" == "1" ]]; then
    cmd+=("--dry-run")
  fi

  echo "→ Dispatching task via wp-task-to-agent.sh"
  echo "  Model: ${model}"
  echo "  Plan:  ${plan}"
  echo "  Agent: ${agent}"
  echo "  Mode:  ${task_mode}"
  echo "  Command: ${cmd[*]}"
  echo ""

  "${cmd[@]}"
}

# Main dispatch
if [[ $# -eq 0 ]]; then
  cmd_serve "${GLOBAL_MODEL:-}"
  exit 0
fi

MODE="$1"
shift

case "$MODE" in
  --serve|-s)
    cmd_serve "${GLOBAL_MODEL:-}"
    ;;
  --run|-r)
    cmd_run "${GLOBAL_MODEL:-}" "$@"
    ;;
  --attach|-a)
    cmd_attach "${GLOBAL_MODEL:-}"
    ;;
  --help|-h)
    usage
    exit 0
    ;;
  wp-plan)
    if [[ $# -lt 1 ]]; then
      echo "Error: wp-plan requires <plan.json> and --agent <name>" >&2
      usage
      exit 2
    fi
    PLAN="$1"
    AGENT=""
    DRY_RUN="0"
    WP_MODEL="${GLOBAL_MODEL:-}"
    i=1
    while [[ $i -lt $# ]]; do
      case "${@:$i:1}" in
        --agent)
          if [[ $((i+1)) -le $# ]]; then
            AGENT="${@:$((i+1)):1}"
            ((i+=2))
          else
            echo "Error: --agent requires a value" >&2
            exit 2
          fi
          ;;
        --model)
          if [[ $((i+1)) -le $# ]]; then
            WP_MODEL="${@:$((i+1)):1}"
            ((i+=2))
          else
            echo "Error: --model requires a value" >&2
            exit 2
          fi
          ;;
        --dry-run) DRY_RUN="1"; ((i++)) ;;
        *) ((i++)) ;;
      esac
    done
    if [[ -z "$AGENT" ]]; then
      echo "Error: --agent is required" >&2
      exit 2
    fi
    cmd_wp_plan "$WP_MODEL" "$PLAN" "$AGENT" "$DRY_RUN"
    ;;
  wp-task)
    if [[ $# -lt 1 ]]; then
      echo "Error: wp-task requires <plan.json> and --agent <name>" >&2
      usage
      exit 2
    fi
    PLAN="$1"
    AGENT=""
    TASK_MODE="implement"
    DRY_RUN="0"
    WP_MODEL="${GLOBAL_MODEL:-}"
    i=1
    while [[ $i -lt $# ]]; do
      case "${@:$i:1}" in
        --agent)
          if [[ $((i+1)) -le $# ]]; then
            AGENT="${@:$((i+1)):1}"
            ((i+=2))
          else
            echo "Error: --agent requires a value" >&2
            exit 2
          fi
          ;;
        --model)
          if [[ $((i+1)) -le $# ]]; then
            WP_MODEL="${@:$((i+1)):1}"
            ((i+=2))
          else
            echo "Error: --model requires a value" >&2
            exit 2
          fi
          ;;
        --mode)
          if [[ $((i+1)) -le $# ]]; then
            TASK_MODE="${@:$((i+1)):1}"
            ((i+=2))
          else
            echo "Error: --mode requires a value" >&2
            exit 2
          fi
          ;;
        --dry-run) DRY_RUN="1"; ((i++)) ;;
        *) ((i++)) ;;
      esac
    done
    if [[ -z "$AGENT" ]]; then
      echo "Error: --agent is required" >&2
      exit 2
    fi
    cmd_wp_task "$WP_MODEL" "$PLAN" "$AGENT" "$TASK_MODE" "$DRY_RUN"
    ;;
  *)
    echo "Unknown command: $MODE" >&2
    usage
    exit 2
    ;;
esac
