# Knowledge

Knowledge is a self-hosted, Confluence-style documentation and knowledge-sharing tool: spaces, a nested page tree, a rich block editor, version history, comments, labels, full-text search, attachments, templates, watches and notifications, and an admin console. It is the sibling of Backlog (`../backlog`) and deliberately shares its stack, conventions, env var names, and deployment shape. The full design lives in `docs/PLAN.md`; read it before changing anything structural.

## Stack

- API: Go 1.25, chi v5, pgx v5 (no ORM), goose migrations, argon2id sessions, go-oidc, aws-sdk-go-v2 S3, bluemonday, goldmark. Module `github.com/hayward-solutions/knowledge/api`.
- Web: Next.js 15 app router, React 18, TypeScript, Tailwind 3, React Query 5, TipTap 2.
- Data: Postgres 16, S3-compatible object storage (RustFS in compose and in the k3s cluster).
- Images: distroless api (uid 65532, read-only root filesystem), Next standalone web (uid 1000). Published to GHCR as multi-arch (amd64, arm64).

## Layout

```
docker-compose.yml, .env.example, README.md, CLAUDE.md, .gitignore, .editorconfig
.github/workflows/{pull_request.yaml,merge.yaml}
docs/{PLAN.md,API.md,CONFIGURATION.md,DEVELOPMENT.md,DEPLOYMENT.md}
api/
  cmd/server/main.go
  migrations/NNNN_name.sql
  internal/{config,domain,auth,bootstrap,store,storage,events,content,search,notify,http}
  internal/http/{router.go,middleware/,handlers/}
web/
  app/                 routes
  components/{layout,ui,editor,pages,spaces,comments,search,notifications,admin}/
  lib/api/             client.ts, types.ts, one module per resource, index.ts
  lib/sse.ts, lib/content/
.claude/pm/<task>/notes.md
```

## Commands

```bash
cp .env.example .env
docker compose up --build

cd api && go build ./... && go vet ./... && go test ./...
cd web && npm run lint && npm run typecheck && npm run build

cd web && npx playwright install chromium && npm run e2e
```

Integration tests use testcontainers-go Postgres and skip cleanly when Docker is absent.

## Conventions

- Never write code comments. No explanatory, section, TODO, or docblock comments, and do not restore ones that were removed. Explanations go in the response, the commit message, `.claude/pm/<task>/notes.md`, or the docs. Licence headers, shebangs, pragmas, and linter or type-checker directives are the only exceptions.
- Every handler file exposes `Routes(r chi.Router)` that registers flat paths using `r.With(...)` middleware chains. Never use `r.Route` or `r.Mount` inside handlers. `http/router.go` mounts each handler's `Routes` exactly once.
- Handlers never touch `Store.Pool`. All SQL lives in `internal/store`, one file per resource.
- Migrations are goose files numbered by range: foundation 0001-0009, spaces 0010-0019, pages 0020-0029, comments 0030-0039, labels and search 0040-0049, attachments 0050-0059, templates 0060-0069, notify 0070-0079, export 0080-0089, integration fixes 0090+. Every migration has Up and Down sections.
- Environment variable names are identical to Backlog's (`POSTGRES_DSN`, `API_PORT`, `PUBLIC_BASE_URL`, `APP_ORIGINS`, `ADMIN_*`, `OIDC_*`, `STORAGE_S3_*`, `AWS_*`) plus `COOKIE_SECURE` and `ATTACHMENT_MAX_BYTES`. There is no `STORAGE_S3_PUBLIC_ENDPOINT`; attachments are proxied through the API.
- JSON fields are snake_case. Every error response, including middleware errors, is `{"error": "...", "code": "..."}`.
- IDs are UUIDv7 generated in Go. The session cookie is `sid`.
- The web `lib/api/` has one module per resource; all of them are re-exported from `index.ts`.
- Agents never add or upgrade dependencies. `go.mod` and `package.json` are pinned by the foundation work; if a dependency seems necessary, say so in the notes and stop that sub-task.
- Agents never run `git`; commits are made by the orchestrator at the end of each phase.
- Only create or edit files you own for the task. Shared wiring is already structural (router mounts, per-resource API modules, pre-assigned migration ranges).
- Backlog at `../backlog` is the reference sibling. Copy its patterns before inventing new ones; the deliberate departures are listed in `docs/PLAN.md`.
