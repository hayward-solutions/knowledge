# D1 integration: notes

Result: clean `docker compose down -v` then `up -d --build --wait`, then `npx playwright test`: 60 passed, 0 flaky, 0 failed (53 s, first attempt, space key ENG). Stack was brought down with `docker compose down -v` afterwards and the temporary `.env` removed.

## Environment caveats for this run

- Another project's containers (waypoint) own host ports 8080, 5432, 9000 and 9001 on this machine, so the stack could not bind the default ports. I ran the real `docker-compose.yml` plus a scratch override that drops the db/rustfs host ports and publishes the api on 18080 (`NEXT_PUBLIC_API_BASE_URL=http://localhost:18080`, `E2E_API_URL=http://localhost:18080`). Nothing else differed. The default-port run (web 3000 / api 8080) is what CI does; it was not run locally.
- Docker Hub no longer serves `minio/minio` or `minio/mc` (pull denied), so the original compose could not start at all. See compose changes below.
- I ran `git -C .. status` once by mistake while checking the Playwright install (read-only, no other git commands).

## Bugs found by running the stack, and fixes

1. `docker compose up` failed on a clean checkout: MinIO images are gone from Docker Hub/Quay. Replaced with RustFS (`rustfs/rustfs:1.0.4`, same S3 API as the k3s cluster) and a one-shot `amazon/aws-cli:2.39.2` bucket creator. Service names are now `rustfs` and `rustfs-createbucket`; env var names (`MINIO_ROOT_USER`, ...) are unchanged and mapped to `RUSTFS_ACCESS_KEY`/`RUSTFS_SECRET_KEY`.
2. The API exited at boot with the shipped env: `.env.example` had `ADMIN_PASSWORD=change-me-on-first-boot` (and compose defaulted to `change-me`) which fail the password policy enforced by `HashPassword` in the bootstrap. Both now use `Change-Me-On-First-Boot-1`. The brief's "admin@example.com / change-me" cannot work against the policy; the e2e suite reads the credentials from `.env` (falls back to `.env.example`, then env overrides `E2E_ADMIN_EMAIL`/`E2E_ADMIN_PASSWORD`).
3. TopBar Create could never open the dialog: `AppShell` (and therefore `TopBar`) was rendered outside the `SpaceProvider` in `app/spaces/[key]/layout.tsx`. The provider now wraps the whole shell (and accepts `null` while loading).
4. `LabelPicker` committed a stale suggestion on Enter: typing a new label then Enter picked the highlighted result of the previous (debounced) query, adding a different label. Suggestions are now also filtered by the typed prefix client side.
5. Space settings tabs were missing on Templates and Import/export (those pages did not use `SettingsShell`). Both pages now do, with `adminOnly={false}`, and read role from `useSpace()`.
6. Starter templates only seeded when a system admin first called `GET /templates`, so a non-admin saw no templates until then. The API now seeds them at startup after the admin bootstrap (`handlers.SeedStarterTemplates`, attributed to the first active system admin). The lazy seed in `List` stays.
7. `base_version` > int32 (or negative) on draft/publish and a `{version}` path above int32 produced a 500; now 400 (review P3-15).

## Listed issues

