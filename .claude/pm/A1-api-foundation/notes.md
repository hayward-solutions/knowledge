# A1 api-foundation: notes

Module `github.com/hayward-solutions/knowledge/api`, `go 1.25.0`. Everything below lives under `api/`. Gate passes (output at the end).

## What was built

- `cmd/server/main.go`: `var version` (set with `-ldflags -X main.version=`), `healthcheck` subcommand (GET `http://127.0.0.1:$API_PORT/health`, exit 0/1), otherwise config.Load -> wait for DB (30 s) -> goose Up from `MIGRATIONS_DIR` -> pgxpool -> `bootstrap.Admin` -> `auth.LoadOIDC` -> `storage.New` -> Bus/Hub/Presence -> router -> http server with SIGINT/SIGTERM graceful shutdown (10 s) -> hourly `DeleteExpiredSessions` goroutine (also runs once at start).
- `internal/config`: `Config`, `Load()`, `LoadFrom(getenv)`, `LoadStorage()`. Env names exactly as specified. Fields: `PostgresDSN, APIPort, MigrationsDir, AppOrigins []string, PublicBaseURL, CookieSecure, Admin AdminSettings, OIDC OIDCSettings, Storage StorageSettings, AttachmentMaxBytes int64` (default 26214400). Errors on missing `POSTGRES_DSN`, bad port, bad `ATTACHMENT_MAX_BYTES`.
- `internal/domain`: `types.go` (all tables), `permissions.go`, `events.go`.
- `internal/auth`: `password.go` (argon2id m=32768 KiB, t=3, p=2, PHC `$argon2id$v=19$m=..,t=..,p=..$salt$hash`, `ValidatePassword`, `HashPassword`, `VerifyPassword`, `DummyVerify` for login timing), `oidc.go` (`LoadOIDC(ctx, config.OIDCSettings) (*OIDCConfig, error)`, `GroupsFrom`, `HasGroup`).
- `internal/bootstrap`: `Admin(ctx, store, config.AdminSettings) error`.
- `internal/store`: `store.go`, `users.go`, `sessions.go`, `space_access.go`.
- `internal/storage`: `ObjectStore` interface + S3 implementation.
- `internal/events`: `Bus`, `Hub`, `Presence`.
- `internal/content`: `sanitize.go`, `text.go`, `mentions.go`.
- `internal/http`: `router.go`, `middleware/{auth,space,ratelimit}.go`, `handlers/{deps,util,auth,oidc,users,admin}.go` plus 13 stub handler files.
- `migrations/0001_users_sessions_oidc.sql`, `0002_spaces.sql`, `0003_pages.sql` (Up and Down).
- `internal/testutil`, `Dockerfile`, `.dockerignore`.
- `internal/deps/deps.go`: blank imports of goldmark (`goldmark`, `extension`, `renderer/html`) so `go mod tidy` keeps it pinned until B8 imports it for real. B8 may delete this file once it imports goldmark itself; it must not touch go.mod.

## Routing convention (for every later agent)

Each handler type embeds `Deps` and has `Routes(r chi.Router)`. `router.go` calls every `Routes` once inside a single `r.Route("/api/v1", ...)`. Register FLAT full paths relative to `/api/v1` with `r.With(mws...).Method(pattern, fn)`. Never `r.Route` or `r.Mount` inside `Routes`. `internal/http/routing_test.go` proves several handlers can register under `/spaces/{key}/...` and `/pages/{pageID}` side by side, and that different param names at the same tree position are fine.

URL param names the middleware reads: `{key}` (space key, upper-cased by the middleware, so `eng` works), `{pageID}`. Other names are free (`{userID}`, `{commentID}`, `{attachmentID}`, `{templateID}`...). `GET /admin/spaces` and `POST /admin/templates` were not registered in `admin.go` (users only per brief); B1 / B6 register them in their own `Routes` with `mw.RequireAuth(h.Store), mw.RequireSystemAdmin`.

