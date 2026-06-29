#!/usr/bin/env bash
# Benchmark script for the Python (FastAPI / uvicorn) implementation.
#
# Runs oha against POST /classify with two USE_DB configurations:
#   - USE_DB=false  :  pure inference, no DB write
#   - USE_DB=true   :  inference + synchronous DB commit (the bottleneck)
#
# For each configuration we sweep concurrency levels 1, 8, 32, 128 for 15 s each.
# USE_DB is read once at service startup, so we restart the service via a
# systemd drop-in override each time the value changes.
#
# Usage: bash bench/bench-python.sh

set -euo pipefail

# --- Config ------------------------------------------------------------------
readonly TARGET="http://localhost:8000"
readonly API_KEY="thepasswordyoudontwantotherstoknow" # see .env
readonly SERVICE="pixelwise-python"
readonly COMPETING_SERVICE="pixelwise-go"
readonly CONCURRENCY_LEVELS=(1 8 32 128)
readonly DURATION="15s"
readonly RESULTS_DIR="results"
readonly TIMESTAMP
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
readonly OUTFILE="${RESULTS_DIR}/python_${TIMESTAMP}.txt"

# --- Prerequisites ------------------------------------------------------------
if ! command -v oha &>/dev/null; then
  echo "Error: oha is not installed. Install with: cargo install oha" >&2
  exit 1
fi
if ! command -v curl &>/dev/null; then
  echo "Error: curl is not installed." >&2
  exit 1
fi

# --- Build payload (28x28 grid, all pixels = 200) -----------------------------
# All-200 produces a bright image that classifies successfully every run.
PAYLOAD=$(python3 -c "
import json
row = [200] * 28
pixels = [row for _ in range(28)]
print(json.dumps({'pixels': pixels}))
")

# --- systemd drop-in helpers --------------------------------------------------
# A drop-in file takes precedence over the EnvironmentFile, so setting
# Environment=USE_DB here overrides whatever value .env contains.

set_use_db() {
  local service="$1" value="$2"
  local override_dir="/etc/systemd/system/${service}.service.d"
  sudo mkdir -p "$override_dir"
  printf '[Service]\nEnvironment=USE_DB=%s\n' "$value" \
    | sudo tee "${override_dir}/bench-use-db.conf" >/dev/null
  sudo systemctl daemon-reload
}

remove_use_db_override() {
  local service="$1"
  sudo rm -f "/etc/systemd/system/${service}.service.d/bench-use-db.conf"
  sudo systemctl daemon-reload
}

# Remove the override on exit (even if the script errors out mid-run).
trap 'remove_use_db_override "$SERVICE"' EXIT

# --- Readiness check ----------------------------------------------------------
wait_ready() {
  echo "Waiting for ${SERVICE} to become ready..."
  for i in $(seq 1 15); do
    if curl -sf "${TARGET}/health" >/dev/null 2>&1; then
      echo "Service is ready (attempt ${i})"
      return 0
    fi
    sleep 1
  done
  echo "Error: ${SERVICE} failed to become ready after 15 s" >&2
  exit 1
}

# --- Stop competing service ---------------------------------------------------
echo "Stopping ${COMPETING_SERVICE} (if running)..."
sudo systemctl stop "${COMPETING_SERVICE}" 2>/dev/null || true

# --- Prepare results directory ------------------------------------------------
mkdir -p "${RESULTS_DIR}"
{
  echo "========================================================"
  echo " Python benchmark  —  $(date)"
  echo "========================================================"
  echo ""
} >> "${OUTFILE}"

# --- Benchmark loop -----------------------------------------------------------
for USE_DB_VALUE in false true; do
  echo ""
  echo "--- USE_DB=${USE_DB_VALUE} --------------------------------------------------"

  # Apply USE_DB override and start the service with the correct value.
  set_use_db "${SERVICE}" "${USE_DB_VALUE}"
  sudo systemctl start "${SERVICE}"
  wait_ready

  for C in "${CONCURRENCY_LEVELS[@]}"; do
    echo "Running: Python | c=${C} | USE_DB=${USE_DB_VALUE}"

    {
      echo ""
      echo "========================================================"
      echo " Python | concurrency=${C} | USE_DB=${USE_DB_VALUE}"
      echo "========================================================"
    } >> "${OUTFILE}"

    oha --no-tui \
        -c "${C}" \
        -z "${DURATION}" \
        -H "X-Api-Key: ${API_KEY}" \
        -H "Content-Type: application/json" \
        -d "${PAYLOAD}" \
        -m POST \
        "${TARGET}/classify" >> "${OUTFILE}" 2>&1
  done

  sudo systemctl stop "${SERVICE}"
done

# --- Done --------------------------------------------------------------------
echo ""
echo "Done. Results saved to ${OUTFILE}"
