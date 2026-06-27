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
