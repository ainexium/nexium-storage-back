# NEXIUM Storage — API

Go REST API for NEXIUM Storage. Handles authentication, file management, billing, webhooks, and admin operations.

**Stack:** Go · PostgreSQL · Cloudflare R2 · Mailjet · Adullam

---

## Requirements

- Go 1.22+
- PostgreSQL 16+ (or a managed instance — Supabase, Neon, etc.)
- Cloudflare R2 bucket
- Docker (optional)

---

## Environment variables

Copy `.env.example` to `.env` and fill in your values.

| Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | ✓ | PostgreSQL connection string |
| `JWT_SECRET` | ✓ | Random secret — `openssl rand -hex 32` |
| `R2_ACCOUNT_ID` | ✓ | Cloudflare account ID |
| `R2_ACCESS_KEY_ID` | ✓ | R2 API token key ID |
| `R2_SECRET_ACCESS_KEY` | ✓ | R2 API token secret |
| `R2_BUCKET_NAME` | ✓ | Name of your R2 bucket |
| `APP_URL` | ✓ | Public API URL (e.g. `https://api.nexium.ai`) |
| `PORT` | — | HTTP port (default: `8080`) |
| `MAX_FILE_SIZE_MB` | — | Max upload size per file (default: `100`) |
| `STORAGE_QUOTA_GB` | — | Platform-wide storage quota (default: `10`) |
| `JWT_ACCESS_TTL` | — | Access token TTL (default: `15m`) |
| `JWT_REFRESH_TTL` | — | Refresh token TTL (default: `168h`) |
| `R2_PUBLIC_URL` | — | Public R2 domain (e.g. `https://pub-xxx.r2.dev`) |
| `MAILJET_API_KEY` | — | Mailjet API key (transactional emails) |
| `MAILJET_SECRET_KEY` | — | Mailjet secret key |
| `MAIL_FROM` | — | Sender email address |
| `MAIL_FROM_NAME` | — | Sender display name (default: `NEXIUM Storage`) |
| `ADULLAM_API_KEY` | — | Adullam payment gateway API key |
| `ADULLAM_WEBHOOK_SECRET` | — | Adullam webhook HMAC secret |

---

## Run locally

**With Docker (includes Postgres):**

```bash
docker compose -f docker-compose.dev.yml up --build
```

API available at `http://localhost:8080`.

**Without Docker:**

```bash
# Start Postgres separately, then:
go run ./cmd/server
```

Migrations run automatically on startup.

---

## Seed (first run)

Creates the first admin user:

```bash
go run ./cmd/seed
```

Default credentials are set via `SEED_ADMIN_EMAIL` and `SEED_ADMIN_PASSWORD` environment variables.

---

## Deploy (production)

```bash
# On the VPS — first time
git clone https://github.com/your-org/nexium-storage-api
cd nexium-storage-api
nano .env          # fill in production values
docker compose -f docker-compose.prod.yml up -d --build

# Updates
git pull
docker compose -f docker-compose.prod.yml up -d --build
```

---

## Project structure

```
cmd/
  server/      — entrypoint
  seed/        — admin user seed script
internal/
  auth/        — registration, login, JWT, password reset
  projects/    — project CRUD
  buckets/     — bucket CRUD
  files/       — file upload, download, delete
  apikeys/     — API key management
  usage/       — storage usage tracking
  billing/     — plans, subscriptions, payments (Adullam)
  webhooks/    — outbound webhook delivery
  admin/       — admin panel operations
  logs/        — activity logs
  expiry/      — inactivity & subscription expiry job
  mailer/      — Mailjet email client
  storage/     — Cloudflare R2 client
pkg/
  config/      — environment config
  database/    — pgx connection & migrations
  middleware/  — JWT auth, API key auth, rate limiting, admin guard
  adullam/     — Adullam payment gateway client
  apierr/      — typed API errors
migrations/    — SQL migrations (applied automatically at startup)
```

---

## API reference

Full documentation: [https://storage.nexium.ai/docs](https://storage.nexium.ai/docs)

External API endpoints (API key auth) are prefixed with `/api/v1/ext/`.
