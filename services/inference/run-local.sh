#!/usr/bin/env bash
set -euo pipefail

# Run the inference worker directly on the host to access the laptop camera.
# Docker Desktop for Mac cannot pass through host cameras to containers.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Detect if venv exists, create if not
if [ ! -d ".venv" ]; then
    echo "→ Creating Python virtual environment..."
    python3 -m venv .venv
fi

source .venv/bin/activate

# Install dependencies if needed
if ! python -c "import av, PIL" 2>/dev/null; then
    echo "→ Installing Python dependencies..."
    pip install -r requirements.txt
fi

export ORCHESTRATOR_URL="${ORCHESTRATOR_URL:-http://localhost:8080}"
export OUTPUT_DIR="${OUTPUT_DIR:-$SCRIPT_DIR/data/snapshots}"
export POLL_INTERVAL="${POLL_INTERVAL:-10}"
export SAMPLE_INTERVAL="${SAMPLE_INTERVAL:-1.0}"

echo "→ Starting inference worker..."
echo "  Orchestrator: $ORCHESTRATOR_URL"
echo "  Output dir:   $OUTPUT_DIR"
echo "  Sampling:     every ${SAMPLE_INTERVAL}s"
echo ""

python main.py