1. Reserved slugs: `store.SlugFromTitle` appends `-page` for `edit`, `history`, `attachments` (test cases added). The C2 `slugSegment` workaround is removed.
2. Bubble menu `removeChild`: root cause is `@tiptap/react`'s `BubbleMenu`, which renders a React-owned div that the plugin then moves into a tippy popper; unmounting only the menu makes React call `removeChild` on a parent that no longer contains it. New `components/editor/SafeBubbleMenu.tsx` registers the same `BubbleMenuPlugin` against a detached element and renders children through a portal, so React owns nothing in the editor DOM. `BubbleMenus.tsx` uses it and the `key` workaround on `PageComments` is removed. Proven: with the old `BubbleMenu` and no `key`, the e2e test "a page trashed elsewhere updates in place..." fails (page crashes); with the fix it passes.
3. TopBar Create opens `NewPageDialog` (via `BodyPortal`, the header's backdrop-filter traps fixed children) when inside a space where the user can edit and the space is not archived; otherwise it links to `/spaces`.
4. `PageLabels` invalidates `["page", id]` after a change.
5. Settings tabs: see bug 5.
6. `NewPageDialog` uses `TemplatePicker` (imported from the file, not the barrel, to keep the template editor out of every bundle).
7. Purge deletes watches: done inside `PurgePage` (one transaction: lock and verify trashed, delete `watches` rows for the whole subtree with a recursive CTE, delete the page) rather than as a separate handler call, so a rejected purge never drops watches and descendants (removed by FK cascade) are covered. Tests extended in `store/pages_test.go`.
8. Contract: `content/contract_test.go` embeds the `__contract__.ts` samples (plus `data-align` p and h3) and asserts `SanitizeHTML` returns them unchanged. Only difference: the sanitiser encodes `"` in text as `&#34;` (code block sample); semantically identical, the test normalises it. Neither side changed. The e2e test "the editor produces HTML the sanitiser keeps intact" also checks real editor output after publish.
9. Notification, activity, dashboard, templates, admin spaces/users/labels payloads were compared against `lib/api/types.ts` on the live API: field names match; SSE `unread_count` is `{count}` and `notification` is a full Notification, both handled by the bell; `before` cursors on notifications and activity (created_at RFC3339, strictly older) work as C4 assumed. Import sends `webkitRelativePath` and the Go side reads the raw filename, as assumed.
10. Docker: see compose changes and verification below.

## Files changed (and why)

API
- `api/internal/store/spaces.go`, `spaces_test.go`: reserved slugs.
- `api/internal/store/pages.go`, `watches.go`, `pages_test.go`: purge deletes watches.
- `api/internal/store/users.go`: `FirstSystemAdminID`.
- `api/internal/http/handlers/templates.go`, `templates_test.go`, `api/cmd/server/main.go`: startup seeding of starters.
- `api/internal/http/handlers/pages.go`, `versions.go`, `pages_test.go`: int32 bounds on versions.
- `api/internal/content/contract_test.go` (new): contract samples.

Web
- `web/components/pages/utils.ts`: workaround removed.
- `web/components/editor/SafeBubbleMenu.tsx` (new), `BubbleMenus.tsx`; `web/components/pages/PageBody.tsx` (key removed).
- `web/components/layout/TopBar.tsx`, `web/components/pages/NewPageDialog.tsx`, `web/components/spaces/SpaceContext.tsx`, `web/app/spaces/[key]/layout.tsx`.
- `web/components/labels/PageLabels.tsx`, `LabelPicker.tsx`.
- `web/app/spaces/[key]/settings/templates/page.tsx`, `import-export/page.tsx`.
- `web/package.json`, `web/package-lock.json`: `@playwright/test` 1.64.1 (exact, devDependency) and `"e2e": "playwright test"`.
- `web/playwright.config.ts`, `web/.dockerignore` (e2e and report dirs excluded from the image context), `web/e2e/**` (below).

Repo
- `docker-compose.yml`: RustFS + aws-cli bucket job with pinned tags, api waits for the bucket job (`service_completed_successfully`), web healthcheck (`wget` on `/login`), `ADMIN_PASSWORD` default.
- `.env.example`: `ADMIN_PASSWORD`.
- `.github/workflows/pull_request.yaml`: `stack` path filter and `e2e` job (copies `.env.example`, `docker compose up -d --build --wait`, `npm ci`, `playwright install --with-deps chromium`, `npm run e2e`, uploads `web/playwright-report` and `web/test-results` on failure, dumps compose logs on failure, always `down -v`). No secrets.
- `.gitignore`: `web/e2e/.state/`.
- `CLAUDE.md`: RustFS wording and the e2e command.
- `docs/DEVELOPMENT.md`, `docs/CONFIGURATION.md`: only the `ADMIN_PASSWORD` default (D2 edits those files concurrently and already documents RustFS and the e2e setup).

## e2e suite (`web/e2e`)

`playwright.config.ts`: baseURL `http://localhost:3000` (`E2E_BASE_URL`), chromium only, 1 worker, retries 1, no `webServer`. `support/global-setup.ts` waits for api and web, logs in as admin once (storage state in `e2e/.state`, gitignored), and picks the space key: `ENG`, or `ENG` + 4 random chars if it exists, so the suite can be re-run on a used stack. Identifiers get a per-worker random tag so retries never collide with data from the failed attempt. Admin API calls reuse the saved cookie (the auth limiter is 10 burst, 10/min per IP, so logins are kept to about 15 per run).

- `01-auth`: redirect to login, wrong password, sign in/out, OIDC config `{enabled:false}`, health.
- `02-space-and-pages`: create ENG, home renders, top-bar Create dialog, create page from sidebar dialog, editor (typing, `/info` panel, image upload and render, `/table`), publish with message, view, editor HTML kept by sanitiser, second publish, history (two versions, messages, diff, side by side), restore v1 (creates v3), reserved slug.
- `03-collaboration`: page comment and reply, inline comment from selected text with resolve/reopen, labels add/reload/label page, search with snippet marks, label/space filters, quick search, `/` shortcut, star and dashboard.
- `04-notifications`: admin creates a second user in the admin UI, adds them as editor in space settings, watch toggle, second user publishes in a separate context and the admin bell increments over SSE, notification list, mark all read, mention notification.
- `05-export-import`: page export md and html downloads, space zip, settings tabs, import markdown file.
- `06-trash-and-archive`: trash subtree, trash view restore, purge, the BubbleMenu regression, archive/unarchive, move dialog, drag and drop in the tree, copy, viewer permissions, private space hidden, phone width.
- `07-templates-activity-attachments`: starter templates, use from gallery, new page dialog template picker, space template create and use, activity feed, attachment upload/download/delete.
- `08-admin-and-account`: create user, reset password, system admin toggle, disable/enable, account display name and password, admin spaces/templates/labels pages, non-admin blocked.
- `09-editor-features`: every slash item inserts the right element, mention, page link, task list, code block survive publish, draft autosave and resume, presence banner, publish conflict with "Publish anyway".

## Verified manually or by script

- `docker run --read-only --user 65532:65532 --tmpfs /tmp` of the api image on the compose network: `/health` ok, `server healthcheck` exits 0, object storage connects. Compose api runs with `read_only: true`, tmpfs `/tmp`, user `nonroot`, and uploads work (e2e). Web image under `--read-only --user 1000 --tmpfs /app/.next/cache`: `/login` 200.
- `docker buildx build --platform linux/arm64` succeeded for `api` and `web` (the host is Apple Silicon so this is native; amd64 cross-build and QEMU were not exercised).
- Crawl of 22 routes in a production build with console/network capture: only issue is below.
- `go test` full suite, `gofmt -l` clean, `go vet` clean; `npm run lint`, `npm run typecheck`, `npm run build` clean.

## Not fixed / observations

- Editor page console shows a 404 for `GET http://localhost:3000/api/v1/attachments/<id>/content` when the API is on another origin (compose dev with `NEXT_PUBLIC_API_BASE_URL=http://localhost:8080`). The stored `src` is the contract's relative path and `getHTML()` creates detached `img` elements that the browser fetches against the web origin; the node view and `ContentView` display the image correctly through `apiUrl`. Harmless, and a non-issue behind a single hostname.
- Review P3s left alone: 1 MiB JSON cap on draft/publish (effective document about 500 KB), 400 vs 413 on oversized comment/space/label bodies, unbounded list endpoints, `parent_id: null` meaning "under home page" on create, cleanup `Storage.Delete` using the request context.
- Notifications and activity rows for purged pages remain (no FK), links from them 404.
- Inline comment from inside the editor (bubble "Comment" button) is not wired on the edit page (`onRequestComment` unused); inline comments are created from the published view, which is what the suite covers.
- RustFS and aws-cli are pinned to tags that exist today; bump them deliberately.

## Final verification after review remediation

Run on the real `docker-compose.yml` with default ports (8080/3000/5432/9000/9001 free), `.env` copied from `.env.example`, `docker compose down -v` then `up -d --build --wait`, then `cd web && npx playwright test`. Result: `59 passed (52.8s)`, no flaky, no failures, space key `ENG`. (The count is 59, not 60: two 08 tests were merged, see below.) `.env` removed and the stack taken down with `docker compose down -v` afterwards.

Failures the remediation exposed (all test-side; no product code changed in this pass):
- `08-admin-and-account` hit `429 too many requests` on a UI login. The new per-email login limiter (burst 5, 12 s refill) and the per-IP limiter (burst 10, 6 s refill) are exhausted by the suite's logins. Fixes, all in e2e only:
  - `e2e/08-admin-and-account.spec.ts`: the disable/enable check now uses a throwaway user created through the API instead of re-using the Linus Torvalds account (saves a failed login on that email); the "non-admin cannot open the admin console" test is merged into the account test and reuses its session, so the account makes 3 login-limited calls (wrong password, reset-password login, post-reset login) plus the password change.
  - `e2e/support/ui.ts`: new `userContext(browser, email, password)` logs a user in once and re-uses the saved storage state for later contexts.
  - `e2e/04-notifications.spec.ts`: uses `userContext`, so the second user logs in once instead of three times.
- Nothing else failed: private-space 404, trashed-page 404/409 behaviour, trash requiring editor, new modal portals, mobile drawer, NotificationsProvider SSE bell, inline comments with prefix/suffix, and the hardened `safeNext` all passed the existing specs unchanged.

Other checks
- `docker run -d --read-only --user 65532:65532 --tmpfs /tmp` on the rebuilt api image (Go 1.26 builder): `/health` ok, `server healthcheck` exits 0, object storage connects. Compose api container: user `nonroot`, `ReadonlyRootfs=true`. Web image with `--read-only --user 1000 --tmpfs /app/.next/cache`: `/login` 200.
- `docker buildx build --platform linux/amd64` succeeded for both `api` and `web` (emulated on this Apple Silicon host; images report architecture `amd64`). linux/arm64 builds were native and succeeded in the earlier pass. Temporary tags removed.
- Gates: `gofmt -l .` empty, `go build`, `go vet`, `go test -count=1 ./...` all ok; `npm run lint` clean, `npm run typecheck` exit 0, `npm run build` ok.

Still failing: nothing. Remaining caveats are unchanged from the section above (the editor-page 404 console noise when the API is on a different origin, and the P3 list).