Wiring without touching `router.go` or `main.go`:
- Every stub is `type XHandler struct{ Deps }` constructed in `router.go` as `&handlers.XHandler{Deps: d}`, so any handler can use `h.Store, h.Bus, h.Hub, h.Presence, h.Storage, h.Config, h.OIDC, h.Version` without router edits.
- `NotificationHandler` has a no-op `Subscribe()` that `NewRouter` calls once at construction. B7 fills it in (`h.Bus.Subscribe(...)`, writing notifications/activity, then `h.Hub.Publish(userID, events.Event{Kind: "notification", ...})` and publishing `domain.NotificationCreated`). Nothing else in `main.go` needs to change for the notify package.
- Stub files in `internal/http/handlers/`: `spaces.go (SpaceHandler)`, `pages.go (PageHandler)`, `versions.go (VersionHandler)`, `comments.go (CommentHandler)`, `labels.go (LabelHandler)`, `search.go (SearchHandler)`, `attachments.go (AttachmentHandler)`, `templates.go (TemplateHandler)`, `watch.go (WatchHandler)`, `notifications.go (NotificationHandler)`, `activity.go (ActivityHandler)`, `dashboard.go (DashboardHandler)`, `export.go (ExportHandler)`. Each agent replaces only its own file(s). Keep the struct name and the `Deps` embed.

## Exact helper names and signatures

### handlers package (unexported, same package only), `handlers/util.go`
```
writeJSON(w http.ResponseWriter, status int, v any)
readJSON(r *http.Request, v any) error               // 1 MiB limit via MaxBytesReader
urlUUID(r *http.Request, key string) (uuid.UUID, error)
httpErr(w http.ResponseWriter, status int, msg string)                    // {"error": msg}
httpErrCode(w http.ResponseWriter, status int, code, msg string)          // {"error": msg, "code": code}
internalErr(w http.ResponseWriter, r *http.Request, err error, msg string) // logs, 500 {"error": msg or "internal error"}
queryInt(r *http.Request, key string, def, min, max int) int
normalizeEmail(s string) (string, bool)
cleanName(s string) string
```
For the 409 version conflict, call `writeJSON(w, 409, map[string]any{"error": "...", "code": "version_conflict", "current_version": n})`.

### handlers.Deps
```
type Deps struct {
    Store *store.Store; Bus *events.Bus; Hub *events.Hub; Presence *events.Presence
    Storage storage.ObjectStore   // nil when STORAGE_S3_BUCKET unset: attachment handlers must answer 503
    Config *config.Config; OIDC *auth.OIDCConfig; Version string
}
```

### middleware (`mw "github.com/hayward-solutions/knowledge/api/internal/http/middleware"`)
```
RequireAuth(s *store.Store) func(http.Handler) http.Handler        // 401 {"error":"unauthenticated"}
OptionalAuth(s *store.Store) func(http.Handler) http.Handler
RequireSystemAdmin(next http.Handler) http.Handler                 // plain middleware, not a factory; needs RequireAuth first; 403 forbidden
SpaceByKey(s *store.Store, perm domain.Perm) func(http.Handler) http.Handler
SpaceByPage(s *store.Store, perm domain.Perm) func(http.Handler) http.Handler
SpaceByResolver(s *store.Store, resolve func(*http.Request) (uuid.UUID, error), perm domain.Perm) func(http.Handler) http.Handler
RequireSpacePerm(perm domain.Perm) func(http.Handler) http.Handler
UserFrom(ctx context.Context) *domain.User                         // nil if unauthenticated
SpaceFrom(ctx context.Context) *SpaceCtx                           // nil outside the Space* chain
SessionTokenFrom(ctx context.Context) (string, bool)              // raw cookie token (for SSE re-validation: store.GetValidSession)
URLUUID(r *http.Request, param string) (uuid.UUID, error)         // wraps ErrInvalidID; use inside resolvers
WriteError(w http.ResponseWriter, status int, msg string)
AuthLimiter() func(http.Handler) http.Handler                      // per-IP, burst 10, refill 10/min, 429 + Retry-After
NewSessionCookie(token string, expires time.Time) *http.Cookie; ClearSessionCookie() *http.Cookie
var CookieSecure bool (set by NewRouter from Config.CookieSecure); const SessionCookie = "sid"
var ErrInvalidID error                                             // resolver returns this (or wraps it) -> 400; store.ErrNotFound -> 404
type SpaceCtx struct { ID uuid.UUID; Key string; Role domain.Role; Visibility domain.Visibility; Archived bool; Deleted bool }
func (*SpaceCtx) ReadOnly() bool
```
`SpaceByKey/SpaceByPage/SpaceByResolver` already include `RequireAuth`, so use them alone: `r.With(mw.SpaceByKey(h.Store, domain.PermEditPages)).Post("/spaces/{key}/pages", h.Create)`. For a resolver, return the space id for the entity (e.g. `func(r) { id, err := mw.URLUUID(r, "commentID"); ...; return h.Store.GetCommentSpaceID(ctx, id) }`; return `store.ErrNotFound` for a missing row).

