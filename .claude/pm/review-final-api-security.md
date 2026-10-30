# Final API security review

Scope: the whole `api/` tree as it stands now (cmd/server, config, auth, bootstrap, domain, events, http router, middleware and every handler, store, content, notify, search, storage, migrations). Read against `docs/PLAN.md`, `docs/API.md` and `.claude/pm/review-wave2a-api.md`. Paths are relative to `api/` unless stated.

Baseline: `go vet ./...` clean, `go test -count=1 ./...` all green (handlers 29.7 s, store 8.8 s, Docker up so integration tests ran). `govulncheck ./...` and `golangci-lint -E gosec` were also run (results in P2-7 and the P3 list).

Method: every finding below was confirmed by reading the code, and the ones marked "reproduced" were also run with throwaway Go tests. The tests live outside the repo in `/private/tmp/claude-501/-Users-matt-Projects-hayward-solutions-knowledge/1093f6a2-0bd0-4c38-9db7-f630dbd777e5/scratchpad/rv/` and were injected with `go test -overlay <overlayN.json>` (virtual files mapped onto `internal/.../zz_review*_test.go`), so nothing was written under the repo. Re-run any of them with, for example, `cd api && go test -overlay <that dir>/overlay7.json -run TestReviewE2ECycle -v ./internal/http/handlers/`.

Wave 2a items that are now fixed and re-checked: trash list and trashed-page reads are editor-only (`IsPageVisible`, `pageLoad`, `labelGate`, `attachmentPageVisible`, `commentPageVisible`), never-published trashed pages are creator/admin only, write endpoints on trashed pages answer `409 page_trashed`, 403/404 oracle in `resolveSpace` removed, template space scoping (`pages.go:237`), purge deletes attachment objects, PDFs no longer inline, reply depth capped at two, `base_version` bounds, `GET /pages/{id}` returns real labels/watching.

## Summary

| Rank | ID | Finding |
|---|---|---|
| P1 | P1-1 | Markdown import: a ~40 KB filename allocates ~860 MB (quadratic) and the same request can create thousands of pages; ~200 KB OOM-kills the process. Any signed-in user can reach it (anyone can create a space and become its admin). |
| P1 | P1-2 | A page-tree cycle can be created (cycle check stops at depth 1000, import builds deeper chains in one request); every unguarded recursive CTE then runs forever and starves the DB pool. |
| P1 | P1-3 | Login/change-password rate limit is bypassed by any `X-Forwarded-For` value (`chimw.RealIP` trusts the client); also unbounded limiter map growth and unbounded concurrent argon2 (32 MiB each). |
| P2 | P2-1 | OIDC login can demote the last system admin (no last-admin guard, flag overwritten on every login). |
| P2 | P2-2 | OIDC links to an existing local account by email with no `email_verified` check; a local user can set their email to a victim's before the victim's first SSO login. |
| P2 | P2-3 | Changing a password does not revoke the user's other sessions. |
| P2 | P2-4 | Space activity feed shows titles and comment excerpts of trashed and purged draft pages (and trashed published pages) to every viewer. |
| P2 | P2-5 | No `ReadTimeout`/`IdleTimeout`/`MaxHeaderBytes`: unauthenticated slow-body connections are held forever. |
| P2 | P2-6 | Import CPU/memory: Markdown conversion is super-linear (100 KB of `>` = 3 s, 1 MB > 45 s, 1.6 MB of link definitions = 21 s) with a 25 MiB per-file cap, no cancellation, and ~200 MiB buffered per request. |
| P2 | P2-7 | Known-vulnerable dependencies reachable from the server: `golang.org/x/net` v0.58.0 (html parser/renderer, http2) and `golang.org/x/crypto` v0.55.0. |

---

## P1 (must fix)

### P1-1 Markdown import path handling is quadratic in memory and unbounded in page count (reproduced)

