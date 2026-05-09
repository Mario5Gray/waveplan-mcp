#!/usr/bin/env bash
# T3.1 — argv-only runner with stdout/stderr artifact capture.
set -euo pipefail
cd "$(dirname "$0")/../.."

go test ./internal/swim/... -run 'WritesStdoutAndStderrLogs|FailureWritesLogPathsAndExitCode|RetryUsesAttemptInLogFileNames' -count=1

echo "PASS: T3.1 runner logging"
