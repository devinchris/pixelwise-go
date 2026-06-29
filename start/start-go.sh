#!/bin/bash
# start/start-go.sh
# Stops the Python service, compiles the Go binary, runs tests,
# and starts the Go service. Run this before any Go benchmark.
set -euo pipefail

# Resolve the project root (one level above this script's directory).
PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "==> Stopping Python service (if running)..."
sudo systemctl stop pixelwise-python 2>/dev/null || true

# Make sure models/weights.json exists; the Go binary needs it at startup.
# If it's missing, export it from the trained sklearn model first:
#   source .venv/bin/activate
#   python tools/export_weights.py --model models/digit_classifier_v1.pkl --out models/weights.json
if [ ! -f "$PROJECT_DIR/models/weights.json" ]; then
    echo "ERROR: models/weights.json not found."
    echo "       Run: python tools/export_weights.py --model models/digit_classifier_v1.pkl --out models/weights.json"
    exit 1
fi

echo "==> Compiling Go binary..."
cd "$PROJECT_DIR"
go build -o pixelwise-go .
echo "    Build OK."

echo "==> Running tests (must pass before service starts)..."
go test ./...
echo "    Tests OK."

echo "==> Starting Go service..."
sudo systemctl start pixelwise-go
echo "    pixelwise-go is running on :8000."