- `internal/http/handlers/export.go:268-288` (`exportSplitPath` has no limit on segment count or length), `343-370` (`exportPlanner.folder`), `442-459` and `488-508` (planning loops), `407-409` (only the file count is capped), `internal/store/export.go:54-95` (`ImportPages` inserts every planned page in one transaction under the space advisory lock).
- Issue: each `files` part's `filename` may carry a relative path, and every distinct folder segment becomes a page. `folder()` recurses once per segment and computes `key := strings.Join(dirs, "/")` at every level, and `key` stays live across the recursive call. Memory is therefore O(n²) in the number of segments and recursion is O(n) deep. Go's multipart reader accepts part headers up to 10 MiB, so the filename is bounded only by that. The request body cap (201 MiB) does not help because the payload is tiny.
- Measured (planner alone, `planner_review_test.go`): 2 000 segments = 4 KB header = 9 MB; 10 000 = 20 KB = 209 MB; 20 000 = 40 KB = 839 MB / 1.6 s. At ~100 KB it is ~5 GB and at ~200 KB ~20 GB, so one request OOM-kills the API container. Even short of that, each segment becomes a page (20 000 pages in one transaction holding the space lock, plus 20 000 version rows).
- Reproduction: sign in as any user, `POST /api/v1/spaces {"key":"ATK","name":"x"}`, then `POST /api/v1/spaces/ATK/import` multipart with `files` part `filename="` + `d/` repeated 20 000 times + `x.md"`. `TestReviewPlannerDeepPath` (`REVIEW_N=20000`) and `TestReviewE2ECycle` show the behaviour.
- Fix:
  1. In `exportSplitPath` reject more than 8 directory segments, any segment over 100 runes, and a total name over 512 bytes (`400 invalid file path`). Also reject the whole `files` part header early: stop reading when `len(name) > 1024`.
  2. Cap total planned pages per request (for example 500) and return `400`/`413` when exceeded.
  3. Make `folder()` iterative and key folders by `(parentIndex, segment)` instead of a joined path, so work is O(segments).
  4. Set a maximum page depth for the whole tree (see P1-2) and have `ImportPages` enforce it.

### P1-2 Page-tree cycle via the depth-1000 cap, then unbounded recursive CTEs (reproduced end to end)

- `internal/store/pages.go:375-385` (cycle check `WHERE a.depth < 1000`), `470-476` (`TrashPage` CTE), `534-542` (`RestorePage` CTE), `internal/store/attachments.go:104-108` (`ListAttachmentObjectKeysUnderPage`), `internal/store/watches.go:64-68` (`ListPublishWatcherIDs`) and `114-119` (`deletePageSubtreeWatches`). `CreatePage` (also used by Copy), `MovePage` and `ImportPages` impose no depth limit.
- Issue: `MovePage` detects a cycle by walking up from the new parent for at most 1 001 nodes. If the page being moved is an ancestor of the new parent at distance 1 001 or more, the walk never reaches it and the move succeeds, creating a real cycle. All the other subtree CTEs are `UNION ALL` with no depth or visited guard, so on a cycle they never terminate (the only escape is the request context, and the attacker keeps the connection open). The default pgx pool is small, so a few of these queries exhaust it and take down the API; the pages in the cycle also drop out of the tree (no root).
- Reachability: one import request builds the deep chain (P1-1 shows a 1 100-segment filename is 2 KB and takes 0.7 s), then one move closes the loop. Without import, ~1 000 `POST /spaces/{key}/pages {parent_id}` calls do the same.
- Reproduced:
  - Store level: 1 003 chained pages, `MovePage(root, parent=deepest)` returns `nil`, the cycle edge exists, `TrashPage` on any member is still running when a 5 s context expires (`TestReviewCycleDeepChain`).
  - HTTP level: `POST /spaces/ATK/import` with a 1 100-segment filename returns `201` with 1 101 pages; `POST /pages/{first}/move {"parent_id": "{deepest}"}` returns `200`; `POST /pages/{mid}/trash` never responds (client times out at 8 s) (`TestReviewE2ECycle`).
- Fix:
  1. Replace the cycle check with a walk that cannot be defeated by depth and always terminates: `WITH RECURSIVE anc(id) AS (SELECT $1::uuid UNION SELECT p.parent_id FROM pages p JOIN anc a ON p.id = a.id WHERE p.parent_id IS NOT NULL) SELECT EXISTS (SELECT 1 FROM anc WHERE id = $2)`. `UNION` (not `UNION ALL`) on a single id column dedupes, so even an existing cycle terminates.
  2. Change every subtree/ancestor CTE listed above to the same `UNION`-on-id shape (carry no depth column; for `listAncestors` keep its depth bound or compute order after the fact).
  3. Enforce a maximum tree depth (for example 64) in `CreatePage`, `MovePage` (parent depth + height of the moved subtree) and `ImportPages`, returning `ErrInvalidParent`/400.
  4. Defence in depth: set `statement_timeout` (for example 30 s) on the pool (`pgxpool.Config.ConnConfig.RuntimeParams`), and add a `BEFORE UPDATE OF parent_id` trigger or a one-off health query that fails when a cycle exists.

