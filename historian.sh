#!/usr/bin/env bash
# historian - wrapper around copilot CLI that records history of agent calls

set -uo pipefail

HISTORICAL_PATH="${HISTORICAL_PATH:-/workspaces/workspace/ai/agentic-history}"

usage() {
    echo "Usage: $(basename "$0") [-e|-p] <prompt...>" >&2
    echo "  -p  Run: copilot -p <prompt>" >&2
    echo "  -e  Run: copilot --allow-all-tools --add-dir <pwd> -p <prompt>" >&2
    exit 1
}

[[ $# -lt 1 ]] && usage

MODE="$1"
shift

[[ "$MODE" != "-e" && "$MODE" != "-p" ]] && usage

CALL_PWD=$(pwd)
NOW_UNIX=$(date +%s)
NOW_HUMAN=$(date '+%Y-%m-%d %H:%M:%S')
NOW_DIR=$(date '+%Y-%m-%d_%H-%M-%S')

mkdir -p "$HISTORICAL_PATH"

# Determine group ID: reuse if any prior call was within 20 minutes
GROUP_ID="$NOW_UNIX"
CUTOFF=$((NOW_UNIX - 1200))

for dir in "$HISTORICAL_PATH"/*/; do
    [[ -d "$dir" ]] || continue
    dirname=$(basename "$dir")
    # Directory name ends with a 10-digit unix timestamp
    dir_ts=$(echo "$dirname" | grep -oE '[0-9]{10}$' || true)
    [[ -z "$dir_ts" ]] && continue
    (( dir_ts < CUTOFF )) && continue
    meta="${dir}/metadata.txt"
    [[ -f "$meta" ]] || continue
    existing_group=$(grep '^Group ID:' "$meta" 2>/dev/null | sed 's/^Group ID: //' | head -1)
    if [[ -n "$existing_group" ]]; then
        GROUP_ID="$existing_group"
        break
    fi
done

HISTORY_DIR="${HISTORICAL_PATH}/${NOW_DIR}_${NOW_UNIX}"
mkdir -p "$HISTORY_DIR"

# Gather git info
GIT_REPO="N/A"
GIT_BRANCH="N/A"
GIT_DIRTY="N/A"
if git -C "$CALL_PWD" rev-parse --is-inside-work-tree &>/dev/null; then
    GIT_REPO=$(git -C "$CALL_PWD" remote get-url origin 2>/dev/null \
               || git -C "$CALL_PWD" rev-parse --show-toplevel 2>/dev/null \
               || echo "N/A")
    GIT_BRANCH=$(git -C "$CALL_PWD" rev-parse --abbrev-ref HEAD 2>/dev/null || echo "N/A")
    if [[ -n "$(git -C "$CALL_PWD" status --porcelain 2>/dev/null)" ]]; then
        GIT_DIRTY="true"
    else
        GIT_DIRTY="false"
    fi
fi

START_TIME=$(date +%s)

# Run copilot, streaming output to terminal and capturing to file
COPILOT_EXIT=0
if [[ "$MODE" == "-p" ]]; then
    copilot --add-dir "$HISTORICAL_PATH" -p "$@" 2>&1 | tee "$HISTORY_DIR/raw_output.txt"
    COPILOT_EXIT="${PIPESTATUS[0]}"
else
    # TODO - consider whether we want execute mode agent to have access to historical files by default
    copilot --allow-all-tools --add-dir "$HISTORICAL_PATH" --add-dir "$CALL_PWD" -p "$@" 2>&1 | tee "$HISTORY_DIR/raw_output.txt"
    COPILOT_EXIT="${PIPESTATUS[0]}"
fi

END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

# Format duration
if (( DURATION >= 3600 )); then
    DURATION_STR="$((DURATION / 3600))h $(( (DURATION % 3600) / 60 ))m $((DURATION % 60))s"
elif (( DURATION >= 60 )); then
    DURATION_STR="$((DURATION / 60))m $((DURATION % 60))s"
else
    DURATION_STR="${DURATION}s"
fi

cat > "$HISTORY_DIR/metadata.txt" << EOF
Date/Time:  $NOW_HUMAN
Prompt:     $*
PWD:        $CALL_PWD
Git Repo:   $GIT_REPO
Git Branch: $GIT_BRANCH
Git Dirty:  $GIT_DIRTY
Duration:   $DURATION_STR
Group ID:   $GROUP_ID
EOF

echo "" >&2
echo "History saved to: $HISTORY_DIR" >&2

exit "$COPILOT_EXIT"
