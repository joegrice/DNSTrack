# AGENTS.md

## Build & Run

```bash
go build -o dnstrack ./cmd/dnstrack/
./dnstrack
```

No test suite or lint config exists yet.

## Architecture

- **Binary**: single entrypoint `cmd/dnstrack/main.go`
- **Module**: `github.com/joegrice/dnstrack`
- **Port**: 8420 (set via `config.yaml`)
- **Database**: SQLite at `data/dnstrack.db` (WAL mode, pure Go via `modernc.org/sqlite`)
- **Frontend**: static SPA at `web/dist/index.html` (Chart.js, vanilla JS)
- **Scheduler**: `robfig/cron/v3` — runs DNS tests against configured providers
- **DNS resolution**: UDP via `miekg/dns`, DoH via `POST application/dns-message`

Config lives in `config.yaml` at the repo root. The Docker entrypoint expects:
`-config /app/config.yaml -db /app/data/dnstrack.db -frontend /app/web/dist`

## Docker

Multi-stage build (`Dockerfile`): `golang:1.26-alpine` → `alpine:latest`.
Image published to `ghcr.io/joegrice/dnstrack` on stable release via `.github/workflows/release.yml`.

## Key Dependencies

- `github.com/go-chi/chi/v5` — HTTP router
- `github.com/miekg/dns` — DNS protocol
- `github.com/robfig/cron/v3` — Cron scheduler
- `modernc.org/sqlite` — Pure-Go SQLite
- `gopkg.in/yaml.v3` — YAML config parsing

## Capabilities

- **UDP DNS resolution** on port 53 against configured provider IPs
- **DoH (DNS over HTTPS)** resolution via `POST application/dns-message` when `provider.type == "doh"`
- **Configurable DNS record types** (`A`, `AAAA`, `MX`, `TXT`, etc.) via `record_types` in config
- **Per-provider availability tracking** via `GET /api/availability?hours=N` — returns total/succeeded/percentage per provider
- **Health indicator dots** and uptime % labels on the dashboard SPA