### P1-3 Auth rate limiting is bypassable with a request header; limiter and argon2 are unbounded (reproduced)

- `internal/http/router.go:23` (`chimw.RealIP`), `internal/http/middleware/ratelimit.go:41-69` (`allow`, `clientIP`), `internal/http/handlers/auth.go:22,26` (routes), `internal/auth/password.go:19-21` (argon2 32 MiB, 3 passes).
- Issue: `chimw.RealIP` overwrites `r.RemoteAddr` from `True-Client-IP`, `X-Real-IP` or the first `X-Forwarded-For` entry with no trusted-proxy check, and the limiter keys on that value. Any client can send a fresh header value per request. Behind a typical proxy that appends to `X-Forwarded-For`, the left-most (attacker-supplied) entry still wins. Consequences:
  1. Login and change-password throttling is void: unlimited online password guessing against any account, including the bootstrap admin.
  2. Memory DoS on the limiter: every distinct key creates a `limiterEntry` that is only swept after 30 minutes idle, and the key can be up to the full header (default `MaxHeaderBytes` is 1 MiB, so one request can pin ~1 MiB of key).
  3. Unauthenticated concurrent argon2: each `POST /auth/login` runs `VerifyPassword` or `DummyVerify` (32 MiB). With the limiter void, N parallel requests allocate N x 32 MiB with no semaphore.
  4. `chimw.Logger` logs the spoofed address, so log attribution is forgeable.
- Reproduced (`TestReviewRateLimitBypass`): 40 wrong-password logins with a fixed `X-Forwarded-For` give `{401: 10, 429: 30}`; 40 with a different `X-Forwarded-For` each give `{401: 40}`. The existing test helper `ipTransport` (`handlers/helpers_test.go:24-30`) relies on exactly this to dodge the limiter.
- Fix:
  1. Remove `chimw.RealIP`. Use `RemoteAddr` by default. If the API sits behind a known proxy, add a `TRUSTED_PROXIES` CIDR list and take the right-most `X-Forwarded-For` entry that is not in the list.
  2. Cap the limiter map (for example 100 000 entries, evict oldest) and reject keys longer than 64 bytes.
  3. Add a package-level semaphore (4 to 8 slots) around `VerifyPassword`, `DummyVerify` and `HashPassword` so concurrent logins queue instead of allocating.
  4. Add a second limiter keyed on the normalised email (for example 10 failures per 15 minutes) so a botnet cannot spread guesses across IPs.
  5. Update `helpers_test.go` to use per-client `RemoteAddr` or a test-only trusted-proxy config.

---

## P2 (should fix)

### P2-1 OIDC login can demote the last system admin (reproduced)

- `internal/store/users.go:243-250` (existing identity: `is_system_admin = COALESCE($4, is_system_admin)`), `258-261` (email-link branch, same), `internal/http/handlers/oidc.go:152-156` and `158-165`.
- Issue: when `OIDC_ADMIN_GROUP` is set, every SSO login overwrites `is_system_admin` with group membership and bypasses the `ErrLastAdmin` guard that `AdminUpdateUser` enforces. A local admin (the bootstrap admin, or anyone promoted in the admin UI) who signs in via SSO without the group is silently demoted, and if they were the only admin the instance has none. The same happens when the IdP omits the groups claim for a login (the claim is simply absent, `HasGroup` returns false, and `false` is written). The disabled-user check at `oidc.go:163` runs after the update has been applied.
- Reproduced (`TestReviewOIDCDemotion`): one local admin, `UpsertOIDCUser(..., same email, &false)` returns the same user with `is_system_admin=false`; `CountActiveSystemAdmins` is 0.
- Fix: never write `false` through the OIDC path unless the groups claim was present and the guard in `AdminUpdateUser` (lock active admins, refuse when count would be 0) passes. Prefer "promote only" (`is_system_admin OR $4`) and add a column recording admin source if demotion by group removal is wanted. Pass `nil` (no change) when the groups claim is missing. Skip all updates for disabled users.

