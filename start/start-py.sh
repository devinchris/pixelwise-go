#!/bin/bash
# start/start-py.sh
# Stops the Go service and starts the Python (uvicorn) service.
# Run this before any Python benchmark.
set -euo pipefail

# Resolve the project root (one level above this script's directory).
PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "==> Stopping Go service (if running)..."
sudo systemctl stop pixelwise-go 2>/dev/null || true

# Verify the virtualenv exists so the service can actually start.
VENV="$PROJECT_DIR/.venv"
if [ ! -d "$VENV" ]; then
    echo "ERROR: .venv not found at $VENV."
    echo "       Run: python -m venv .venv && source .venv/bin/activate && pip install -r requirements.txt"
    exit 1
fi

echo "==> Starting Python service..."
sudo systemctl start pixelwise-python
echo "    pixelwise-python is running on :8000."
