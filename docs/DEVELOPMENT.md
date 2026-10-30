# Development

## Prerequisites

- Go 1.25+
- Node 22+
- Docker (for Postgres and MinIO, and for the API integration tests)

## Running everything with Docker

```bash
cp .env.example .env
docker compose up --build
```

Web is at <http://localhost:3000>, the API at <http://localhost:8080>, and the MinIO console at <http://localhost:9001>. The API container runs with a read-only root filesystem and `/tmp` on tmpfs, the same way the k3s cluster runs it.

Rebuild one service:

```bash
docker compose up -d --build api
docker compose up -d --build web
```

## Running outside Docker

### Postgres and MinIO

```bash
docker compose up -d db minio minio-createbucket
```

This exposes Postgres on `localhost:5432`, MinIO's S3 API on `localhost:9000`, and the MinIO console on `localhost:9001`, and creates the `knowledge-dev` bucket.

### API

```bash
cd api

export POSTGRES_DSN="postgres://knowledge:knowledge@localhost:5432/knowledge?sslmode=disable"
export API_PORT=8080
export PUBLIC_BASE_URL=http://localhost:3000
export APP_ORIGINS=http://localhost:3000
export COOKIE_SECURE=false
export ADMIN_EMAIL=admin@example.com
export ADMIN_PASSWORD=change-me-on-first-boot
export ADMIN_DISPLAY_NAME=Admin

export STORAGE_S3_BUCKET=knowledge-dev
export STORAGE_S3_REGION=us-east-1
export STORAGE_S3_ENDPOINT=http://localhost:9000
export STORAGE_S3_FORCE_PATH_STYLE=true
export AWS_ACCESS_KEY_ID=minioadmin
export AWS_SECRET_ACCESS_KEY=minioadmin
export ATTACHMENT_MAX_BYTES=26214400

go run ./cmd/server
```

On start the API connects to Postgres, runs goose migrations, bootstraps the admin if `users` is empty, initialises the S3 client, and listens on `API_PORT`. `go run ./cmd/server healthcheck` probes the running server and exits non-zero when it is unhealthy.

### Web

```bash
cd web
npm ci
export NEXT_PUBLIC_API_BASE_URL=http://localhost:8080
npm run dev
```

Web runs at <http://localhost:3000> with hot reload.

## Repo layout

```
knowledge/
├── docker-compose.yml, .env.example, README.md, CLAUDE.md
├── .github/workflows/{pull_request.yaml,merge.yaml}
├── docs/
├── api/
│   ├── cmd/server/main.go
│   ├── migrations/
│   └── internal/
│       ├── config/ domain/ auth/ bootstrap/ store/ storage/
│       ├── events/ content/ search/ notify/
│       └── http/{router.go,middleware/,handlers/}
└── web/
    ├── app/
    ├── components/{layout,ui,editor,pages,spaces,comments,search,notifications,admin}/
    └── lib/{api,content}/, lib/sse.ts
```

## Migrations

Migrations are goose SQL files in `api/migrations/` named `NNNN_name.sql`, each with `-- +goose Up` and `-- +goose Down` sections. They run automatically when the API starts. Number ranges are pre-assigned per area:

| Area | Range |
|---|---|
| foundation | 0001-0009 |
| spaces | 0010-0019 |
| pages | 0020-0029 |
| comments | 0030-0039 |
| labels and search | 0040-0049 |
| attachments | 0050-0059 |
| templates | 0060-0069 |
| notify | 0070-0079 |
| export | 0080-0089 |
| integration fixes | 0090+ |

To apply against a local database manually:

```bash
goose -dir api/migrations postgres "$POSTGRES_DSN" up
```

## Adding an API endpoint

1. Add the handler method in the resource's file in `api/internal/http/handlers/`.
2. Register a flat path in that file's `Routes(r chi.Router)` using `r.With(...)` for the middleware chain: `RequireAuth`, then `ResolveSpaceRole`, then `RequireSpacePerm(...)`. Do not use `r.Route` or `r.Mount`.
3. Put all SQL in the resource's file in `api/internal/store/`.
4. Mirror the types in `web/lib/api/types.ts` and add the call to the resource's module in `web/lib/api/`.

Routes keyed by page id use `ResolveSpaceIDFromPage` so the space role can still be resolved.

## Gates

```bash
cd api && go build ./... && go vet ./... && go test ./...
cd web && npm run lint && npm run typecheck && npm run build
```

Integration tests start Postgres with testcontainers-go and skip cleanly when Docker is not available. CI runs the same checks on every pull request.

## Common tasks

```bash
docker compose logs -f api
docker compose exec db psql -U knowledge -d knowledge
docker compose down -v
```

`docker compose down -v` deletes the database and every uploaded attachment.

## Troubleshooting

- **`admin bootstrap skipped` on first boot:** users already exist. Reset the database with `docker compose down -v` or sign in with a known account.
- **CORS errors in dev:** confirm the browser calls `NEXT_PUBLIC_API_BASE_URL` and that its origin is listed in `APP_ORIGINS`.
- **Login succeeds but the session is lost over HTTPS:** set `COOKIE_SECURE=true`. Over plain HTTP leave it `false` or the browser drops the cookie.
- **API container exits immediately with a read-only filesystem error:** something is writing outside `/tmp`; the image must work with `read_only: true`.
- **Uploads rejected:** the file exceeds `ATTACHMENT_MAX_BYTES`.