### P2-2 SSO account linking by email without `email_verified`, plus unauthenticated-email pre-hijack (reproduced)

- `internal/store/users.go:254-263`, `internal/http/handlers/oidc.go:127-146` and `158`, `internal/http/handlers/auth.go:99-135` (`PATCH /auth/me` changes email with no re-authentication).
- Issue: an SSO login with an unknown `(issuer, subject)` is bound to any existing local user with the same email, including admins, and `email_verified` is never read. Two exploitable variants: (a) an IdP that lets users set an unverified email lets them take over any local account; (b) pre-hijack: a local user changes their own email to the victim's address, then the victim's first SSO login is attached to the attacker's account, which still has the attacker's password hash. The victim then works in an account the attacker can log into.
- Reproduced (`TestReviewPrehijack`): after `UpdateUserProfile(attacker, victim@corp.test)` and `UpsertOIDCUser(..., victim@corp.test)`, the OIDC user id equals the attacker's id and the password hash is still present.
- Fix: require `email_verified == true` (bool or `"true"`) in the ID token or userinfo, else fail the login. Make linking to an existing local account opt-in (`OIDC_LINK_EXISTING_USERS=true`), and when it happens clear `password_hash` or require the local user's current password. Require `current_password` for `PATCH /auth/me` email changes. Optionally compare the userinfo `sub` with the ID token `sub` before using userinfo claims (`oidc.go:129-141`).

### P2-3 Password change does not revoke other sessions (reproduced)

- `internal/http/handlers/auth.go:170-174`, `internal/store/users.go:222-232`, `internal/store/sessions.go:61-64`.
- Issue: `ChangePassword` updates the hash and returns 204. Existing sessions (30 days, no idle timeout) stay valid, so a stolen cookie survives the user's response to a compromise. The admin reset path does call `DeleteUserSessions`; the self-service path does not. Disabling a user also leaves rows in place (blocked per request, revived on re-enable).
- Reproduced (`TestReviewChangePasswordKeepsOtherSessions`): two sessions, change password on one, the other still gets `200` on `/auth/me`.
- Fix: after `SetUserPassword`, delete all the user's sessions except the current token (`DELETE FROM sessions WHERE user_id = $1 AND id <> $2`) and rotate the current one. Also call `DeleteUserSessions` when an admin disables an account.

### P2-4 Activity feed leaks draft, trashed and purged page content (reproduced)

- `internal/store/activity.go:47-50` (`p.id IS NULL OR p.status <> 'draft' OR p.created_by = $3 OR $4`), `migrations/0070_notify.sql:27-35` (`activity.page_id` has no foreign key), `internal/notify/notify.go:223,266,279,294` (activity is recorded for draft pages).
- Issue: while a page is a draft its activity rows are hidden. The moment it is trashed its status is `trashed`, so the `<> 'draft'` test passes and the rows (`title`, `page_title`, `excerpt` of every comment, `message`) are shown to every viewer of the space. After purge the join returns `p.id IS NULL` and the rows are public forever. Trashed published pages are likewise listed to viewers although `GET /pages/{id}` answers 404 for them. A natural workflow ("write a draft, comment, delete it") exposes the title and comment text.
- Reproduced (`TestReviewActivityLeakTrashedDraft`): viewer sees 0 rows before the trash and 1 row after, including `"excerpt":"layoffs are coming"`.
- Fix:
  1. Skip `recordActivity` for `comment.created`, `page.moved`, `page.trashed` and `page.restored` when the page status is draft (or a never-published trashed page).
  2. Rewrite the filter to use the same rules as `IsPageVisible`: draft, or trashed with `version = 0 AND published_at IS NULL`, only for creator or space admin; trashed otherwise only for editors; `p.id IS NULL` only when `a.page_id IS NULL`.
  3. Delete the activity (and notification) rows of purged pages in `PurgePage`, or add `REFERENCES pages(id) ON DELETE CASCADE` on `activity.page_id`.

### P2-5 No server read/idle timeouts: unauthenticated slow connections are held forever