Role resolution: system admin -> admin; else member role; else viewer if space visibility is `internal`; else 403 `forbidden`. Unknown key / page / resolver miss -> 404 `not found`. Malformed id -> 400 `invalid id`. Archived spaces: anyone whose effective role is not `admin` gets 403 `space is read-only` for every perm except `PermViewSpace` (so comments are blocked too). Space admins and system admins keep full access to archived spaces (they need it to unarchive). Draft-page visibility (creator and space admins only) is NOT enforced by middleware; B2 must do it in handlers.

### store (additions beyond the brief that other agents can use)
```
New(pool) *Store; (*Store).Pool; WithTx(ctx, func(pgx.Tx) error) error
ErrNotFound, ErrConflict, ErrLastAdmin; IsUniqueViolation(err), IsForeignKeyViolation(err)
CreateUser(ctx, domain.User, passwordHash string) error                       // ErrConflict on duplicate email
CountUsers, CountActiveSystemAdmins
GetUser(ctx, id) (domain.User, error)                                         // no PasswordHash
GetUserByEmail(ctx, email) (domain.User, error)                               // PasswordHash populated (nil for SSO-only users)
GetUserWithHash(ctx, id) (domain.User, error)
ListUsers(ctx) ([]domain.User, error)
SearchUserRefs(ctx, q string, limit int) ([]domain.UserRef, error)           // active users, LIKE wildcards escaped, never nil
GetUserRef(ctx, id) (domain.UserRef, error); GetUserRefs(ctx, ids []uuid.UUID) (map[uuid.UUID]domain.UserRef, error)
UpdateUserProfile(ctx, id, email, displayName *string) (domain.User, error)
AdminUpdateUser(ctx, id, store.UserUpdate{Email, DisplayName *string; IsSystemAdmin, Disabled *bool}) (domain.User, error)   // ErrLastAdmin enforced under row locks
SetUserPassword(ctx, id, hash) error
UpsertOIDCUser(ctx, issuer, subject, email, displayName string, isSystemAdmin *bool) (domain.User, error)
CreateSession(ctx, userID, expiresAt) (rawToken string, err error)
GetValidSession(ctx, rawToken) (domain.Session, error)
DeleteSession(ctx, rawToken) error; DeleteUserSessions(ctx, userID) error; DeleteExpiredSessions(ctx) (int64, error)
NewSessionToken() (string, error); HashSessionToken(token) string              // hex sha256
GetSpaceAccessByKey(ctx, key, userID) / GetSpaceAccessByID(ctx, spaceID, userID) / GetSpaceAccessByPage(ctx, pageID, userID) (store.SpaceAccess, error)
GetPageSpaceID(ctx, pageID) (uuid.UUID, error)
```
Sessions: `sessions.id` is `hex(sha256(token))`; the cookie `sid` carries the raw token (43 chars base64url). 30-day lifetime is `domain.SessionTTL`.

`store/spaces.go`, `pages.go` etc. are not created; B1/B2 own them and should reuse `notFoundIf`-style handling (`pgx.ErrNoRows` -> `store.ErrNotFound`).

