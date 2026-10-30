# Development

## Prerequisites

- Go 1.26+ (`api/go.mod` says `go 1.26.0`, and the Docker builder is `golang:1.26-alpine`). Use **Go 1.26.10 or newer** locally: older 1.26 releases carry standard-library vulnerabilities that `govulncheck` reports (32 findings on 1.26.0, none on 1.26.10), and the published images are built with the current 1.26 patch release.
- Node 22+
- Docker with the Compose plugin (for Postgres and object storage, and for the API integration tests)
- `goose` CLI (optional; the API runs migrations on start). Install with `go install github.com/pressly/goose/v3/cmd/goose@v3.27.0`.

## Running everything with Docker

```bash
cp .env.example .env
docker compose up --build
```

Web is at <http://localhost:3000>, the API at <http://localhost:8080>, and the RustFS console at <http://localhost:9001/rustfs/console/> (credentials `MINIO_ROOT_USER` / `MINIO_ROOT_PASSWORD` from `.env`). `ADMIN_PASSWORD` in `.env` must satisfy the password policy (12+ characters with upper, lower, digit, and symbol) or the API exits at start.

The API container runs with a read-only root filesystem and `/tmp` on tmpfs, the same way the k3s cluster runs it. Its healthcheck is `/app/server healthcheck` (the distroless image has no shell or `wget`).

Rebuild one service:

```bash
docker compose up -d --build api
docker compose up -d --build web
```

`NEXT_PUBLIC_API_BASE_URL` is a build argument of the `web` service. After changing it, rebuild `web`.

## Running outside Docker

### Postgres and object storage

```bash
docker compose up -d db rustfs rustfs-createbucket
```

This exposes Postgres on `localhost:5432`, the RustFS S3 API on `localhost:9000` and its console on `localhost:9001`, and creates the `knowledge-dev` bucket. Do not also start the `api` and `web` containers, which would take ports 8080 and 3000.

### API

The API reads only environment variables, not `.env`. Export them in the shell you run it from:

```bash
cd api

export POSTGRES_DSN="postgres://knowledge:knowledge@localhost:5432/knowledge?sslmode=disable"
export API_PORT=8080
export PUBLIC_BASE_URL=http://localhost:3000
export APP_ORIGINS=http://localhost:3000
export COOKIE_SECURE=false
export ADMIN_EMAIL=admin@example.com
export ADMIN_PASSWORD='Change-Me-On-First-Boot-1'
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

Run it from `api/`: `MIGRATIONS_DIR` defaults to `./migrations`. On start the API:

1. Loads config (exits if `POSTGRES_DSN` is missing).
2. Waits up to 30 seconds for Postgres, then runs goose migrations.
3. Connects a `pgxpool`.
4. Creates the admin from env if `users` is empty (logs `admin bootstrap skipped` otherwise).
5. Loads OIDC (a no-op when unset) and initialises the S3 client (logs `object storage disabled` when `STORAGE_S3_BUCKET` is unset).
6. Starts the in-process event bus, SSE hub, and presence tracker, and the hourly expired-session cleanup.
7. Listens on `API_PORT`.

`go run ./cmd/server healthcheck` probes `http://127.0.0.1:$API_PORT/health` and exits non-zero when it is unhealthy. This is what the container healthcheck runs.

### Web

```bash
cd web
npm ci
export NEXT_PUBLIC_API_BASE_URL=http://localhost:8080
npm run dev
```

Web runs at <http://localhost:3000> with hot reload.