- `cmd/server/main.go:114-127` (only `ReadHeaderTimeout: 5 * time.Second`), `internal/http/handlers/util.go:23-25` (`readJSON`).
- Issue: after headers arrive, the body read has no deadline, idle keep-alives never expire and `MaxHeaderBytes` is the 1 MiB default. A client that declares `Content-Length` and trickles bytes to the public `POST /auth/login` keeps a goroutine and a file descriptor indefinitely (the limiter runs first but is bypassable, P1-3); a few thousand such connections exhaust the process. Not run; derived from the server configuration. Authenticated upload paths additionally pin a temp file per stalled upload (wave 2a P3-23, still open).
- Fix: set `ReadTimeout: 60 * time.Second`, `IdleTimeout: 120 * time.Second`, `MaxHeaderBytes: 64 << 10` on the server. For upload and import handlers extend per request with `http.NewResponseController(w).SetReadDeadline(time.Now().Add(10 * time.Minute))`; for the SSE route call `SetWriteDeadline(time.Time{})`. Do not set `WriteTimeout` globally (it would kill SSE and large downloads).

### P2-6 Import: super-linear Markdown conversion, 25 MiB per file, no cancellation, large per-request memory (reproduced)

- `internal/http/handlers/export.go:379-383` (per-file cap = `ATTACHMENT_MAX_BYTES`, total 8x), `411` (`io.ReadAll` per file), `323-326` (`convert`), `internal/content/import.go:70-85` (`FromMarkdown`).
- Issue: conversion cost explodes on adversarial input and nothing checks `r.Context()`, so a disconnected client does not stop the work. Measured on this machine with `FromMarkdown`: 100 000 `>` characters = 3.0 s (then fails with `html: open stack of elements exceeds 512 nodes`, which the handler reports as a 500); 1 000 000 `>` did not finish in 45 s; 200 000 `[` + `]` = 0.5 s; 1.6 MB of `[a]: /x` link definitions = 20.7 s. With 25 MiB per file and 200 files, a single editor can pin every core for hours, and all uploaded bytes (up to ~201 MiB) are held in memory before conversion.
- Fix: introduce a separate Markdown import cap (for example 1 MiB per file, 10 MiB per request); pre-scan and reject runs of more than ~50 `>` or list-indent levels and more than ~1 000 link-reference definitions; run conversion in a worker pool of 2 with a per-file deadline and a `select` on `r.Context().Done()`; map conversion errors to `400 unsupported markdown structure` instead of 500; consider streaming part-by-part instead of buffering all uploads.

### P2-7 Vulnerable dependencies reachable from the server (govulncheck)

- `go.mod:19-20` (`golang.org/x/crypto v0.55.0`, `golang.org/x/net v0.58.0`), `Dockerfile:1` (floating `golang:1.25-alpine`).
- `govulncheck ./...` reports 37 findings reachable from this code (go1.26.0 local toolchain plus modules). The module ones that matter for the running server:
  - `GO-2026-6866` x/net/html "check all ancestor namespace transitions when rendering raw text" (fixed v0.62.0), reached via `content/import.go:164` (`html.Render`).
  - `GO-2026-6860` x/net/html quadratic text-node coalescing (fixed v0.62.0), reached via `content/markdown.go:719` (`html.Parse`, run on stored HTML during export).
  - `GO-2026-6617`, `6612`, `6611`, `6610`, `6603` x/net HTTP/2 server issues (fixed v0.60.0), reached through the `net/http` server in `router.go`.
  - x/crypto `GO-2026-6355`/`6354` (ssh, fixed v0.56.0) are only reached from the test container helper.
  - A long list of stdlib fixes (net/http, net/textproto/mime/multipart header memory, crypto/tls, crypto/x509, net/url, encoding/xml) that apply to whatever toolchain builds the image.
- Fix: `go get golang.org/x/net@v0.62.0 golang.org/x/crypto@v0.56.0 && go mod tidy`; pin the build image to a current patched `golang:1.25.x-alpine` (and refresh periodically); add `govulncheck ./...` to CI.

---

## P3 (nits and hardening)

