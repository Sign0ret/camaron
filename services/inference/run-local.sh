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
if ! python -c "import av, PIL, ultralytics" 2>/dev/null; then
    echo "→ Installing Python dependencies..."
    pip install -r requirements.txt
fi

export ORCHESTRATOR_URL="${ORCHESTRATOR_URL:-http://localhost:8080}"
export OUTPUT_DIR="${OUTPUT_DIR:-$SCRIPT_DIR/data/snapshots}"
export POLL_INTERVAL="${POLL_INTERVAL:-10}"
export INFERENCE_INTERVAL="${INFERENCE_INTERVAL:-0.2}"   # 5 fps for YOLO
export CHUNK_INTERVAL="${CHUNK_INTERVAL:-1.0}"             # 1 fps for MP4 chunking

# Download YOLO weights once if missing
if [ ! -f "yolov8n.pt" ]; then
    echo "→ Downloading YOLOv8n weights..."
    python -c "from ultralytics import YOLO; YOLO('yolov8n.pt')"
fi

echo "→ Starting inference worker..."
echo "  Orchestrator:   $ORCHESTRATOR_URL"
echo "  Output dir:     $OUTPUT_DIR"
echo "  Inference fps:  ${INFERENCE_INTERVAL}s (≈ 5 fps)"
echo "  Chunk fps:      ${CHUNK_INTERVAL}s (≈ 1 fps)"
echo ""

python main.py