**Dev CSP.** `web/next.config.js` sends a Content-Security-Policy on every route. `'unsafe-eval'` is appended to `script-src` only when `NODE_ENV` is not `production` (React's development runtime needs it), so `npm run dev` hydrates while production builds and the published images keep the strict `script-src 'self' 'unsafe-inline'`. `connect-src` and `img-src` include `NEXT_PUBLIC_API_BASE_URL`. If a production build never hydrates, check the console for CSP violations rather than loosening the policy.

## Repo layout

```
knowledge/
├── docker-compose.yml, .env.example, README.md, CLAUDE.md
├── .github/workflows/{pull_request.yaml,merge.yaml}
├── docs/{PLAN.md,API.md,CONFIGURATION.md,DEVELOPMENT.md,DEPLOYMENT.md}
├── api/
│   ├── Dockerfile
│   ├── cmd/server/main.go           wiring, migrations, bootstrap, healthcheck subcommand
│   ├── migrations/NNNN_name.sql     goose migrations
│   └── internal/
│       ├── config/                  env loading into one Config struct
│       ├── domain/                  types, roles, permission matrix, event types
│       ├── auth/                    argon2id passwords, OIDC provider config
│       ├── bootstrap/               admin from env
│       ├── store/                   pgx repositories, one file per resource
│       ├── storage/                 ObjectStore interface and S3 client
│       ├── events/                  Bus, SSE Hub, Presence
│       ├── content/                 sanitiser, text extraction, Markdown export and import
│       ├── search/                  full-text query builder
│       ├── notify/                  bus subscriber that writes notifications and activity
│       ├── testutil/                testcontainers Postgres and fixtures
│       └── http/
│           ├── router.go            mounts every handler's Routes(r) once
│           ├── middleware/          auth, space role resolver, permission gate, rate limiter
│           └── handlers/            one file per resource, each with Routes(r chi.Router)
└── web/
    ├── Dockerfile, next.config.js, playwright.config.ts
    ├── app/                         Next.js app router pages
    ├── e2e/                         Playwright specs and support code
    ├── components/                  activity, comments, editor, importexport, labels, layout,
    │                                notifications, pages, search, spaces, templates, ui
    └── lib/
        ├── api/                     client.ts, types.ts, one module per resource, index.ts
        ├── content/                 slugs, headings, empty-doc helpers
        └── sse.ts, auth.tsx, theme.tsx, providers.tsx
```

## Migrations

Migrations are goose SQL files in `api/migrations/` named `NNNN_name.sql`, each with `-- +goose Up` and `-- +goose Down` sections. They run automatically when the API starts. Number ranges are pre-assigned per area so parallel work never collides:

| Area | Range | Present |
|---|---|---|
| foundation (users, sessions, OIDC, spaces, pages) | 0001-0009 | 0001, 0002, 0003 |
| spaces | 0010-0019 | none |
| pages | 0020-0029 | 0020 |
| comments | 0030-0039 | 0030 |
| labels and search | 0040-0049 | 0040 |
| attachments | 0050-0059 | 0050 |
| templates | 0060-0069 | 0060 |
| notify | 0070-0079 | 0070 |
| export | 0080-0089 | none |
| integration fixes | 0090+ | 0090 |

`0090_perf_indexes.sql` (integration fixes) adds `idx_page_views_page`, `idx_pages_recent` (published pages by `updated_at, id`), `idx_pages_draft_owner` (draft pages by creator), and `idx_notifications_user_created` (notifications by user and `created_at, id`); its Down section drops them in reverse. Pick the next free number in the range for your area, for example `api/migrations/0021_page_example.sql`:

```sql
-- +goose Up
CREATE INDEX idx_pages_example ON pages(updated_at);

-- +goose Down
DROP INDEX idx_pages_example;
```

Every migration needs a working Down section: `TestMigrationsDownAndUp` in `api/internal/store` migrates fully down and back up. To apply against a local database by hand:

```bash
goose -dir api/migrations postgres "$POSTGRES_DSN" up
goose -dir api/migrations postgres "$POSTGRES_DSN" status
```

## Adding an endpoint

1. **Permission.** Reuse a `Perm` from `api/internal/domain/permissions.go`. For a new one, add the constant and put it in the right `case` of `RoleAllows`, and update [CONFIGURATION.md](CONFIGURATION.md#roles-permissions-and-visibility).
2. **Store.** Add the SQL as a method on `*Store` in the resource's file in `api/internal/store/`. Parameterise every value. Handlers never touch `Store.Pool`.
3. **Handler.** Add the method in the resource's file in `api/internal/http/handlers/`. Each handler type embeds `Deps` (`Store`, `Bus`, `Hub`, `Presence`, `Storage`, `Config`, `OIDC`, `Version`). Respond with `writeJSON`, and fail with `httpErr(w, status, msg)` or `httpErrCode(w, status, code, msg)`; both produce `{"error": ...}`.
4. **Route.** Register a flat path in that file's `Routes(r chi.Router)` with `r.With(...)`. Never use `r.Route` or `r.Mount` in a handler; `router.go` calls each `Routes` once inside the single `/api/v1` route.
5. **Web.** Mirror new shapes in `web/lib/api/types.ts`, add the call to the resource's module in `web/lib/api/`, and make sure `web/lib/api/index.ts` re-exports it.
6. **Docs.** Add the row to [API.md](API.md).

### Middleware chain names

| Middleware | What it does |
|---|---|
| `mw.RequireAuth(h.Store)` | Valid `sid` session for an enabled user, else `401 unauthenticated`. Puts the user in the context (`mw.UserFrom`). |
| `mw.OptionalAuth(h.Store)` | Same, but continues without a user (used by logout). |
| `mw.RequireSystemAdmin` | A plain middleware, not a factory. Needs `RequireAuth` before it. `403 forbidden` for non-admins. |
| `mw.SpaceByKey(h.Store, perm)` | `RequireAuth`, then resolves the space from `{key}` (case-insensitive) and the caller's effective role into `mw.SpaceFrom(ctx)`, then `RequireSpacePerm(perm)`. |
| `mw.SpaceByPage(h.Store, perm)` | The same, resolving the space from the page `{pageID}`. |
| `mw.SpaceByResolver(h.Store, resolve, perm)` | The same, where `resolve(r)` returns the space ID of any other entity (see below). |
| `mw.RequireSpacePerm(perm)` | `403 forbidden` unless `RoleAllows(role, perm)`, and `403 space is read-only` for non-admins on archived spaces for anything but `view_space`. |
| `mw.AuthLimiter()` | Per-IP token bucket for credential endpoints. |

The `Space*` chains already include `RequireAuth`, so use them alone:

```go
r.With(mw.SpaceByKey(h.Store, domain.PermEditPages)).Post("/spaces/{key}/pages", h.Create)
r.With(mw.SpaceByPage(h.Store, domain.PermViewSpace)).Get("/pages/{pageID}", h.Get)
r.With(mw.RequireAuth(h.Store), mw.RequireSystemAdmin).Get("/admin/spaces", h.AdminList)
```

Routes keyed by another entity's ID (a comment, an attachment) pass a resolver that returns the entity's space ID, using `mw.URLUUID` for the parameter and `store.ErrNotFound` for a missing row:

```go
func (h *AttachmentHandler) resolveAttachmentSpace(r *http.Request) (uuid.UUID, error) {
	id, err := mw.URLUUID(r, "attachmentID")
	if err != nil {
		return uuid.Nil, err
	}
	return h.Store.GetAttachmentSpaceID(r.Context(), id)
}

r.With(mw.SpaceByResolver(h.Store, h.resolveAttachmentSpace, domain.PermViewSpace)).Get("/attachments/{attachmentID}", h.Download)
```

The middleware reads the URL parameters named `{key}` and `{pageID}`; other names are free. A malformed ID is `400`, a missing target or a space the caller cannot see is `404`.

Beyond the role, handlers enforce page-level visibility themselves (draft pages are creator-or-admin, trashed pages are editor-or-admin). Route page handlers through `pageLoad` in `handlers/pages.go`, which applies those rules and answers `409 page_trashed` for writes to trashed pages.

### Events

Publish after the transaction commits: `h.Bus.Publish(domain.PagePublished{...})`. Event structs live in `api/internal/domain/events.go` and are published by value. The `notify` service subscribes to the bus when the router is built and turns events into `notifications` and `activity` rows.

## Content contract

Page and comment content has three forms: `content_json` (ProseMirror JSON from TipTap, the source of truth), `content_html` (rendered by TipTap on the client at save time and sanitised on the server), and `content_text` (extracted server-side for search, never returned). Markdown import produces HTML with `content_json = null`; the editor loads HTML when the JSON is null.

Custom nodes round-trip as plain HTML, which the editor renders, the viewer styles (`.kn-content` in `web/app/editor.css`), and the sanitiser admits:

| Node | HTML |
|---|---|
| mention | `<span data-type="mention" data-id="<userID>">@Name</span>` |
| page link | `<a data-type="page-link" data-page-id="<id>" href="/spaces/KEY/pages/<id>/<slug>">Title</a>` |
| panel | `<div data-type="panel" data-color="info\|note\|success\|warning\|error"><p>...</p></div>` |
| expand | `<details data-type="expand"><summary>Title</summary><div>...</div></details>` |
| status | `<span data-type="status" data-color="grey\|blue\|green\|yellow\|red\|purple">TEXT</span>` |
| table of contents | `<div data-type="toc"></div>` (the viewer fills it from the headings) |
| inline comment mark | `<span data-type="comment" data-id="<commentID>">...</span>` |
| image | `<img src="/api/v1/attachments/<id>/content" data-attachment-id="<id>" alt width>` |
| task list | `<ul data-type="taskList"><li data-type="taskItem" data-checked="true\|false">` |
| code block | `<pre><code class="language-go">` |

The sanitiser (`api/internal/content/sanitize.go`, bluemonday) is an allowlist applied to every write of HTML (drafts, publish, version restore, comments, templates):

- Allowed elements: `p`, `h1`-`h6`, `ul`, `ol`, `li`, `blockquote`, `pre`, `code`, `strong`, `em`, `u`, `s`, `mark`, `table`, `thead`, `tbody`, `tr`, `th`, `td`, `hr`, `br`, `summary`, `a`, `img`, `span`, `div`, `details`. Scripts, event handlers, `style`, and every other element or attribute are stripped.
- `data-type` is checked per element: `span` may be `mention`, `status`, or `comment`; `div` `panel` or `toc`; `ul` `taskList`; `li` `taskItem`; `a` `page-link`; `details` `expand`.
- `data-id`, `data-page-id`, and `data-attachment-id` must be UUIDs. `data-color` must be one of `info`, `note`, `success`, `warning`, `error`, `grey`, `blue`, `green`, `yellow`, `red`, `purple`. `data-checked` must be `true` or `false`. `data-align` (on paragraphs and headings) must be `left`, `center`, `right`, or `justify`.
- `class` survives only on `code`, and only as `language-*`. Heading `id` must be a plain identifier. `width` and `height` on `img` must be numeric with an optional `px` or `%`. `colspan`, `rowspan`, and `colwidth` are admitted on cells.
- Links and images accept only `http`, `https`, `mailto`, and relative URLs (`data:` and `javascript:` are dropped). `rel` is limited to `noopener`, `noreferrer`, `nofollow`, `ugc` and `target` to `_blank` or `_self`.

`content_json` is stored as sent (it must be an object or null), so treat it as untrusted wherever it is consumed; the HTML is the sanitised form.

Add a custom node by changing, together: the TipTap extension in `web/components/editor/extensions/`, its styles in `web/app/editor.css`, the sanitiser allowlist, `content/text.go` (search text), `content/markdown.go` (export), and the samples in `web/components/editor/__contract__.ts`.

## Events bus and SSE

`api/internal/events` holds three in-memory structures:

- `Bus`: typed domain events. Every subscriber sees every event and runs in its own goroutine, with panics recovered and logged. `Bus.Wait()` blocks until in-flight handlers finish (tests call it; shutdown does too).
- `Hub`: per-user fan-out for `GET /notifications/stream`. Each subscriber has a 16-event buffer, and events are dropped when it is full.
- `Presence`: who is editing which page, with a 30 second TTL swept every 10 seconds.

All three live inside one process. **Run a single API replica.** With two, a notification created on one replica never reaches a stream held by the other, and presence is split. Notifications are still stored in Postgres, so a refetch shows them; only the live push is lost. Scaling out would need Postgres `LISTEN/NOTIFY` or Redis pub/sub behind the same interfaces.

## Testing

### API

```bash
cd api && go build ./... && go vet ./... && go test ./...
```

Unit tests need nothing. Integration tests (store, handlers, bootstrap, notify) use `testutil.NewDB`, which starts one shared `postgres:16-alpine` container per test binary with testcontainers-go, creates a fresh randomly named database per test, runs all migrations, and drops the database afterwards. They skip cleanly when Docker is not reachable.

To use a Postgres you already have, set `KNOWLEDGE_TEST_POSTGRES_DSN`. The role needs permission to create and drop databases, and the DSN's database is only used to connect:

```bash
cd api
export KNOWLEDGE_TEST_POSTGRES_DSN="postgres://knowledge:knowledge@localhost:5432/postgres?sslmode=disable"
go test -count=1 ./...
```

Check dependencies for known vulnerabilities with the same toolchain the images use:

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
cd api && GOTOOLCHAIN=go1.26.10 govulncheck ./...
```

On go1.26.10 it reports no affected code. It also lists vulnerabilities in required modules that the code does not call; those are informational.

In `handlers_test`, `newEnv(t, mutate)` runs the whole router over `httptest` with a cookie-jar client, and `testutil` has fixtures for users, sessions, spaces, members, and pages. Tests that need the event bus or a fake `storage.ObjectStore` build their own router; see `pages_test.go` and `attachments_test.go`. The test helpers set `Config.TrustedProxies` to loopback so each client can send its own `X-Forwarded-For`, which keeps the per-IP login limiter from being shared across tests; a test that builds its own router and relies on that header must set `TrustedProxies` the same way. Rate limiters are per router (`Deps.AuthLimits`), not process-global.

### Web

```bash
cd web && npm run lint && npm run typecheck && npm run build
```

`npm run lint` runs `next lint` (it prints a deprecation notice; Next 16 removes the command). `npm run typecheck` is `tsc --noEmit`.

### End to end

A Playwright suite lives in `web/e2e/` (`playwright.config.ts` in `web/`) and runs against the full Compose stack, with the web app on `http://localhost:3000` and the API on `http://localhost:8080`:

```bash
cp .env.example .env
docker compose up --build -d --wait
cd web
npm ci
npx playwright install chromium
npm run e2e
```

`npm run e2e` is `playwright test`. Specs run serially in one Chromium worker, with one retry, in numbered files (`01-auth.spec.ts` onwards) covering sign-in, spaces and pages, collaboration, notifications, export and import, trash and archive, templates, activity, attachments, admin, and editor features.

The global setup waits for `GET /health` and `/login`, signs in as the admin, and picks a space key that is not taken, so the suite can be re-run against a used stack. It reads the admin credentials from `.env` (falling back to `.env.example`); override with `E2E_ADMIN_EMAIL`, `E2E_ADMIN_PASSWORD`, `E2E_BASE_URL` (default `http://localhost:3000`), and `E2E_API_URL` (default `http://localhost:8080`). Auth state is kept in `web/e2e/.state/`, and the HTML report in `web/playwright-report/`.

### CI

`.github/workflows/pull_request.yaml` runs on pull requests to `main`, with a path filter per job:

- **api** (when `api/**` changes): `go build ./...`, `go vet ./...`, `go test -count=1 -timeout 600s ./...`. The runner has Docker, so the integration tests execute.
- **web** (when `web/**` changes): `npm ci`, `npm run lint`, `npm run typecheck`, `npm run build`.
- **e2e** (when `api/**`, `web/**`, `docker-compose.yml`, or `.env.example` changes): copies `.env.example` to `.env`, runs `docker compose up -d --build --wait`, installs Chromium, and runs `npm run e2e`. On failure it uploads the Playwright report and prints the stack logs; the stack is always torn down with `docker compose down -v`.

`merge.yaml` builds and publishes the images; see [DEPLOYMENT.md](DEPLOYMENT.md).

## Common tasks

```bash
docker compose logs -f api
docker compose exec db psql -U knowledge -d knowledge
docker compose down -v
```

`docker compose down -v` deletes the database and every uploaded attachment.

Reset one user's password without touching the rest: sign in as another system admin and use **Admin** then **Users**, or call `PATCH /api/v1/admin/users/{userID}` with `{"password": "..."}`. For a lost admin see [CONFIGURATION.md](CONFIGURATION.md#recovering-a-lost-admin).

## Troubleshooting

- **API container exits at start with `bootstrap: password must be at least 12 characters...`:** `ADMIN_PASSWORD` does not meet the password policy. Fix it in `.env`; the admin is created on the next start as long as the `users` table is still empty.
- **`admin bootstrap skipped` on first boot:** users already exist. Reset the database with `docker compose down -v` or sign in with a known account.
- **CORS errors in dev:** confirm the browser calls `NEXT_PUBLIC_API_BASE_URL` and that the page's exact origin (scheme, host, and port) is listed in `APP_ORIGINS`. Credentialed requests are allowed only from listed origins, and only the `Content-Type` request header is allowed.
- **Login succeeds but the session is lost over HTTPS:** set `COOKIE_SECURE=true`. Over plain HTTP leave it `false`, or the browser drops the `Secure` cookie.
- **Notifications only appear after a reload (SSE not updating):** confirm `/api/v1/notifications/stream` is a long-lived request in the network panel. A proxy that buffers or compresses the response holds the events back; the API sends `X-Accel-Buffering: no` and `Cache-Control: no-cache`, but a reverse proxy in front must also stream responses unbuffered. Cross-origin setups need the origin in `APP_ORIGINS`.
- **404 for a space or page you know exists:** private spaces answer `404` to non-members, as do draft pages to anyone but their creator and space admins, and trashed pages to viewers. Check membership and role in **Settings** then **Members**, or look as a system admin. Keys are case-insensitive but must be the key, not the name.
- **Everyone gets `429 too many requests` on login behind a proxy:** with `TRUSTED_PROXIES` empty the API keys rate limits on the TCP peer, which behind an ingress is the proxy for all users. Set `TRUSTED_PROXIES` to the proxy's address range (see [CONFIGURATION.md](CONFIGURATION.md#rate-limiting-and-proxies)). One user locked out after a few wrong passwords is the per-email limit, which refills one token every 12 seconds.
- **SSO shows a "Sign-in blocked" page (403):** the IdP did not say the email is verified, and the email belongs to an existing local account. Verify the address at the IdP, sign in with the password, or (only if the IdP guarantees addresses) set `OIDC_REQUIRE_EMAIL_VERIFIED=false`.
- **Markdown import answers 400 or 503:** one of the [import limits](API.md#import-limits) was hit (100 files, 500 pages, 2 MiB per file, 10 folder levels, 20000 lines, 5 second conversion), or all four conversion slots were busy. Nothing is created when an import fails.
- **Page tree errors (`page tree too deep`):** the tree is capped at 64 levels.
- **A long upload dies after five minutes, or a request after sixty seconds:** these are the server's read deadlines. Attachment and import `POST`s get five minutes; every other request must finish arriving within sixty seconds.
- **Uploads rejected with 413:** the file exceeds `ATTACHMENT_MAX_BYTES` (default 25 MiB), or a reverse proxy in front has a smaller body limit and answers 413 itself.
- **Uploads or downloads rejected with 503:** `STORAGE_S3_BUCKET` is unset, so object storage is disabled. Attachment listing still works.
- **Uploads fail with a storage error:** check `STORAGE_S3_ENDPOINT` is reachable from the API (not from the browser), `STORAGE_S3_FORCE_PATH_STYLE=true`, region `us-east-1`, that the bucket exists, and that `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` match the store's credentials.
- **Container fails with a read-only filesystem error:** the API image must work with `read_only: true`, with `/tmp` writable (upload spooling). If it fails, something is writing elsewhere. The web image needs `/app/.next/cache` writable, so mount an `emptyDir` or tmpfs there if you make its root filesystem read-only.
- **`npm run dev` page never hydrates:** the page is being served with the production Content-Security-Policy, which blocks `eval`. This happens when `NODE_ENV=production` is exported in your shell or you are looking at a production build; unset it.