Carried over from the wave 2a list and still open:
1. `handlers/pages.go:597-605` (Copy) copies `ContentHTML` without `content.SanitizeHTML`. Defence in depth if the policy ever tightens.
2. `store/pages.go:178-191` (`pageCheckParent`), used by create, move, copy, import: accepts another user's draft page as a parent (existence oracle for a known id, and a published child can hang under a hidden draft). `store/spaces.go:241` accepts a draft as `home_page_id` although `docs/API.md` says "published page"; the home page can also be moved under another page (`MovePage` has no home check), after which trashing the new parent trashes the home page.
3. `store/pages.go:36-37`: `has_children` counts other users' draft children.
4. `handlers/attachments.go:216`, `:365`, `handlers/pages.go:569`: object cleanup after a failed insert, a delete or a purge uses `r.Context()`, so a client disconnect can orphan objects. Use `context.WithoutCancel`.
5. `handlers/attachments.go:338`: `Cache-Control: private, max-age=3600` on attachment responses keeps content available in the browser after access is revoked or the file is deleted.
6. Unbounded list endpoints: trash, versions, attachments, members, comments, space tree (every page of the space), labels, admin users. Notifications and activity are cursor-paged. `ExportSpace` (`handlers/export.go:193-240`, `store/export.go:25-44`) loads every published page (JSON plus HTML) into memory for any viewer.
7. `handlers/pages.go` PATCH title writes no version row (history shows the old title).

New in this review:
8. Conversion amplification on export: nested lists in `content_json` (depth about 2 500, 320 KB) render to ~6 MB of Markdown in ~1.5 s (`ToMarkdown`, quadratic indentation in `mdPrefixItem`). A malicious editor makes the space-export zip arbitrarily large for everyone. Cap output per page and total zip size.
9. `content/markdown.go:520-528,619` (`mdDest`): `ToMarkdown` copies link hrefs from unvalidated `content_json` verbatim, so a stored `javascript:` href is exported as `[click](<javascript:...>)` (reproduced). Filter schemes to http, https, mailto and relative in `mdDest`.
10. `content_json` is only checked for "starts with `{`" and is never reconciled with `content_html`. Text present only in the JSON is searchable (search snippets and `content_text` prefer JSON) and appears in Markdown export but not in the rendered page. Derive one from the other or compare extracted text.
11. Sanitiser policy choices worth a conscious yes: remote `img src` (http/https/`//host`) allowed (tracking pixels, viewer IP disclosure; the standalone export CSP also allows `img-src *`); `target="_blank"` allowed without forcing `rel="noopener noreferrer"` (modern browsers imply it); heading `id`s such as `location` are allowed (the web app rewrites ids in `ContentView`, other consumers should too).
12. `handlers/spaces.go:117-119`: `POST /spaces` answers `409 space key already in use` for keys of private and soft-deleted spaces, an existence oracle for private space keys (2 to 10 alphanumerics). `PUT /spaces/{key}/members/{userID}` answers 404/409 for unknown/disabled user ids (user-id probing by any space admin).
13. `handlers/export.go:469-476`: import `parent_id` returns `404 "parent page not found"` for a page in another space and `404 "not found"` for a missing page, leaking existence of foreign page ids. Use one message.
14. `internal/http/router.go:24`: `chimw.Logger` logs the full request URI, so `GET /api/v1/auth/oidc/callback?code=...&state=...` and `GET /search?q=...` land in logs. Log the path only (also removes log forging through crafted `X-Forwarded-For` once `RealIP` is gone).
15. Defaults: `config.go:81` `COOKIE_SECURE` defaults to false (derive from `PUBLIC_BASE_URL` starting with `https://`); `docker-compose.yml:64` and `.env.example` ship `ADMIN_PASSWORD=Change-Me-On-First-Boot-1`, a policy-valid, public default that `bootstrap/admin.go:34` accepts. Refuse to bootstrap with that exact value, or log a loud warning. Compose also publishes Postgres on 5432 and the API on 8080 on all interfaces with default credentials (dev only).
16. `handlers/oidc.go:213-224` (`safeNext`) passes a value such as `/%09/evil.test` (tab) and is only safe because `http.SetCookie` strips the tab and the callback re-validates; parse with `url.Parse` and require empty scheme and host. `oidc.go:136` copies groups from userinfo without checking `sub`.
17. `NUL` bytes (`\u0000`) in a title, `content_html`, `content_json` string, comment or space name return `500` (`22021`/`22P05` logged as internal errors) instead of `400` (reproduced via draft save and comment create). Reject `\x00` in `readJSON` strings or map those SQLSTATEs to 400.
18. No quotas anywhere: spaces per user, attachments per page/space/user, versions per page, labels (rows are never garbage-collected), notifications, SSE connections per user (`handlers/notifications.go:143`), one goroutine per event per bus handler (`events/bus.go:27-38`). An authenticated user can grow storage and goroutines without bound; add per-user write rate limiting and a cap on SSE subscribers per user.
19. A space admin can add any active user (`PUT /spaces/{key}/members/{userID}`) and re-trigger `space.added` by remove/re-add; historical notifications and activity keep page titles after access is removed. Consider consent or at least rate limiting.
20. Deleting a deleted comment that still has replies keeps its `inline_anchor.quoted_text` in `store/comments.go:109-125` (only body is blanked).
21. Response bodies are not sent with `X-Content-Type-Options: nosniff` globally (only on downloads); add it as router middleware. `GET /users` returns every active user's email to any signed-in user (matches the plan; flagging it as a conscious decision).
22. `gosec` noise worth fixing mechanically: unchecked `Close`/`Remove` in `storage/storage.go:141-142`; `internalErr` logs `r.URL.Path` (decoded) so a `%0a` in a `{key}` can forge log lines when an internal error happens; use `EscapedPath`.