### events
```
events.NewBus() *Bus
(*Bus).Subscribe(h func(any))        // every handler sees every event; type-switch inside
(*Bus).Publish(event any)            // one goroutine per handler, panics recovered and logged
(*Bus).Wait()                        // blocks until in-flight handlers finish (use in tests, called at shutdown)
events.Event{Kind string `json:"kind"`; Payload any `json:"payload"`}
events.NewHub() *Hub
(*Hub).Subscribe(userID uuid.UUID) (chan events.Event, func())   // buffered 16; the func unsubscribes and closes the channel (idempotent)
(*Hub).Publish(userID uuid.UUID, ev events.Event)                 // non-blocking, drops when full
(*Hub).Subscribers(userID) int
events.NewPresence() *Presence       // 30 s TTL, 10 s sweep; call Close() on shutdown
events.NewPresenceWithTTL(ttl, sweepEvery time.Duration) *Presence
(*Presence).Touch(pageID uuid.UUID, user domain.UserRef)
(*Presence).Editors(pageID uuid.UUID) []domain.UserRef            // sorted by display name, never nil; callers drop themselves if they want
(*Presence).Remove(pageID, userID uuid.UUID)
```
Event structs are in `domain/events.go` and are published by value (not pointer): `domain.PagePublished`, `PageMoved`, `PageTrashed`, `PageRestored`, `CommentCreated`, `CommentResolved`, `SpaceMemberAdded`, `NotificationCreated`. Subscribers should `switch ev := e.(type) { case domain.PagePublished: ...}`.

### storage
```
type ObjectStore interface {
    Put(ctx context.Context, key, contentType string, r io.Reader, size int64) error
    Get(ctx context.Context, key string) (io.ReadCloser, string, int64, error)   // body, content type, size
    Delete(ctx context.Context, key string) error
}
storage.ErrNotFound                       // Get on a missing key
storage.New(ctx, config.StorageSettings) (ObjectStore, error)   // (nil, nil) when Bucket == ""
storage.FromEnv(ctx) (ObjectStore, error)                       // reads env via config.LoadStorage()
storage.NewS3(ctx, cfg) (*S3Store, error)
```
`Put` accepts non-seekable readers (needed for plain-http MinIO): readers up to 8 MiB are buffered in memory, larger ones spool to a temp file under `os.TempDir()` (so `/tmp` must be writable, as it is in the planned deployment). `Put` fails with `io.ErrUnexpectedEOF` if the reader yields fewer bytes than `size`. When `Endpoint` is set the SDK checksum mode is `WhenRequired` for compatibility with MinIO/RustFS. Tests for handlers should use a fake `ObjectStore` (map-backed); `storage_test.go` shows an httptest fake S3.

### content
```
content.SanitizeHTML(html string) string
content.Policy() *bluemonday.Policy
content.ExtractText(contentJSON []byte) string            // "" for nil/null/invalid
content.ExtractTextFromHTML(html string) string
content.MentionedUserIDs(contentJSON []byte) []uuid.UUID  // deduplicated, never nil
```
Sanitiser specifics worth knowing: `data-id`, `data-page-id`, `data-attachment-id` must be UUIDs; `data-color` must be one of info|note|success|warning|error|grey|blue|green|yellow|red|purple; `data-type` is checked per element (span: mention|status|comment, div: panel|toc, ul: taskList, li: taskItem, a: page-link, details: expand); `class` survives only on `code` as `language-*`; heading `id` must match `^[A-Za-z][A-Za-z0-9_\-:.]{0,99}$`; `width`/`height` on img must be numeric with optional px or %; `rel` only from noopener|noreferrer|nofollow|ugc, `target` only `_blank|_self`. `style` and every other attribute is dropped. Mention label text: `ExtractText` uses the mention node's `attrs.label` (falls back to `text`/`alt`), prefixing `@`.

