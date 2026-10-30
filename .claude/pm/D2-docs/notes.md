# D2 docs: notes

## What was written

`README.md`, `docs/API.md`, `docs/CONFIGURATION.md`, `docs/DEVELOPMENT.md`, `docs/DEPLOYMENT.md`. Nothing else in the repo was edited (`docs/PLAN.md` untouched). Style follows `../backlog/README.md` and `backlog/docs/{API,CONFIGURATION,DEVELOPMENT}.md`: same section order, same table shapes, no marketing language, no emojis or em dashes.

- `README.md`: tagline, Why, Features at a glance (Spaces & pages, Editor, Collaboration, Search & organisation, Access & admin), Quick start, docs links, Roadmap (the plan's out-of-scope list), License TBD. Every feature line was checked against code, not the plan.
- `docs/API.md`: all 81 routes under `/api/v1` plus `/health`, taken from the `Routes` functions (snapshot of the registration lines kept while writing and diffed at the end; unchanged). Per route: method, path, access (`auth`, `sysadmin`, or `space:<perm>`), body fields, response TypeScript type. Sections for the permission key, the error table, publishing and `version_conflict`, cursor pagination, the SSE stream format, and search syntax.
- `docs/CONFIGURATION.md`: every env var with the code default and the compose default side by side, storage, OIDC setup with admin-group mapping and the no-demotion rule, cookie security, permission matrix and visibility semantics, first-boot bootstrap, lost-admin recovery, manual verification walkthrough.
- `docs/DEVELOPMENT.md`: prerequisites, compose, running api and web outside compose with the exact exports, dev CSP, repo layout, migrations, adding an endpoint with the real middleware names, content contract and sanitiser rules, events bus and SSE single-process note, tests (Go, `KNOWLEDGE_TEST_POSTGRES_DSN`, web gates, Playwright, CI), troubleshooting.
- `docs/DEPLOYMENT.md`: image tags from `merge.yaml`, runtime constraints (uids, read-only root, `/tmp`, `/app/.next/cache`), ingress path split with empty `NEXT_PUBLIC_API_BASE_URL`, Postgres and RustFS, secrets, and the pi-lab Terraform checklist modelled on `pi-lab/terraform/001-business-tools/backlog/`.

## Verification actually run

- `cd api && go build ./... && go vet ./... && go test -count=1 ./...`: all packages ok (Docker up, integration tests ran).
- `cd web && npm run lint && npm run typecheck && npm run build`: clean, 15 static pages plus the dynamic routes.
- `cp .env.example .env && docker compose config`: ok. `docker compose up --build -d` on a scratch copy of the repo (project name `kndocs`, other agents' `knowledge` project left alone): db, rustfs, bucket job, api, web all healthy; admin bootstrapped from the shipped `.env.example`; `docker compose up -d --wait` returns 0 with the exited bucket job.
- Live API walkthrough with curl against that stack: login, oidc config (`{"enabled":false}`), create user, private space 404 for a non-member, page create and publish, `version_conflict` (409 with `current_version`) and `force`, `page_trashed`, search with `<mark>` snippets, quick search, empty-query 400, space `last_admin` and system last admin 409 (no code on the latter), 401/404/405 envelopes, attachment upload to RustFS and download headers, 26 MB upload gives 413, SSE (`hello`, `notification`, `unread_count`) for a watcher, page export md/html, space zip, nested Markdown import, dashboard, activity, template starters, labels, inline comment.
- API run outside compose with the documented exports (on 8081 to avoid the other stack) against the compose db and RustFS: starts, `server healthcheck` exits 0, and exits 1 for a closed port.
- `go install github.com/pressly/goose/v3/cmd/goose@v3.27.0` then `goose -dir api/migrations postgres "$DSN" status`: lists 0001 to 0070.
- `KNOWLEDGE_TEST_POSTGRES_DSN` against an existing Postgres 16 (store tests, 11 passed, no skips) and against `postgres:15` (store, handlers, bootstrap, notify suites pass), which backs the statement that the pi-lab Postgres 15 works.
- `npm ci` then `npm run dev -- -p 3200`: `/login` is 200 with `'unsafe-eval'` in `script-src`; production image has the strict policy. A web image built with an empty `NEXT_PUBLIC_API_BASE_URL` has `connect-src 'self'` only.
- `docker build` of both images with `VERSION` and an empty `NEXT_PUBLIC_API_BASE_URL`; api container is `nonroot` with a read-only root and tmpfs `/tmp`; web runs as `node` (uid 1000) and serves `/login` with `--read-only --user 1000 --tmpfs /app/.next/cache`.
- Lost-admin recovery option 4 (Go argon2id hash generator plus `UPDATE users`): generated a hash, wrote it with psql, and logged in with it.
- Confirmed `DELETE FROM users` fails once a space exists (foreign keys from `spaces.created_by` and others), so the old recovery text was wrong and was replaced.

## Not run by me

- The Playwright suite (`npm run e2e`). D1 was still adding specs while I wrote; the docs describe the files, scripts, env vars, and the CI job as they were at the end.
- `kubectl rollout restart ...` and the Terraform checklist (no cluster access, and the stack does not exist yet).
- The 503 path with `STORAGE_S3_BUCKET` unset (documented from the handler code and A1/B5 notes, covered by the API tests).
- A real OIDC login (documented from `handlers/oidc.go`, `auth/oidc.go`, and `store.UpsertOIDCUser`).
- The UI steps of the verification walkthrough were written from the component sources (labels, menu names, routes), not clicked through.

## Findings for others

1. `CLAUDE.md` says every error response is `{"error","code"}`. In the code `code` exists only on `version_conflict`, `page_trashed`, and `last_admin` (space members). Everything else is `{"error"}`. API.md documents the real behaviour.
2. The plan and the old DEVELOPMENT.md name middleware `ResolveSpaceRole` and `ResolveSpaceIDFromPage`. The code has `SpaceByKey`, `SpaceByPage`, `SpaceByResolver` (each: `RequireAuth`, resolve role, `RequireSpacePerm`) and a standalone `RequireSpacePerm`. DEVELOPMENT.md uses the real names.
3. `.env.example` and the compose default for `ADMIN_PASSWORD` used to be weak (`change-me-on-first-boot`, `change-me`), which the password policy rejects, so the API exited at bootstrap. D1 fixed both while I was working (`Change-Me-On-First-Boot-1`); docs state the policy so a user who edits it does not hit the same wall.
4. The old CONFIGURATION.md said `OIDC_SCOPES` is "comma- or space-separated"; `splitList` splits on commas only. Also, when `OIDC_ADMIN_GROUP` is set and an admin linked to SSO is not in the group, the SSO login demotes them with no last-admin check. Documented, not changed.
5. Compose moved from MinIO to RustFS during D1 (the `MINIO_ROOT_*` variable names are kept and are fed to RustFS as `RUSTFS_ACCESS_KEY` / `RUSTFS_SECRET_KEY`). MinIO and AWS S3 are described as "same variables, not exercised here". The RustFS console is at `/rustfs/console/` on port 9001, not the root.
6. JSON body size: pages and templates answer 413 for bodies over 1 MiB; spaces, comments, labels, and auth answer `400 bad body`.
7. Starter templates are now also seeded at API start (`handlers.SeedStarterTemplates`), in addition to the lazy seed on an admin's `GET /templates`. B6's notes still describe only the lazy path.
8. The web image defines no HEALTHCHECK; compose adds a `wget` healthcheck on `/login`. Backlog's deployment defines no web probe either.

## Process note

I ran `git status` once by mistake while in `api/` (read-only, no output kept) before remembering the no-git rule. No other git command was run and nothing was staged or changed.

## Refresh after the review remediation

Edited: `docs/API.md`, `docs/CONFIGURATION.md`, `docs/DEVELOPMENT.md`, `docs/DEPLOYMENT.md`, and `CLAUDE.md` (the stack line now says Go 1.26; it was the only line in the file that said 1.25). `README.md` needed no change. Every item below was checked in code (`config.go`, `router.go`, `middleware/{realip,ratelimit,longrequest}.go`, `handlers/{auth,oidc,export,comments,pages}.go`, `store/{store,activity,users,pages,export}.go`, `cmd/server/main.go`, `migrations/0090`), not copied from the notes.

Documented:
- `TRUSTED_PROXIES` (default empty; effect; right-most-untrusted `X-Forwarded-For` rule; invalid entry fails startup) and `OIDC_REQUIRE_EMAIL_VERIFIED` (default true; 403 "Sign-in blocked" page, only when linking to an existing local account; new accounts and already-linked identities unaffected). `docker-compose.yml` does not forward either, which the docs say. DEPLOYMENT.md puts `TRUSTED_PROXIES` in the env table, the proxy section (how to find the range, the spoofing caveat, how to check it took effect), and checklist item 8.
- Go 1.26 prerequisite and builder; govulncheck: 32 findings on local go1.26.0 (all standard library), none with `GOTOOLCHAIN=go1.26.10`, so the docs say to use 1.26.10+ locally. The lost-admin hash generator now pins `golang.org/x/crypto@v0.58.0` (re-run and it produces a valid hash).
- Per-email login limiter (burst 5, one token per 12 s, every attempt costs a token, checked before hashing) and the lock-out trade-off; per-IP bucket; 4-slot argon2 semaphore; password change deletes the user's other sessions.
- Server timeouts (10 s header, 60 s read, 120 s idle, 64 KiB headers, no write timeout), the 5-minute deadline for `POST .../attachments` and `.../import`, `statement_timeout` 30 s. `WithTxTimeout` exists but nothing calls it yet, so the docs only claim the 30 s default.
- Import hard limits table (100 files, 500 pages, 10 dir levels, 120/255/4096, 2 MiB or `ATTACHMENT_MAX_BYTES`, 32 MiB body, 20000 lines, 32 nesting, 5 s, 4 slots with 503, depth 64) and the exact error strings.
- Tree depth cap 64 (create, copy, move, restore, import), home page can be neither trashed nor moved (even through an ancestor), private spaces answer 404, trashed-page visibility on page/version/comment/label/attachment/activity routes, `inline_anchor` `prefix`/`suffix` (64 runes each), activity feed visibility rules, publish notifies only new mentions and not repeatedly while unread.
- OIDC: admin-group mapping never demotes the last active admin (kept, warning logged), under advisory lock 7351001 shared with the admin console; `next` validation rules. The earlier statements that the API ignores `email_verified` and that the mapping has no last-admin guard were removed.
- Migration 0090 (four indexes) in DEVELOPMENT.md; troubleshooting entries for 429 behind a proxy, Sign-in blocked, import 400/503, depth errors, upload deadlines.

Re-run and passing: `go build ./... && go vet ./... && go test -count=1 ./...` (all packages ok); `npm run lint && npm run typecheck && npm run build`; the store, handlers, bootstrap and notify suites against `postgres:15` via `KNOWLEDGE_TEST_POSTGRES_DSN`; `docker build` of the api image on `golang:1.26-alpine`; a container run of that image with a read-only root: five wrong logins for one email then 429 on the sixth and seventh, and a spoofed `X-Forwarded-For` did not help (default `TRUSTED_PROXIES`); doc links and anchors resolve; no em dashes.

Not verified: that the Tailscale ingress proxy sends `X-Forwarded-For` and which address range it connects from (DEPLOYMENT.md tells the operator how to check); the pi-lab cluster pod CIDR (k3s default 10.42.0.0/16 is stated as a default to confirm).