---

## Verified correct (no action)

Authorization and visibility
- Every route registered under `/api/v1` goes through `r.With(...)`, apart from the three public OIDC GETs and `/health`. A grep for `r.With(` lines without `RequireAuth`, `SpaceBy*`, `byComment`, `view/edit/purge` or `OptionalAuth` finds only `/auth/login` (public by design, limiter attached). Permission per route matches `docs/API.md` (view, comment, edit, manage_attachments/labels/templates, manage_members/space, purge_trash, delete_space, sysadmin on `/admin/*`).
- `resolveSpace` (`middleware/space.go:84-124`) returns the same `404 not found` for unknown ids, soft-deleted spaces (non-sysadmin) and private spaces the caller is not in; 403 only appears after the caller has a role. Entity routes (`SpaceByPage`, comment/attachment resolvers) derive the space from the entity, so no id can ride on another space's permission. Archived/deleted read-only handled in `RequireSpacePerm`.
- Draft and trashed rules (`store.IsPageVisible`) are applied by `pageLoad` (get, children, star, draft, versions, restore, export, publish, move, copy, purge), comments, labels (`labelGate`), attachments, watches, dashboard (membership and draft filters in SQL), search (published only), quick search (own drafts only), tree, trash list, breadcrumbs. Activity is the exception (P2-4).
- Cross-space references: `parent_id` (create, move, copy, import), `after_id` (sibling list), comment `parent_id` (`parent.PageID != pageID`), `template_id` (`tpl.SpaceID` check, global templates allowed), `home_page_id` (same space, non-trashed), watch targets (access check by page or space), attachment space from page row, notification recipients (`FilterNotifiable`: active, space access, draft rules, mentions filtered the same way).
- Ownership: comment edit author-only, comment delete author or space admin, drafts per user, template mutation by role or sysadmin. Last-admin guards: space members (`lockSpaceAdmins` FOR UPDATE), system admins (`AdminUpdateUser`, except the OIDC path in P2-1). Admin-only routes enforce `RequireSystemAdmin`.

Injection
- All SQL is parameterised. The only dynamic fragments are constants (`dashboardBase` pieces, `AccessClause`, column lists in `AdminUpdateUser`, `HeadlineOptions` built from random hex at start). LIKE/ILIKE inputs are escaped with `EscapeLike`/`escapeLike`. Search uses `websearch_to_tsquery` on a bound parameter, capped at 200 characters. Snippets are HTML-escaped before the per-process random markers are replaced by `<mark>`.
- Content-Disposition: tested with quotes, CRLF, bidi controls, `../`, `%0d%0a`, semicolons and non-ASCII (`TestReviewDisposition`); output is always a quoted ASCII fallback plus RFC 5987 `filename*`, no header injection. Export names come from slugs.
- Paths: import filenames never touch the filesystem (absolute paths, `..` and drive letters are rejected; only used to build page titles). Export zip entries are slug-derived (`[letters digits - _]`), deduplicated, no separators. S3 keys are `spaces/{uuid}/pages/{uuid}/{uuid}/{sanitised filename}` with path parts, control and format characters removed.

