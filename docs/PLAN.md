# Knowledge: Confluence-style documentation and knowledge-sharing tool

## Context

Matt needs a self-hosted documentation and knowledge-sharing tool for fields and projects where Confluence cannot be used. The deliverable is a best-effort Confluence clone: a Go API plus a Next.js web app, containerised, published to GHCR as arm64/amd64 images, and later deployed to the pi-lab k3s cluster beside Backlog (the Jira-like sibling at `/Users/matt/Projects/hayward-solutions/backlog`). Knowledge must look and feel like a sibling of Backlog: same stack, same conventions, same deployment shape.

I (the orchestrator) write no code. Every file is produced by a Sonnet sub-agent working from a brief derived from this plan. I review each agent's output, run the build and test gates, send remediation back to the same agent, and commit at the end of each phase.

Decisions already made with Matt:

- Frontend: Next.js app router + TipTap (ProseMirror) editor, two containers like Backlog.
- No real-time co-editing in v1. Autosave drafts, "someone else is editing" presence, and conflict detection on publish. Document model is ProseMirror JSON so Yjs can be added later.
- Full best-effort clone scope (listed under Features).
- Deliver container images, docker-compose, and CI only. The pi-lab Terraform stack is a later task in the pi-lab repo.

## Reference implementation: Backlog

Agents copy Backlog's conventions rather than inventing new ones. Key references:

| Concern | Backlog file |
|---|---|
| Server wiring, migrations, admin bootstrap | `backlog/api/cmd/server/main.go` |
| Router and middleware composition | `backlog/api/internal/http/router.go`, `backlog/api/internal/http/middleware/` |
| Handler struct style, JSON helpers | `backlog/api/internal/http/handlers/util.go`, `teams.go`, `tasks.go` |
| Auth: argon2id, sessions, OIDC | `backlog/api/internal/auth/`, `handlers/auth.go`, `handlers/oidc.go` |
| Role and permission matrix | `backlog/api/internal/domain/` |
| Thin pgx store, no ORM | `backlog/api/internal/store/store.go`, `tasks.go` |
| Goose migrations | `backlog/api/migrations/0001_init.sql` onward |
| S3 presigned upload and download | `backlog/api/internal/storage/`, `handlers/attachments.go` |
| In-process SSE hub | `backlog/api/internal/events/`, `handlers/stream.go` |
| Dockerfiles | `backlog/api/Dockerfile`, `backlog/web/Dockerfile` |
| CI: PR checks and multi-arch GHCR publish | `backlog/.github/workflows/pull_request.yaml`, `merge.yaml` |
| Compose and env | `backlog/docker-compose.yml`, `.env.example` |
| Web shell, API client, SSE hook | `backlog/web/components/AppShell.tsx`, `Sidebar.tsx`, `TopBar.tsx`, `web/lib/api.ts`, `web/lib/sse.ts` |
| Docs style | `backlog/README.md`, `backlog/docs/{API,CONFIGURATION,DEVELOPMENT}.md` |

Most useful single files: `middleware/auth.go` (RequireAuth, OptionalAuth, RequireAdmin, ResolveTeamRole, RequirePerm, cookie helpers), `domain/permissions.go` (`Allows` matrix), `handlers/comments.go` + `store/comments.go` (a complete CRUD pair), `events/hub.go` + `handlers/stream.go` (SSE with 30 s heartbeat that re-validates the session), `web/lib/api.ts`, `web/lib/sse.ts`, `web/lib/providers.tsx`, `web/lib/theme.tsx` (class-based dark mode), `web/next.config.js` (CSP and security headers). IDs are UUIDv7 generated in Go. Session cookie is `sid`, server-side sessions table, 30-day lifetime, `COOKIE_SECURE` flag. Password policy is 12+ chars with mixed classes. Auth limiter is a per-IP token bucket (burst 10, 10/min).