### testutil
```
testutil.NewDB(t *testing.T) *store.Store
testutil.NewDBWithDSN(t) (*store.Store, string)
testutil.DefaultPassword = "Correct-Horse-9-Battery"
testutil.CreateUser(t, s, email, password string, admin bool) domain.User     // empty password -> DefaultPassword; hashes are cached
testutil.NewSession(t, s, userID) string                                      // raw token for the sid cookie
testutil.CreateSpace(t, s, testutil.SpaceOpts{Key, Visibility, Archived, Deleted, CreatedBy}) uuid.UUID
testutil.AddMember(t, s, spaceID, userID, role)
testutil.CreatePage(t, s, spaceID, createdBy, title) uuid.UUID               // published page, raw SQL
```
`NewDB` starts one shared `postgres:16-alpine` testcontainer per test binary (sync.Once, reaped by Ryuk at process exit), creates a fresh randomly named database per call, runs all migrations with a goose Provider, and drops the database on cleanup. It calls `t.Skip` through `testcontainers.SkipIfProviderIsNotHealthy` when Docker is unreachable. Setting `KNOWLEDGE_TEST_POSTGRES_DSN` uses that server instead of a container (it needs CREATEDB rights). The helper fixtures use raw SQL on `s.Pool` so they keep working before `store/spaces.go` exists.
In `handlers_test` (external test package of `internal/http/handlers`) `helpers_test.go` defines `newEnv(t, mutate)`, `env.client()`, `client.do(method, path, body) response`, `client.login/mustLogin`. These run the full router over `httptest.Server` with a cookie jar and a unique `X-Forwarded-For` per client so the auth limiter does not leak between tests. Later agents can add `*_test.go` files to package `handlers_test` and reuse them (do not edit `helpers_test.go`; if a helper is missing, define a new one in your own test file).

## Endpoints implemented (all under /api/v1)

`POST /auth/login` (limited), `POST /auth/logout` (idempotent, always 204, clears cookie), `GET|PATCH /auth/me`, `POST /auth/change-password` (limited), `GET /auth/oidc/config|login|callback`, `GET /users?q=&limit=` (UserRef list, limit 1-50 default 20, active users only), `GET /users/{userID}` (UserRef), `GET|POST /admin/users`, `PATCH /admin/users/{userID}`. Plus `GET /health` -> `{"ok":true,"version":...}`. Unknown routes and wrong methods return the JSON envelope (`not found`, `method not allowed`).

## Deviations from the plan/brief and why