XSS and content
- The bluemonday policy (`content/sanitize.go`) allow-lists elements and attributes, anchors all `data-*` values with `^...$` regexes (Go `$` matches only end of text), allows only http, https, mailto and relative URLs with `RequireParseableURLs`, and drops svg, math, iframe/srcdoc, meta, base, form, style, event handlers, `details open`. A 24-vector battery (`TestReviewSanitizerBattery`, covering encoded and tab-split `javascript:`, svg/math `xlink:href`, nested mXSS shapes, `srcdoc`, meta refresh, attribute junk on mention/status/panel spans, table `background`) produced no active content.
- Sanitiser coverage on write: draft save, publish, version restore, page create from template, template create/update/seed, comment create/update, Markdown import (`FromMarkdown` sanitises last), standalone HTML export (re-sanitises, escapes the title). Copy is the only path that skips it (P3-1). The web viewer renders the sanitised `content_html` only (`web/components/editor/ContentView.tsx`); `content_json` is loaded into the editor, so the server should treat it as untrusted (P3-9, P3-10).
- Standalone HTML export is served with `Content-Disposition: attachment`, `nosniff` and `sandbox; default-src 'none'; style-src 'unsafe-inline'; img-src * data:`.

Sessions and auth
- Tokens are 32 random bytes, stored as SHA-256, cookie `HttpOnly`, `SameSite=Lax`, `Secure` when configured, new token per login (no fixation), logout deletes the row and clears the cookie, 30-day expiry with hourly cleanup. Every request re-loads the user and rejects disabled accounts; SSE re-validates the session and user at every 30 s ping.
- Login equalises timing (`DummyVerify` for unknown user, missing hash and invalid email), argon2id parameters validated on verify, constant-time compare, policy enforced on every set path (login limiter coverage gap is P1-3).
- OIDC: random state and nonce in short-lived cookies, state compared before use, nonce compared to the verified ID token (issuer, audience, signature, expiry by go-oidc), `next` re-validated at callback and prefixed with `PUBLIC_BASE_URL`, failures redirect with fixed reason strings. Admin password reset revokes sessions.

Resource handling
- Every JSON body goes through `readJSON` (1 MiB `MaxBytesReader`; `pageDecode` and `templateDecode` map overflow to 413). Attachment upload streams multipart with `MaxBytesReader(limit + 1 MiB)`, spools once to a 0600 temp file removed by `defer`, then `Put`; the object is deleted if the row insert fails. Import is capped at 200 files and 8x per-file limit overall. Search `page` capped at 10 000, limits clamped, JSON nesting is limited to 10 000 by `encoding/json`, `x/net/html` limits open elements to 512, colspan is capped at 64 in Markdown export.
- SSE: `unsubscribe` deferred, `Hub.Publish` non-blocking and safe against close (publish and close both under the hub lock), `r.Context().Done()` ends the loop. Presence entries are keyed only after authorisation and swept every 10 s with a 30 s TTL. Bus handler panics are recovered.
- Error envelope is generic: `internalErr` logs method and path, never the query or body, and returns a fixed message; no driver error text is returned to clients.
- Tree operations (create, move, trash, restore, import) run in one transaction under a per-space advisory lock, events are published after commit.
- Attachment responses carry `nosniff`, `Content-Security-Policy: default-src 'none'; sandbox`, `Content-Disposition: attachment` for everything except png, jpeg, gif, webp, avif on `/content`; the stored content type is always parsed through `mime.ParseMediaType` and never taken verbatim from the upload part.
- CORS is an exact-origin allow-list with `Vary: Origin`; no wildcard; credentials only for listed origins.

## Overall verdict

Not ready to expose to untrusted users yet. The authorization model, SQL, sanitiser, session handling and storage paths are solid and consistent, and the wave 2a findings were fixed properly. The three P1s are all cheap for any signed-in user (or, for P1-3, an anonymous client) to trigger: one import request can OOM the process, one import plus one move wedges the database with an unterminating query and orphans a subtree, and the login throttle is a header away from useless. Fix P1-1 to P1-3 and P2-1 to P2-5 before the first public deployment; P2-6 and P2-7 should follow immediately after, and the P3 list can be folded into the next hardening pass.
