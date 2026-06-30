# PixelWise Go

A Go reimplementation of the PixelWise handwritten-digit classifier and a benchmark comparing it against the original Python/FastAPI backend. 

## Setup

On a fresh Ubuntu VM (x86_64 or ARM64):

1. Clone this repo to `/opt/pixelwise-go`.
2. Create the Python venv: `python3 -m venv .venv`
3. Copy `.env.example` to `.env` and fill in the secrets.
4. Run the provisioner:

   ```bash
   ./setup-server.sh
   ```

`setup-server.sh` is idempotent. It installs system packages, pulls the model,
generates the Go inference weights and golden test fixtures, sets up PostgreSQL,
builds the Go binary, and installs the systemd/nginx units.

After setup, verify the Go side:

```bash
go test ./...     # must pass (golden-file inference check)
go build -o pixelwise-go .
```

## Where things are

| What | Where |
|------|-------|
| Go server (routes + handlers, inference, DB) | `main.go`, `model.go`, `db.go` |
| Go tests (golden-file validation) | `inference_test.go`, `handlers_test.go` |
| Python reference implementation | `app/` |
| Benchmark scripts (run sequentially, never together) | `bench/bench-python.sh`, `bench/bench-go.sh` |
| Benchmark results | `results/` |
| Deployment units (systemd, nginx) | `deploy/` |
| One-time server setup | `setup-server.sh` |
