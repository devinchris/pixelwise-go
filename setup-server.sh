#!/bin/bash
# Sets up a blank Ubuntu VM to run the PixelWise benchmark.
# Idempotent: safe to run multiple times.
#
# Assumptions:
#   - Standard Ubuntu install with one user already created.
#   - Project is located at /opt/pixelwise-go.
#   - .env is present at /opt/pixelwise-go/.env before running.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CURRENT_USER="$(whoami)"

sudo apt update
sudo apt install -y git python3 python3-pip python3-venv curl postgresql nginx

# Create venv if it doesnt exist yet
if [ ! -d "$SCRIPT_DIR/.venv" ]; then
    python3 -m venv "$SCRIPT_DIR/.venv"
fi

# Activate venv and install pinned dependencies
if [ -d "$SCRIPT_DIR/.venv" ] && [ -f "$SCRIPT_DIR/requirements.txt" ]; then
    source "$SCRIPT_DIR/.venv/bin/activate"
    pip install -r "$SCRIPT_DIR/requirements.txt"
fi

# Pull the model
if [ -f "$SCRIPT_DIR/.env" ]; then
    set -a; source "$SCRIPT_DIR/.env"; set +a
    if [ -n "${MODEL_REPO:-}" ] && [ -n "${MODEL_VERSION:-}" ]; then
        mkdir -p "$SCRIPT_DIR/models"
        rm -rf /tmp/pixelwise-model
        git clone --depth 1 --branch "$MODEL_VERSION" "$MODEL_REPO" /tmp/pixelwise-model
        cp /tmp/pixelwise-model/*.pkl "$SCRIPT_DIR/models/"
        cp /tmp/pixelwise-model/MODELCARD.md "$SCRIPT_DIR/models/"
        rm -rf /tmp/pixelwise-model
    fi
fi

# -- Export sklearn weights -> weights.json (required by Go inference) ---------
# Use MODEL_PATH from .env if set; fall back to the versioned default.
if [ -f "$SCRIPT_DIR/.env" ]; then
    set -a; source "$SCRIPT_DIR/.env"; set +a
fi
MODEL_PKL="${MODEL_PATH:-models/digit_classifier_v1.pkl}"
if [ -f "$SCRIPT_DIR/$MODEL_PKL" ]; then
    echo "Exporting model weights to models/weights.json..."
    (
        cd "$SCRIPT_DIR"
        source .venv/bin/activate
        python tools/export_weights.py \
            --model "$MODEL_PKL" \
            --out models/weights.json
    )
fi

# -- Go ------------------------------------------------------------------------
if ! command -v go &>/dev/null && [ ! -x /usr/local/go/bin/go ]; then
    ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
    curl -fsSL "https://go.dev/dl/go1.24.4.linux-${ARCH}.tar.gz" \
        | sudo tar -C /usr/local -xz
    echo 'export PATH=$PATH:/usr/local/go/bin' | sudo tee /etc/profile.d/go.sh
fi
export PATH="$PATH:/usr/local/go/bin"

# Build the Go binary
if [ -f "$SCRIPT_DIR/go.mod" ]; then
    (cd "$SCRIPT_DIR" && go build -o pixelwise-go .)
    # Smoke-test: verify the binary is executable and starts without panicking.
    BINARY="$SCRIPT_DIR/pixelwise-go"
    if [ ! -x "$BINARY" ]; then
        echo "ERROR: go build succeeded but binary not found or not executable: $BINARY" >&2
        exit 1
    fi
    echo "Go binary smoke-test..."
    (
        cd "$SCRIPT_DIR"
        # Source .env so the binary has DB_PASSWORD etc. available
        # skip actual DB writes
        set -a; source .env; set +a
        USE_DB=false "$BINARY" &
        SERVER_PID=$!
        # probe /health
        for i in $(seq 1 10); do
            sleep 0.5
            if curl -sf -o /dev/null http://127.0.0.1:8000/health; then
                kill "$SERVER_PID" 2>/dev/null
                echo "Go binary smoke-test passed."
                exit 0
            fi
        done
        kill "$SERVER_PID" 2>/dev/null
        echo "ERROR: Go binary did not respond on /health within 5 s." >&2
        exit 1
    )
fi

# -- install oha (HTTP load tester used by the bench scripts) ----------------------------
if ! command -v oha &>/dev/null; then
    echo "Installing oha..."
    ARCH=$(uname -m)   # oha release names use the raw arch string
    OHA_TAG=$(curl -fsSL https://api.github.com/repos/hatoo/oha/releases/latest \
        | python3 -c "import sys,json; print(json.load(sys.stdin)['tag_name'])")
    curl -fsSL \
        "https://github.com/hatoo/oha/releases/download/${OHA_TAG}/oha-linux-${ARCH}" \
        -o /tmp/oha
    sudo install -m 755 /tmp/oha /usr/local/bin/oha
    rm -f /tmp/oha
fi

# -- Results directory (written to by bench scripts) -----------------------------
mkdir -p "$SCRIPT_DIR/results"

# -- PostgreSQL: user + database ------------------------------------------------
if command -v psql &>/dev/null && [ -f "$SCRIPT_DIR/.env" ]; then
    set -a; source "$SCRIPT_DIR/.env"; set +a
    sudo -u postgres psql -tAc \
        "SELECT 1 FROM pg_roles WHERE rolname='pixelwise'" \
        | grep -q 1 || \
    sudo -u postgres psql -c \
        "CREATE USER pixelwise WITH PASSWORD '$DB_PASSWORD';"
    sudo -u postgres psql -tAc \
        "SELECT 1 FROM pg_database WHERE datname='pixelwise'" \
        | grep -q 1 || \
    sudo -u postgres createdb -O pixelwise pixelwise
fi

# -- DB schema (predictions table) ---------------------------------------------
# Run before starting the services to avoid a startup panic on a missing table.
if [ -f "$SCRIPT_DIR/init_db.py" ]; then
    (
        cd "$SCRIPT_DIR"
        source .venv/bin/activate
        python init_db.py
    )
fi

# -- Systemd service units ------------------------------------------------------
# The source files contain User=produser; substitute the actual OS user on install
# so the service runs as whoever set up the VM.
# Python is the production default (enabled); Go is disabled at boot.
if command -v systemctl &>/dev/null; then
    sudo sed "s/User=produser/User=${CURRENT_USER}/" \
        "$SCRIPT_DIR/deploy/pixelwise-python.service" \
        | sudo tee /etc/systemd/system/pixelwise-python.service >/dev/null
    sudo sed "s/User=produser/User=${CURRENT_USER}/" \
        "$SCRIPT_DIR/deploy/pixelwise-go.service" \
        | sudo tee /etc/systemd/system/pixelwise-go.service >/dev/null
    sudo systemctl daemon-reload
    sudo systemctl enable pixelwise-python
    sudo systemctl disable pixelwise-go
    sudo systemctl start pixelwise-python
    sudo systemctl status pixelwise-python --no-pager
fi

# -- Nginx + frontend -----------------------------------------------------------
if [ -f "$SCRIPT_DIR/deploy/pixelwise.nginx" ] && command -v nginx &>/dev/null; then
    sudo mkdir -p /var/www/pixelwise
    sudo cp -r "$SCRIPT_DIR/frontend/"* /var/www/pixelwise/

    # Substitute the API key placeholder in app.js (idempotent: no-op if already done).
    KEY=$(grep ^SECRET_API_KEY "$SCRIPT_DIR/.env" | cut -d= -f2)
    sudo sed -i "s/REPLACE_ME/$KEY/" /var/www/pixelwise/app.js

    sudo cp "$SCRIPT_DIR/deploy/pixelwise.nginx" \
        /etc/nginx/sites-available/pixelwise
    sudo ln -sf /etc/nginx/sites-available/pixelwise \
        /etc/nginx/sites-enabled/pixelwise
    sudo rm -f /etc/nginx/sites-enabled/default
    sudo nginx -t && sudo systemctl reload nginx
fi

# -- Sudoers --------------------------------------------------------------------
# Single consolidated write covering:
#   bench scripts  - start/stop/daemon-reload + USE_DB drop-in management
#   auto-deploy    - restart pixelwise-python
if command -v systemctl &>/dev/null; then
    sudo tee /etc/sudoers.d/pixelwise >/dev/null <<EOF
# Pixelwise benchmark: service lifecycle
${CURRENT_USER} ALL=(root) NOPASSWD: /usr/bin/systemctl stop pixelwise-python
${CURRENT_USER} ALL=(root) NOPASSWD: /usr/bin/systemctl stop pixelwise-go
${CURRENT_USER} ALL=(root) NOPASSWD: /usr/bin/systemctl start pixelwise-python
${CURRENT_USER} ALL=(root) NOPASSWD: /usr/bin/systemctl start pixelwise-go
${CURRENT_USER} ALL=(root) NOPASSWD: /usr/bin/systemctl restart pixelwise-python
${CURRENT_USER} ALL=(root) NOPASSWD: /usr/bin/systemctl restart pixelwise-go
${CURRENT_USER} ALL=(root) NOPASSWD: /usr/bin/systemctl daemon-reload
# Pixelwise benchmark: USE_DB drop-in override management
${CURRENT_USER} ALL=(root) NOPASSWD: /usr/bin/mkdir -p /etc/systemd/system/pixelwise-python.service.d
${CURRENT_USER} ALL=(root) NOPASSWD: /usr/bin/mkdir -p /etc/systemd/system/pixelwise-go.service.d
${CURRENT_USER} ALL=(root) NOPASSWD: /usr/bin/tee /etc/systemd/system/pixelwise-python.service.d/bench-use-db.conf
${CURRENT_USER} ALL=(root) NOPASSWD: /usr/bin/tee /etc/systemd/system/pixelwise-go.service.d/bench-use-db.conf
${CURRENT_USER} ALL=(root) NOPASSWD: /usr/bin/rm -f /etc/systemd/system/pixelwise-python.service.d/bench-use-db.conf
${CURRENT_USER} ALL=(root) NOPASSWD: /usr/bin/rm -f /etc/systemd/system/pixelwise-go.service.d/bench-use-db.conf
EOF
    sudo chmod 0440 /etc/sudoers.d/pixelwise
    sudo visudo -cf /etc/sudoers.d/pixelwise
fi

# -- Auto-deploy timer (only if deploy units are present) -------------------------
# User=produser is substituted to CURRENT_USER, same as the main service units.
if [ -f "$SCRIPT_DIR/deploy/systemd/pixelwise-deploy.timer" ] \
   && command -v systemctl &>/dev/null; then
    sudo sed "s/User=produser/User=${CURRENT_USER}/" \
        "$SCRIPT_DIR/deploy/systemd/pixelwise-deploy.service" \
        | sudo tee /etc/systemd/system/pixelwise-deploy.service >/dev/null
    sudo cp "$SCRIPT_DIR/deploy/systemd/pixelwise-deploy.timer" \
        /etc/systemd/system/pixelwise-deploy.timer
    sudo systemctl daemon-reload
    sudo systemctl enable --now pixelwise-deploy.timer
fi