Deliberate departures from Backlog (Backlog's docs claim some of these but the code does not do them):

- Attachments are proxied through the API in both directions: multipart `POST` upload streamed to S3 (configurable cap, default 25 MB), and `GET` streamed back with correct `Content-Type` and `Content-Disposition`. No presigned URLs and no `STORAGE_S3_PUBLIC_ENDPOINT`, so RustFS behind the Tailscale ingress needs no CORS and the browser never talks to object storage.
- A real `config` package with one `Config` struct and a single `Load()`; env var names stay identical to Backlog's so the pi-lab secrets and compose files carry over.
- Middleware errors are JSON `{"error": ...}` like handler errors, not plain text.
- Images run as non-root: api on `gcr.io/distroless/static` as uid 65532 with a `server healthcheck` subcommand for the compose healthcheck (no wget); web uses Next `output: "standalone"`, `npm ci`, and uid 1000. Both accept a `VERSION` build arg exposed at `GET /health` and in the account menu.
- Argon2id parameters tuned for a Raspberry Pi: m=32 MiB, t=3, p=2.
- Hourly expired-session cleanup goroutine.
- Tests exist and run in CI: Go unit + testcontainers integration, web lint + typecheck + build, Playwright e2e in Phase 3.
- Handlers never touch `Store.Pool` directly; all SQL lives in `store/`.

Deployment target facts that shape the build (from pi-lab): every node is arm64; images are pulled from `ghcr.io/hayward-solutions/<repo>/<service>:main-arm64`; api runs as uid 65532 with a read-only root filesystem and a `/tmp` emptyDir; web runs as uid 1000 with `/app/.next/cache` writable; liveness probe is `GET /health` on 8080; object storage is RustFS (S3 API, path-style, region `us-east-1`); OIDC is Cognito with groups claim `cognito:groups`; the Tailscale ingress routes `/api` and `/health` to the api service and everything else to web on 3000. Everything in this list must work unchanged for Knowledge.

## Features (v1 scope)

Spaces with keys (e.g. `ENG`), space home page, space directory, space visibility (private, internal), space roles (admin, editor, viewer), space archive.
Page tree with unlimited nesting, ordering, drag move, breadcrumbs, child page list, page slugs, "unpublished draft" new pages.
Rich block editor: headings, lists, task lists, tables with row/column controls, code blocks with syntax highlighting, images (upload by drop or paste), links, internal page links with search picker, `@` mentions, panels (info, note, success, warning, error), expand/collapse sections, status lozenges, table of contents, horizontal rule, blockquote, slash command menu, bubble menu, keyboard shortcuts.
Autosave drafts per user, publish with optional version message, conflict detection, edit presence banner.
Version history: list, view any version, side-by-side or inline diff between versions, restore.
Page comments (threaded, edit, delete) and inline comments anchored to editor text with resolve/reopen.
Labels on pages, label pages across the space.
Full-text search (Postgres FTS) with snippets, filters by space, label, author, and a quick-switcher for titles.
Attachments on pages (presigned S3 upload), listing, download, delete, image embedding.
Templates: global and per-space page templates, create page from template, built-in starters (meeting notes, decision record, how-to, retrospective).
Watch pages and spaces, in-app notifications (bell, unread count, list, mark read) delivered over SSE, activity feed per space and a home dashboard (recent updates, recently viewed, my drafts, starred pages).
Star pages.
Trash: trash page subtree, restore, purge; space-level trash view.
Export page as Markdown or standalone HTML, export whole space as a Markdown zip, import Markdown files as pages.
Admin console: users (create, disable, reset password, grant system admin), spaces overview, global templates, global labels.
Local email+password auth and OIDC SSO with admin-group mapping, identical configuration surface to Backlog.
Account settings: display name, email, password.

Out of scope for v1, noted so agents do not drift into them: real-time co-editing, page-level restrictions, anonymous/public spaces, PDF export beyond print stylesheet, email delivery, Confluence XML import, macros beyond those listed, Backlog cross-links (a later integration).

## Architecture

```
knowledge/
├── docker-compose.yml, .env.example, README.md, CLAUDE.md, .gitignore
├── .github/workflows/{pull_request.yaml,merge.yaml}
├── docs/{API.md,CONFIGURATION.md,DEVELOPMENT.md,DEPLOYMENT.md}
├── api/                                 Go 1.25, module github.com/hayward-solutions/knowledge/api
│   ├── cmd/server/main.go
│   ├── migrations/NNNN_name.sql         goose, numbered ranges per agent (see Orchestration)
│   └── internal/
│       ├── config/                      env loading into Config struct
│       ├── domain/                      types, roles, permission matrix, event types
│       ├── auth/                        argon2id, sessions, OIDC
│       ├── bootstrap/                   admin-from-env
│       ├── store/                       pgx repos, one file per resource
│       ├── storage/                     S3 presigned client
│       ├── events/                      in-process bus + SSE hub + presence
│       ├── content/                     ProseMirror JSON walkers: text extraction, markdown export, html sanitiser, md import
│       ├── search/                      FTS query builder
│       ├── notify/                      event subscribers writing notifications and activity
│       └── http/
│           ├── router.go                mounts every handler's Routes(r)
│           ├── middleware/              RequireAuth, RequireSystemAdmin, ResolveSpaceRole, RequireSpacePerm, rate limit, CORS
│           └── handlers/                one file per resource, each with a Routes(r chi.Router) method
└── web/                                 Next.js 15, React 18, TypeScript, Tailwind 3, React Query 5, TipTap 2
    ├── app/                             routes (listed under Web routes)
    ├── components/{layout,ui,editor,pages,spaces,comments,search,notifications,admin}/
    └── lib/api/{client.ts,types.ts,<resource>.ts,index.ts}, lib/sse.ts, lib/content/
```

Runtime: Postgres 16 (compose) or 15 (pi-lab module), MinIO in compose and RustFS in pi-lab. Single API process; the event bus is in-memory, as in Backlog.

### Content model

- `content_json` (jsonb, nullable): ProseMirror document, source of truth, produced by TipTap.
- `content_html` (text): rendered by TipTap on the client at save time, sanitised server-side with bluemonday using an allowlist that admits the editor's elements, `class`, `data-type`, `data-id`, `data-page-id`, `data-level`, `data-color`, `data-checked`, `href`, `src`, `alt`, `title`, `colspan`, `rowspan`, `colwidth`, and `id` on headings. Scripts, event handlers, and non-http(s)/relative URLs are stripped.
- `content_text` (text): extracted server-side from `content_json` (or from HTML when JSON is null) for search.
- Markdown import produces HTML via goldmark with `content_json = null`; the editor loads HTML when JSON is null and TipTap re-derives JSON on first save.
- Custom node HTML contract (editor renders, viewer styles, sanitiser admits):
  - mention: `<span data-type="mention" data-id="<userID>">@Name</span>`
  - page link: `<a data-type="page-link" data-page-id="<id>" href="/spaces/KEY/pages/<id>/<slug>">Title</a>`
  - panel: `<div data-type="panel" data-color="info|note|success|warning|error"><p>…</p></div>`
  - expand: `<details data-type="expand"><summary>Title</summary><div>…</div></details>`
  - status: `<span data-type="status" data-color="grey|blue|green|yellow|red|purple">TEXT</span>`
  - toc: `<div data-type="toc"></div>` (viewer builds it from headings)
  - inline comment mark: `<span data-type="comment" data-id="<commentID>">…</span>`
  - image: `<img src="/api/v1/attachments/<id>/content" data-attachment-id="<id>" alt width>`
  - task list: `<ul data-type="taskList"><li data-type="taskItem" data-checked="true|false">`
  - code block: `<pre><code class="language-go">`

### Data model (Postgres)

```
users(id uuid pk, email citext unique, display_name, password_hash null, is_system_admin bool, disabled_at null, created_at, updated_at)
sessions(id text pk hashed token, user_id, expires_at, created_at)
oidc_identities(user_id, issuer, subject, pk(issuer,subject))
spaces(id uuid pk, key text unique upper, name, description, icon text null, visibility 'private'|'internal', home_page_id uuid null, created_by, created_at, updated_at, archived_at null, deleted_at null)
space_members(space_id, user_id, role 'admin'|'editor'|'viewer', created_at, pk(space_id,user_id))
pages(id uuid pk, space_id, parent_id null, title, slug, position int, status 'draft'|'published'|'trashed', version int default 0,
      content_json jsonb null, content_html text, content_text text, search_vector tsvector generated (title weight A, content_text weight B), 
      created_by, updated_by, created_at, updated_at, published_at null, trashed_at null, trashed_by null)
      indexes: (space_id,parent_id,position), gin(search_vector), (space_id,status)
page_versions(id uuid pk, page_id, version int, title, content_json, content_html, author_id, message text null, created_at, unique(page_id,version))
page_drafts(page_id, user_id, title, content_json, content_html, base_version int, updated_at, pk(page_id,user_id))
page_views(user_id, page_id, viewed_at, pk(user_id,page_id))
page_stars(user_id, page_id, created_at, pk(user_id,page_id))
labels(id uuid pk, name text unique lower, created_at)
page_labels(page_id, label_id, pk)
comments(id uuid pk, page_id, parent_id null, author_id, body_json jsonb, body_html text, inline_anchor jsonb null {quoted_text}, resolved_at null, resolved_by null, created_at, updated_at, deleted_at null)
attachments(id uuid pk, page_id, space_id, uploader_id, filename, content_type, size bigint, s3_key, created_at, deleted_at null)
templates(id uuid pk, space_id null (null = global), name, description, content_json, content_html, created_by, created_at, updated_at)
watches(user_id, target_type 'page'|'space', target_id, created_at, pk(user_id,target_type,target_id))
notifications(id uuid pk, user_id, type, actor_id, space_id null, page_id null, comment_id null, payload jsonb, read_at null, created_at) index(user_id, read_at, created_at desc)
activity(id uuid pk, space_id, page_id null, actor_id, type, payload jsonb, created_at) index(space_id, created_at desc)
```

### Permissions

- System admin: everything, implicit admin in every space.
- Space `admin`: space settings, members, archive, delete, templates, purge trash, plus everything editor can do.
- Space `editor`: create, edit, publish, move, trash, restore pages; attachments; labels; comments; templates (create/edit).
- Space `viewer`: view pages, history, attachments; create and edit own comments; star; watch.
- Visibility `internal`: any signed-in user is an implicit viewer. `private`: members only.
- Draft-status pages (never published) are visible only to their creator and space admins.
- The permission matrix lives in `api/internal/domain/permissions.go` as `Perm` constants and `RoleAllows(role, perm) bool`, mirroring Backlog's domain package. Middleware `ResolveSpaceRole` puts the effective role in context; `RequireSpacePerm(perm)` gates. Routes keyed by page id use a `ResolveSpaceIDFromPage` resolver (Backlog's `ResolveTeamIDFromLabel` pattern).

