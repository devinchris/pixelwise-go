#!/bin/bash

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

sudo apt update
sudo apt install -y git python3 python3-pip python3-venv curl postgresql nginx


# Activate venv and install pinned dependencies
if [ -d "$SCRIPT_DIR/.venv" ] && [ -f "$SCRIPT_DIR/requirements.txt" ]; then
    source "$SCRIPT_DIR/.venv/bin/activate"
    pip install -r "$SCRIPT_DIR/requirements.txt"
fi


# Pull the model
if [ -f .env ]; then
	set -a; source .env; set +a
	if [ -n "${MODEL_REPO:-}" ] &&  [ -n "${MODEL_VERSION:-}" ]; then
		mkdir -p models/
		rm -rf /tmp/pixelwise-model
		git clone --depth 1 --branch "$MODEL_VERSION" "$MODEL_REPO" /tmp/pixelwise-model
		cp /tmp/pixelwise-model/*.pkl models/
		cp /tmp/pixelwise-model/MODELCARD.md models/
		rm -rf /tmp/pixelwise-model
	fi
fi

# Install systemd unit
if [ -f deploy/pixelwise.service ] && command -v systemctl > /dev/null 2>&1 && id produser > /dev/null 2>&1; then
	sudo cp deploy/pixelwise.service /etc/systemd/system/pixelwise.service
	sudo systemctl daemon-reload
	sudo systemctl enable pixelwise
	sudo systemctl start pixelwise
	sudo systemctl status pixelwise --no-pager
fi

# Provision the postgresql database
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if command -v psql >/dev/null 2>&1 && [ -f "$SCRIPT_DIR/.env" ]; then
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

# Initialise the predictions table on every VM via Alchemy
if [ -f "$SCRIPT_DIR/init_db.py" ] && [ -d "$SCRIPT_DIR/.venv" ]; then
	(cd "$SCRIPT_DIR" && source .venv/bin/activate && python init_db.py)
fi

# Install Nginx site and deploy the frontend on prod
if [ -f deploy/pixelwise.nginx ] && \
   command -v nginx >/dev/null 2>&1 && \
   id produser >/dev/null 2>&1; then

    # Deploy the frontend files.
    sudo mkdir -p /var/www/pixelwise
    sudo cp -r frontend/* /var/www/pixelwise/

    # Substitute the API key into app.js.
    KEY=$(grep ^SECRET_API_KEY /opt/pixelwise/.env \
        | cut -d= -f2)
    sudo sed -i \
        "s/REPLACE_ME/$KEY/" \
        /var/www/pixelwise/app.js

    # Install the site config.
    sudo cp deploy/pixelwise.nginx \
        /etc/nginx/sites-available/pixelwise
    sudo ln -sf /etc/nginx/sites-available/pixelwise \
        /etc/nginx/sites-enabled/pixelwise
    sudo rm -f /etc/nginx/sites-enabled/default
    sudo nginx -t && sudo systemctl reload nginx
fi

# Grant produser passwordless sudo for the one restart auto-deploy needs
if command -v systemctl >/dev/null 2>&1 && id produser >/dev/null 2>&1; then
    sudo tee /etc/sudoers.d/pixelwise >/dev/null <<'EOF'
produser ALL=(root) NOPASSWD: /usr/bin/systemctl restart pixelwise
EOF
    sudo chmod 0440 /etc/sudoers.d/pixelwise
    sudo visudo -cf /etc/sudoers.d/pixelwise
fi

# Install the auto-deploy systemd timer on prod
if [ -f "$SCRIPT_DIR/deploy/systemd/pixelwise-deploy.timer" ] \
   && command -v systemctl >/dev/null 2>&1 \
   && id produser >/dev/null 2>&1; then
    sudo cp "$SCRIPT_DIR/deploy/systemd/pixelwise-deploy.service" \
        /etc/systemd/system/pixelwise-deploy.service
    sudo cp "$SCRIPT_DIR/deploy/systemd/pixelwise-deploy.timer" \
        /etc/systemd/system/pixelwise-deploy.timer
    sudo systemctl daemon-reload
    sudo systemctl enable --now pixelwise-deploy.timer
fi