1. Dependency pins. The newest `golang.org/x/*` (crypto v0.58, net v0.62, oauth2 v0.37, sync, sys, text, time v0.16...), goose v3.28+/v3.27.1+, and OpenTelemetry v1.47 all declare `go 1.26`/`1.25.7`, which would have forced the `go` directive above `1.25.0` and broken the `golang:1.25-alpine` build. Pinned to the newest versions compatible with `go 1.25.0`: goose v3.27.0, x/crypto v0.55.0, x/net v0.58.0, x/oauth2 v0.36.0, x/sync v0.22.0, x/sys v0.47.0, x/text v0.41.0, x/time v0.15.0 (tidy also resolved otel to older compatible versions). Everything else is latest: chi v5.5.0, pgx v5.11.0, uuid v1.6.0, go-oidc v3.21.0, aws-sdk-go-v2 v1.47.5 (config v1.33.12, credentials v1.20.12, s3 v1.114.6), bluemonday v1.0.27, goldmark v1.8.7, testcontainers-go v0.44.0 + modules/postgres v0.44.0. `golang.org/x/net` is a direct dependency (HTML tokenizer in `ExtractTextFromHTML`). Bumping the x/* set later requires bumping the Dockerfile builder to Go 1.26 first.
2. `domain.User.PasswordHash` is a `*string` tagged `json:"-"`. `GetUser` does not load it; only `GetUserByEmail` / `GetUserWithHash` do. `User.DisabledAt` and other nullable fields serialise as `null` (no `omitempty`) so the web types are stable. `ContentText` on `Page` is `json:"-"`; `Attachment.S3Key` is `json:"-"` (internal keys are not an API field).
3. Session API is token-based: `CreateSession(ctx, userID, expiresAt) (token, error)`, `GetValidSession(ctx, token)`, `DeleteSession(ctx, token)`. Context holds the raw token (`SessionTokenFrom`), not a session id.
4. Deleted spaces: for non-system-admins a soft-deleted space is `404 not found` for every perm including view (the brief said read-only). A soft-deleted space has no business being readable by members; system admins still resolve it as `admin`. `SpaceCtx` has an extra `Deleted` field.
5. `RequireSystemAdmin` is a plain middleware (not a factory) and relies on `RequireAuth` having run; `Space*` chains include `RequireAuth` themselves.
6. OIDC admin flag: `UpsertOIDCUser` takes `isSystemAdmin *bool`. The callback passes nil when `OIDC_ADMIN_GROUP` is unset, so SSO logins no longer demote the bootstrap or locally promoted admin (Backlog demotes everyone in that case). When the group is set, membership is authoritative on every login, as in Backlog. Email changes from the IdP are applied unless they collide with another user. OIDC default post-login redirect is `/`. `oidc_identities` keyed by `(issuer, subject)` per the plan (Backlog used a column on users).
7. Login verifies the password before reporting `account disabled` (no disabled-account enumeration), runs a dummy argon2 verify for unknown or password-less users, and rejects SSO-only accounts with `invalid credentials`. Admin password reset revokes all of that user's sessions. Admin create requires a valid email and the password policy; duplicate email gives 409.
8. The password policy is Backlog's (12+ chars, upper, lower, digit, symbol) counted in runes with Unicode-aware classes and a 256-byte cap to bound hashing cost.
9. Events carry a few extra fields beyond the plan list so subscribers need no extra lookups: `SpaceID` on `CommentCreated`/`CommentResolved`, `OldParentID`/`NewParentID` on `PageMoved`, `Role`/`ActorID` on `SpaceMemberAdded`.
10. `storage.New(ctx, cfg.Storage)` is what `main.go` calls (single Config); `storage.FromEnv` exists and behaves per the brief. `FromEnv`/`New` return the `ObjectStore` interface with an untyped nil, so `if h.Storage == nil` is reliable.
11. `Presence` has `Close()` (stops the sweeper) and `Remove()`; `Bus` has `Wait()`; `Hub` has `Subscribers()`. All additive.
12. CORS allows `PUT` too (Backlog's list lacked it; Knowledge uses PUT for drafts, members, labels, watch).
13. Migrations: a few supporting indexes beyond the plan (`sessions(user_id)`, `sessions(expires_at)`, `oidc_identities(user_id)`, `space_members(user_id)`, `page_drafts(user_id, updated_at desc)`, `page_views(user_id, viewed_at desc)`, `page_stars(page_id)`). `spaces.visibility` defaults to `internal`; `spaces.key` has `CHECK (key = upper(key))`. `pages.parent_id` and `pages.space_id` cascade on delete, `spaces.home_page_id` is `ON DELETE SET NULL`. Down migrations drop everything in reverse order (`citext` extension is left in place); a test runs full down then up.
14. There are no Go comments anywhere and no SQL comments (only the `-- +goose Up/Down` directives). The brief's "explain in notes" is satisfied by this file.

## Things later agents should know

- The stub handlers register nothing yet, so every `/api/v1/spaces...` route 404s with the JSON envelope until B1 lands.
- `Config.AttachmentMaxBytes` and `h.Storage` are available on `Deps`; B5 must answer 503 when `h.Storage == nil`.
- `chimw.Logger` logs full URLs including query strings; avoid putting secrets in query strings.
- The auth limiter is a process-global, keyed on `RemoteAddr` after `chimw.RealIP` (so it trusts `X-Forwarded-For`; fine behind the Tailscale ingress, spoofable if exposed directly).
- Docker verified manually: image 15.4 MB, `docker run --read-only --tmpfs /tmp --user 65532` with a Postgres on a network migrates, bootstraps the admin from env, serves `/health` (`{"ok":true,"version":"1.2.3"}` with `--build-arg VERSION=1.2.3`), `docker exec <ctr> /app/server healthcheck` exits 0 (and 1 for a closed port), `docker stop` shuts down gracefully.

## Gate output

```
$ cd api && go build ./... && go vet ./... && go test -count=1 ./...
?   	github.com/hayward-solutions/knowledge/api/cmd/server	[no test files]
ok  	github.com/hayward-solutions/knowledge/api/internal/auth	0.457s
ok  	github.com/hayward-solutions/knowledge/api/internal/bootstrap	1.780s
ok  	github.com/hayward-solutions/knowledge/api/internal/config	0.166s
ok  	github.com/hayward-solutions/knowledge/api/internal/content	0.185s
?   	github.com/hayward-solutions/knowledge/api/internal/deps	[no test files]
ok  	github.com/hayward-solutions/knowledge/api/internal/domain	0.165s
ok  	github.com/hayward-solutions/knowledge/api/internal/events	0.165s
ok  	github.com/hayward-solutions/knowledge/api/internal/http	0.122s
ok  	github.com/hayward-solutions/knowledge/api/internal/http/handlers	3.881s
ok  	github.com/hayward-solutions/knowledge/api/internal/http/middleware	2.121s
ok  	github.com/hayward-solutions/knowledge/api/internal/storage	0.230s
ok  	github.com/hayward-solutions/knowledge/api/internal/store	2.130s
?   	github.com/hayward-solutions/knowledge/api/internal/testutil	[no test files]
GATE_OK

$ docker build -t knowledge-api:dev .      -> success
```
Integration tests ran against a real `postgres:16-alpine` testcontainer (no skips). `go test -race` on events, http, storage also passes. `gofmt -l .` is clean and `go mod tidy` is a no-op.

## Remediation: no space existence leak (coordinator request)

`resolveSpace` in `internal/http/middleware/space.go` now answers 404 `{"error":"not found"}` when the space exists but the caller has no access (private space, non-member, not a system admin). A missing key, page or resolver target was already 404, so the two cases are now indistinguishable. This replaces deviation 4's neighbour behaviour: previously this case was 403 `forbidden`.

Unchanged: 403 `forbidden` when the caller has access but lacks the permission (e.g. a viewer editing, a non-member of an internal space editing); 403 `space is read-only` for non-admin writes to archived spaces; soft-deleted spaces stay 404 for non-system-admins; system admins always resolve as `admin`.

Tests updated (expected status only, for private non-member access): `middleware/middleware_test.go` (4 cases renamed from "forbidden" to "not found"/"hidden"), `handlers/attachments_test.go` (outsider upload/GET/DELETE on a private page), `comments_test.go` (outsider post/list/patch/delete/resolve; test renamed `TestCommentNonMemberOnPrivateSpaceHidden`), `labels_test.go` (non-member GET/PUT private page labels, private space labels and label pages), `pages_test.go` (outsider create, get, star; the create loop is now a map of expected codes: viewer 403, outsider 404), `spaces_test.go` (outsider GET on private space, already 404 by the time I edited), `versions_test.go` (outsider list versions). Assertions about viewers, editors and internal-space users were left at 403. `search_test.go` had no such assertion.

Gate after the change: `gofmt -l .` clean, `go build ./... && go vet ./... && go test -count=1 ./...` all packages ok (including other agents' packages as of the final run).

## Content package additions (coordinator request)

- `content/sanitize.go`: `data-align` is admitted on `p` and `h1`-`h6` when its value is exactly `left|center|right|justify`. Any other value is dropped, `style` is still stripped, and the attribute is rejected on every other element. Test: `TestSanitizeDataAlign`.
- `content/text.go`: ProseMirror node type `pageLink` is an inline leaf whose text is `attrs.title` only (no fallback to `label`/`text`/`alt`; empty when absent), so page-link titles are indexed and appear inline in the surrounding line. Test: `TestExtractTextPageLink`.
- Gate: gofmt clean, `go build ./... && go vet ./... && go test -count=1 ./...` all ok.