### Event bus

`events.Bus` with typed events defined in `domain/events.go`: `PagePublished{PageID, SpaceID, ActorID, Version, MentionedUserIDs}`, `PageMoved`, `PageTrashed`, `PageRestored`, `CommentCreated{CommentID, PageID, ActorID, ParentID, Inline bool, MentionedUserIDs}`, `CommentResolved`, `SpaceMemberAdded`, `NotificationCreated{UserID, NotificationID}`. Handlers publish after the transaction commits. `notify` subscribes and writes `notifications` and `activity` rows, then publishes `NotificationCreated`, which the SSE handler fans out per user. `events.Presence` tracks editors per page with a 30 s TTL.

### API (all under `/api/v1`, JSON, cookie session, same error envelope as Backlog)

Auth and users (foundation): `POST /auth/login`, `POST /auth/logout`, `GET /auth/me`, `PATCH /auth/me`, `POST /auth/change-password`, `GET /auth/oidc/config|login|callback`, `GET /users?q=` (for mentions, any signed-in user), `GET /users/{id}`.
Admin: `GET|POST /admin/users`, `PATCH /admin/users/{id}` (display name, email, password, is_system_admin, disabled), `GET /admin/spaces`.
Spaces: `GET /spaces` (accessible to caller, with role), `POST /spaces` (any signed-in user; creator becomes admin), `GET /spaces/{key}`, `PATCH /spaces/{key}`, `DELETE /spaces/{key}` (soft), `POST /spaces/{key}/archive|unarchive`, `GET|PUT|DELETE /spaces/{key}/members[/{userID}]`, `GET /spaces/{key}/tree`, `GET /spaces/{key}/activity`, `GET /spaces/{key}/trash`.
Pages: `POST /spaces/{key}/pages` {title, parent_id, template_id?} creates draft-status page; `GET /pages/{id}` (page + labels + breadcrumbs + children + caller's draft summary + editors present); `PATCH /pages/{id}` (title or metadata only for published pages); `PUT /pages/{id}/draft` {title, content_json, content_html, base_version} upsert caller's draft; `GET|DELETE /pages/{id}/draft`; `POST /pages/{id}/publish` {title, content_json, content_html, base_version, message} -> 409 `{code:"version_conflict", current_version}` when base_version != version unless `force:true`; `POST /pages/{id}/move` {parent_id, after_id|position}; `POST /pages/{id}/trash|restore`; `DELETE /pages/{id}` purge (admin); `POST /pages/{id}/copy`; `GET /pages/{id}/children`; `POST|DELETE /pages/{id}/star`; `POST /pages/{id}/editing` heartbeat returns `{editors:[...]}`; `GET /pages/{id}/versions`, `GET /pages/{id}/versions/{n}`, `POST /pages/{id}/versions/{n}/restore`; `GET /pages/{id}/export?format=md|html`; `GET /spaces/{key}/export` (zip of md); `POST /spaces/{key}/import` (multipart md files, parent_id).
Comments: `GET /pages/{id}/comments` (threads, inline and page, with resolved flag), `POST /pages/{id}/comments` {body_json, body_html, parent_id?, inline_anchor?} returns id used for the editor mark; `PATCH|DELETE /comments/{id}`; `POST /comments/{id}/resolve|reopen`.
Labels: `GET /labels?q=`, `GET|PUT /pages/{id}/labels` (replace set), `GET /spaces/{key}/labels`, `GET /spaces/{key}/labels/{name}/pages`.
Search: `GET /search?q=&space=&label=&author=&page=&limit=` returns hits with headline snippets; `GET /search/quick?q=` titles only.
Attachments: `POST /pages/{id}/attachments` multipart (field `file`) streamed to S3 under key `spaces/{spaceID}/pages/{pageID}/{attachmentID}/{filename}`, returns the attachment; `GET /pages/{id}/attachments`; `GET /attachments/{id}` (streams bytes, `Content-Disposition: attachment`); `GET /attachments/{id}/content` (streams inline with `Cache-Control: private, max-age=3600`, used by `<img>`); `DELETE /attachments/{id}`. Store rows have no `pending` state since upload and insert happen in one request; the object is deleted if the insert fails.
Templates: `GET /templates` (global + spaces caller can edit), `GET|POST /spaces/{key}/templates`, `POST /admin/templates`, `PATCH|DELETE /templates/{id}`.
Watch and notifications: `PUT|DELETE /watch/{type}/{id}`, `GET /watch/{type}/{id}`; `GET /notifications?unread=`, `POST /notifications/read` {ids|all}, `GET /notifications/stream` (SSE: `notification`, `unread_count` events).
Home: `GET /me/dashboard` (recent updates across accessible spaces, recently viewed, my drafts, starred).
Health: `GET /health` outside `/api/v1`.

### Web routes

```
/login                                  email+password, OIDC button
/                                       home dashboard
/spaces                                 directory + create
/spaces/[key]                           space home (renders home page) + sidebar tree
/spaces/[key]/pages/[id]/[slug]         page view
/spaces/[key]/pages/[id]/edit           editor
/spaces/[key]/pages/[id]/history        versions list, diff, restore
/spaces/[key]/pages/[id]/attachments
/spaces/[key]/labels/[name]
/spaces/[key]/activity
/spaces/[key]/trash
/spaces/[key]/settings/{general,members,templates}
/search
/notifications
/templates                              my templates (global + per space)
/account
/admin/{users,spaces,templates,labels}
```

Shell: left sidebar (space switcher, page tree with expand/collapse and drag move, quick create), top bar (global search box with quick-switcher, create page button, notifications bell, account menu), content area. Page view renders `content_html` inside a `prose` container with CSS for the custom node contract, sticky TOC on the right, breadcrumbs, labels, byline, actions menu (edit, move, copy, star, watch, export, history, attachments, trash), child pages, comment section, inline comment sidebar highlighting marks.

## Orchestration

### Rules for every agent brief

- Model: Sonnet. Fresh context. The brief is self-contained: goal, exact files owned, files it must not touch, the contracts it must honour (pasted from this plan), reference files in Backlog, the gate command to run before reporting, and the notes file to write.
- No comments in code (Matt's global rule). Put explanations in `.claude/pm/<task>/notes.md`.
- Agents never run `git`. I commit at the end of each phase.
- Agents never add dependencies. `go.mod` and `package.json` are fully pinned by the foundation agents. An agent that believes it needs a dependency reports it in its notes and stops that sub-task.
- Agents only create or edit files they own. Shared wiring is solved structurally: every handler file exposes `Routes(r chi.Router)` and the foundation router mounts every resource once, including stubs; every web resource has its own `lib/api/<resource>.ts` module; migrations are pre-assigned number ranges.
- Each agent writes `.claude/pm/<task>/notes.md`: what was built, deviations from the brief, gate output, anything blocked.
- API gate: `cd api && go build ./... && go vet ./... && go test ./...`. Web gate: `cd web && npm run lint && npx tsc --noEmit && npm run build`. Integration tests use testcontainers-go Postgres and skip cleanly when Docker is absent.

### Review protocol (mine)

After each wave: run both gates myself; read every migration, every `Routes` function, every middleware chain for authz; spot-check store SQL for parameterisation and permission joins; check the notes for deviations; spawn one Sonnet review agent per group with a checklist (authz on every route, injection, N+1, contract drift, error envelope, no comments, no new deps). Remediation goes back to the original coding agent by SendMessage. Two rounds max on the same issue, then I re-brief the task. Then commit the phase.

### Migration number ranges

foundation 0001–0009 · spaces 0010–0019 · pages 0020–0029 · comments 0030–0039 · labels+search 0040–0049 · attachments 0050–0059 · templates 0060–0069 · notify 0070–0079 · export 0080–0089 · integration fixes 0090+

### Phase 0 (me): repo init

`git init` on `main`, first commit after Phase 1. Create `.claude/pm/`.

### Phase 1: foundation (3 agents in parallel)

**A1 api-foundation.** Owns `api/` entirely at this stage. Delivers: `go.mod` pinned with chi v5, pgx v5, goose v3, uuid, x/crypto, go-oidc v3, oauth2, aws-sdk-go-v2 (config, credentials, s3), bluemonday, goldmark, x/time, testcontainers-go + postgres module; `cmd/server/main.go` (migrate, bootstrap, serve, graceful shutdown, `healthcheck` subcommand, hourly session cleanup); `config`; `domain` (all types above, `Perm` matrix, event types); `auth`; `bootstrap`; `store/store.go` + `users.go` + `sessions`; `storage` S3 client behind an `ObjectStore` interface (Put, Get, Delete) with a `FromEnv` that returns nil when `STORAGE_S3_BUCKET` is unset; `events` bus, SSE hub, presence; `http` router with CORS, request logging, rate limiter, `RequireAuth`, `RequireSystemAdmin`, `ResolveSpaceRole`, `RequireSpacePerm`, `ResolveSpaceIDFromPage` (reads `pages.space_id` directly); handlers `auth.go`, `oidc.go`, `admin.go` (users only), `users.go`, `health`; stub handler files for spaces, pages, comments, labels, search, attachments, templates, watch, notifications, export, dashboard, each with an empty `Routes`, all mounted in `router.go`; `content/sanitize.go` (bluemonday policy per the contract) and `content/text.go` (JSON to text); migrations 0001 users+sessions+oidc, 0002 spaces+members (schema only, so middleware compiles), 0003 pages (schema only, so resolver compiles); `Dockerfile` (multi-stage, distroless static, uid 65532, `VERSION` arg via `-ldflags`); `testutil` package that starts a testcontainers Postgres and runs migrations; tests for auth, permission matrix, sanitiser, middleware. Gate passes.

**A2 web-foundation.** Owns `web/`. Delivers: Next.js 15 app (`output: "standalone"`, CSP and security headers as Backlog's `next.config.js`, class-based dark mode with `ThemeProvider` as Backlog's `lib/theme.tsx`) with pinned `package.json` including every dependency later waves need (TipTap core, react, starter-kit, table family, task-list family, code-block-lowlight + lowlight, image, link, placeholder, mention, suggestion, underline, highlight, text-align, typography, bubble-menu, floating-menu, `tippy.js`, `@tailwindcss/typography`, `@tanstack/react-query`, `@dnd-kit/*`, `diff`, `date-fns`, `lucide-react`; zip export is server-side so no client zip library); Tailwind config; `lib/api/client.ts` (fetch with credentials, error envelope, same shape as Backlog's `api.ts`), `lib/api/types.ts` (every type from the data model and API contract), one stub module per resource re-exported from `index.ts`; `lib/sse.ts`; layout components `AppShell`, `Sidebar` (space switcher + tree slot), `TopBar` (search slot, bell slot, account menu), `ui/` primitives (Button, Input, Modal, Dropdown, Badge, Avatar, Toast, EmptyState, Spinner); `/login`, `/account`, `/admin/users`, root layout with auth redirect, a placeholder `/`; `Dockerfile` matching Backlog's; eslint + tsconfig. Gate passes.

**A3 repo-infra.** Owns root files, `.github/`, `docs/` skeleton. Delivers: `docker-compose.yml` (db, minio, minio-createbucket, api, web) with Knowledge names and ports api 8080 / web 3000; `.env.example` with the Backlog variable set (same names, minus `STORAGE_S3_PUBLIC_ENDPOINT`, plus `COOKIE_SECURE`, `ATTACHMENT_MAX_BYTES`, `STORAGE_S3_BUCKET=knowledge-dev`); compose api healthcheck uses `["/app/server","healthcheck"]`; `.gitignore`; `CLAUDE.md` (stack, commands, conventions, no comments, file ownership note); `README.md` draft; `docs/` placeholders; `.github/workflows/pull_request.yaml` (paths-filter; api: go vet + test with Docker available for testcontainers; web: npm ci, lint, tsc, build) and `merge.yaml` copied from Backlog's multi-arch publish (amd64 on `ubuntu-latest`, arm64 on `ubuntu-24.04-arm`, per-arch tags `main-<arch>` and `<sha>-<arch>`, then `imagetools create` manifests for `main` and `<sha>`), with `api` and `web` contexts and the `VERSION` build arg actually consumed. A `.claude/settings.local.json` allowlist for the build, test, compose, and curl commands agents will run (no credentials in it).

### Phase 2a: core API + editor (6 agents in parallel)

**B1 spaces.** Owns `store/spaces.go`, `handlers/spaces.go`, migration 0010 (space indexes, `home_page_id` FK deferred), tests. Spaces CRUD, key validation (`^[A-Z][A-Z0-9]{1,9}$`), members, archive, directory listing with caller role, `tree` endpoint (all non-trashed pages the caller may see, ordered by position), space trash listing. Creating a space creates its home page (published, title = space name) and sets `home_page_id`.

**B2 pages.** Owns `store/pages.go`, `store/versions.go`, `store/drafts.go`, `handlers/pages.go`, `handlers/versions.go`, migrations 0020–0022, tests. Everything under Pages in the API contract except export/import and attachments. Slug from title. Position assignment and reordering with `after_id`. Move validates no cycle. Trash cascades to subtree and records `trashed_by`; restore reattaches to parent or space root if parent is trashed. Publish: sanitise HTML, extract text, bump version, write `page_versions`, delete caller's draft, publish `PagePublished` with mentioned user ids parsed from JSON. Restore version = publish with that version's content and message "Restored version n". Presence heartbeat. `page_views` upsert on GET. Stars.

**B3 comments.** Owns `store/comments.go`, `handlers/comments.go`, migration 0030, tests. Threads, inline anchors, resolve, soft delete keeps thread shape, author-or-admin edit/delete rules, `CommentCreated`/`CommentResolved` events with mentions.

**B4 labels and search.** Owns `store/labels.go`, `store/search.go`, `search/`, `handlers/labels.go`, `handlers/search.go`, migration 0040 (labels, page_labels, GIN index if not in 0003), tests. FTS with `websearch_to_tsquery`, `ts_headline`, permission join against accessible spaces, filters, ranked, paginated. Quick search by title prefix/ILIKE.

**B5 attachments.** Owns `store/attachments.go`, `handlers/attachments.go`, migration 0050, tests with a fake storage client (the `storage` package exposes an interface `ObjectStore` with `Put`, `Get`, `Delete` so B5 can fake it). Proxied multipart upload streamed to S3, streamed download and inline content endpoints, size limit from config, content-type sniffed and allowlisted for inline rendering (images only inline; everything else forced to attachment), deletion removes the object.

**C1 editor (web).** Owns `web/components/editor/**`, `web/lib/content/**`, `web/app/spaces/[key]/pages/[id]/edit/page.tsx`, `web/lib/api/pages.ts` and `attachments.ts` and `users.ts` (implements the contract). TipTap editor with every extension and custom node in the content contract, slash menu, bubble menu, table controls, image upload via attachments API (drop, paste, toolbar), mention suggestion via `GET /users?q=`, page-link suggestion via `GET /search/quick`, inline comment mark (mark only; comment UI is C3), title field, autosave draft debounced 1.5 s with status indicator, publish dialog with message, 409 conflict dialog (reload latest vs force), presence banner from the heartbeat, unsaved-changes guard, `prose` styles for all custom nodes in `web/app/globals.css` (shared with the viewer). Works against the API contract even before B2 is reviewed; integration verified in Phase 4.

### Phase 2b: remaining API + web features (6 agents in parallel, after 2a review)

**B6 templates.** Owns `store/templates.go`, `handlers/templates.go`, migration 0060 + seed of four starter templates, tests. Create page from template handled by B2's `POST /spaces/{key}/pages` reading `template_id`; B6 provides `store.GetTemplateContent` which B2 calls (B2 leaves a clearly named call site; B6 fills the function in its own file).

**B7 watch, notifications, activity, dashboard.** Owns `notify/`, `store/notifications.go`, `store/watches.go`, `store/activity.go`, `handlers/watch.go`, `handlers/notifications.go`, `handlers/dashboard.go`, `handlers/activity.go`, migration 0070, tests. Subscribes to the bus; rules: page published notifies watchers of page and ancestors and space (not the actor), mentions notify mentioned users, comment notifies page watchers and thread participants, reply notifies parent author. Auto-watch a page you create or comment on. SSE stream per user. Dashboard query.

**B8 export and import.** Owns `content/markdown.go`, `content/import.go`, `handlers/export.go`, migration none, tests with fixture documents. JSON to Markdown for every node in the contract, standalone HTML export with inline CSS, space zip export preserving tree as folders, Markdown import via goldmark with GFM, one page per file, nested folders become child pages.

**C2 spaces and pages UI.** Owns `web/app/spaces/**` except edit and labels, `web/app/page.tsx` (dashboard), `web/components/spaces/**`, `web/components/pages/**`, `web/lib/api/spaces.ts`, `versions.ts`, `dashboard.ts`. Space directory and create dialog, space home, sidebar tree with lazy expansion state, drag move with dnd-kit and a move dialog fallback, page view (breadcrumbs, byline, labels slot, TOC, child pages, actions menu, star, watch toggle slot, export menu, "you have a draft" banner), history with diff (word-level using `diff` on text and an HTML side-by-side view), restore, trash view, space settings (general, members, archive), copy page, move page, home dashboard.

**C3 comments, labels, search UI.** Owns `web/components/comments/**`, `web/components/labels/**`, `web/components/search/**`, `web/app/search/**`, `web/app/spaces/[key]/labels/**`, `web/lib/api/comments.ts`, `labels.ts`, `search.ts`. Page comment thread with a small TipTap instance for bodies, inline comment sidebar that maps `data-type="comment"` spans in the rendered page to threads, highlight on hover, resolve/reopen, label editor on page view, label pages view, search page with filters and snippets, top-bar quick switcher with keyboard navigation.

**C4 notifications, templates, admin, export UI.** Owns `web/components/notifications/**`, `web/components/admin/**`, `web/app/notifications/**`, `web/app/templates/**`, `web/app/admin/**` (extends A2's users page), `web/app/spaces/[key]/settings/templates/**`, `web/app/spaces/[key]/activity/**`, `web/app/spaces/[key]/attachments` route, `web/lib/api/notifications.ts`, `templates.ts`, `admin.ts`, `export.ts`, `attachmentsList.ts`. Bell with unread count over SSE, notifications page, watch toggle component, templates gallery and editor (reuses C1 editor), create-from-template in the new page dialog, space activity feed, attachments page, export and import dialogs, admin spaces/templates/labels pages.

### Phase 3: integration and quality (sequential waves)

**D1 integration.** Owns `web/e2e/**`, `playwright.config.ts`, compose tweaks, and any file needed to make the whole thing run together (reported file by file). Brings up `docker compose up --build`, fixes cross-agent mismatches (type drift between `lib/api/types.ts` and handlers, route mounts, CSS), writes Playwright e2e: login, create space, create page, edit with slash menu, upload image, publish, view, comment inline, search finds page, history shows diff and restore, trash and restore, notification received, OIDC config endpoint. Adds the e2e job to `pull_request.yaml` using compose.

**D2 docs.** Owns `README.md`, `docs/*.md`. Writes the Backlog-style README, API reference generated by reading every `Routes` function, configuration reference from `config`, development guide, and `DEPLOYMENT.md` describing the pi-lab stack to write later (namespace egress to rustfs, `cluster_access` on the rustfs ingress, secrets in OpenBao `secret/knowledge/manual`, image tags, probes, uids).

**D3 reviews (3 parallel Sonnet reviewers, then fixes by original agents).** Security review of the API (authz per route, sanitiser bypasses, presigned key scoping, rate limits, SSE auth), correctness review of stores and migrations (down migrations, indexes, cascades), web review (XSS in rendering, broken contracts, accessibility basics, mobile layout).

**Final (me).** Compare the running app against the Features list, not against the agent briefs. Anything missing becomes a new brief. Commit, then write memory notes about project decisions.

## Verification

1. `cd api && go build ./... && go vet ./... && go test ./...` passes with Docker running (integration tests execute).
2. `cd web && npm run lint && npx tsc --noEmit && npm run build` passes.
3. `cp .env.example .env && docker compose up --build`; open `http://localhost:3000`, sign in as the bootstrapped admin, and walk the Features list manually: create space `ENG`, create nested pages, use every slash command item, upload an image, publish, see a version, edit again and publish, diff and restore, comment inline and on the page, label, search, watch from a second user and receive a notification, export Markdown, import a Markdown file, trash and restore, archive the space, admin create user and reset password, OIDC config endpoint returns disabled when unset.
4. `npx playwright test` against the compose stack passes.
5. `docker buildx build --platform linux/arm64 api` and `web` succeed locally (multi-arch sanity before CI).
6. Image constraints: api container starts with a read-only root filesystem and uid 65532; web with uid 1000 (`docker run --read-only --user 65532 ...`).
