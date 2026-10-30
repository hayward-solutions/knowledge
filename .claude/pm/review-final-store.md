# Final review: database and data integrity (api/migrations, internal/store, internal/notify, internal/search)

Scope read in full: `api/migrations/*.sql` (9 files), `api/internal/store/*.go` (non-test), `api/internal/notify/notify.go`, `api/internal/search/query.go`, plus the handler call sites needed to judge invariants (`handlers/pages.go`, `versions.go`, `comments.go`, `oidc.go`, `notifications.go`).

Baseline: `go test -count=1 ./internal/store/...` passes (postgres:16-alpine, 8.4 s). `go test -count=1 -run Migrat ./internal/store/` passes (`TestMigrationsDownAndUp`, `TestNotifyMigrationCascades`).

Extra verification performed (all outside the repo, scratch dir `/private/tmp/claude-501/-Users-matt-Projects-hayward-solutions-knowledge/1093f6a2-0bd0-4c38-9db7-f630dbd777e5/scratchpad`):

- Whole `./internal/...` suite run against `postgres:15.19` via `KNOWLEDGE_TEST_POSTGRES_DSN`: green. No 16-only syntax or functions found (grep for `any_value`, `IS JSON`, `JSON_*` constructors, `pg_input_*`, `MERGE`, `uuidv7`: none).
- Migrations Up then Down then Up executed with psql on PG15 against a populated database (50 000 pages, 600 000 page_views, 25 000 notifications): clean.
- EXPLAIN ANALYZE of the dashboard, search, quick search, label, notification, tree and activity queries on that dataset.
- Concurrency probes in a copy of `api/` (not the repo): concurrent publish, concurrent create, concurrent cross-moves, last space admin race, last system admin race, home page trash, restore edge cases.
- Proposed migrations 0090/0091/0092 below were added to the copy and the whole suite re-run on PG15: green (including the full Down-to-0 and Up cycle).

## Verdict

No P1. The schema, transaction structure and most queries are sound; the version-conflict, position-renumbering, cycle-guard and last-space-admin logic held under concurrent probing. There are five P2 items that should be fixed before calling the store done (home page can be trashed through an ancestor, mention notifications re-fire on every publish, dashboard cost grows with total page count, a missing FK index that makes page purge scan page_views, and OIDC email auto-linking). Everything else is P3 hardening. Ship after the P2s; the P3s can be batched.

## P2

### P2-1 Home page can be trashed via an ancestor (invariant: home page is never trashed)

- `api/internal/store/pages.go:446` `TrashPage` checks `home_page_id == id` only for the page being trashed (lines ~462-468). The descendant CTE at line 470 then trashes the whole subtree, which may contain the home page.
- `api/internal/store/pages.go:348` `MovePage` has no guard against moving the home page under another page, so the situation is reachable by any editor (`POST /pages/{id}/move` with `parent_id`).
- Reproduced (PG15): move home under page P, `TrashPage(P)` returns nil, home page status becomes `trashed`, and `spaces.home_page_id` still points at it. Space home then 404s/blanks, and a later purge of P cascades the home page away (`ON DELETE SET NULL`).
- Fix (code, no migration):
  1. In `TrashPage`, after the `home` lookup, replace the equality check with a subtree check:
     ```sql
     WITH RECURSIVE sub AS (
         SELECT id FROM pages WHERE id = $1
         UNION ALL
         SELECT c.id FROM pages c JOIN sub ON c.parent_id = sub.id WHERE c.status <> 'trashed'
     )
     SELECT EXISTS (SELECT 1 FROM sub WHERE id = $2)
     ```
     with `$2 = home_page_id`; return `ErrHomePage` when true.
  2. In `MovePage`, after loading `cur`, load `spaces.home_page_id` in the same tx and return `ErrHomePage` when `cur.ID == home` and `p.ParentID != nil`.
  3. Add store tests for both.

### P2-2 `page.mentioned` notifications re-sent on every publish and every version restore

- `api/internal/http/handlers/pages.go:189` passes `content.MentionedUserIDs(page.ContentJSON)` (all mentions in the whole page) in `PagePublished`. `api/internal/notify/notify.go:233` adds every one of them as a recipient, and `deliver` (`notify.go:122`) inserts unconditionally. Result: every publish, and `POST /pages/{id}/versions/{n}/restore` (which goes through the same `pagePublishNow`), re-notifies everyone mentioned anywhere on the page, even if the mention is years old.
- The in-event dedupe (`recipients.add`) is correct, but there is no cross-event dedupe.
- Fix: compute only newly added mentions. In `pagePublishNow`, have `Store.PublishPage` return the previous `content_json` (it already holds the row under `FOR UPDATE`; `cur.ContentJSON` at `pages.go:306`) and publish `MentionedUserIDs = mentions(new) minus mentions(old)`. Optional DB backstop after the code fix (needs a one-time dedupe before the index):
  ```sql
  -- +goose Up
  DELETE FROM notifications a USING notifications b
  WHERE a.type = 'page.mentioned' AND b.type = 'page.mentioned'
    AND a.user_id = b.user_id AND a.page_id = b.page_id
    AND (a.created_at, a.id) < (b.created_at, b.id);
  CREATE UNIQUE INDEX uq_notifications_page_mention ON notifications(user_id, page_id) WHERE type = 'page.mentioned';
  -- +goose Down
  DROP INDEX uq_notifications_page_mention;
  ```
  If the backstop is adopted, `InsertNotification` must use `ON CONFLICT DO NOTHING` and `deliver` must skip `publish` when `RowsAffected() == 0`, otherwise the SSE hub re-pushes the old row.

### P2-3 Dashboard cost grows with total page count (4 queries, each scanning all of `pages`)

- `api/internal/store/dashboard.go:36-52` (`dashboardBase`) puts `EXISTS (SELECT 1 FROM pages c WHERE c.parent_id = p.id AND c.status <> 'trashed')` in the select list of a query with `ORDER BY ... LIMIT 20`. The planner evaluates it as a hashed subplan: a full seq scan of `pages` (48 500 rows) per query even though only 20 rows are returned.
- `recent_updates` additionally has no usable index for `ORDER BY p.updated_at DESC` (full scan + top-N sort: 49 ms at 50k pages). `my_drafts` (`dashboard.go:90-95`) joins all pages to `page_drafts` and filters with an OR across the left join, so the `idx_page_drafts_user` and any draft index are never used (26 ms).
- Measured on PG15, 50k pages, laptop: recent 49 ms, drafts 26 ms, each of the other two a few ms; with the fixes below recent drops to 0.2 ms (index plus EXISTS outside the LIMIT) and drafts to 0.08 ms. Expect 3-4x worse on a Pi.
- Fix:
  1. Migration 0090 (below) adds `idx_pages_recent` (partial, published) and `idx_pages_draft_owner` (partial, draft).
  2. Move the EXISTS out of the limited query by wrapping it:
     ```sql
     SELECT x.id, x.title, x.slug, x.parent_id, x.position, x.status,
            EXISTS (SELECT 1 FROM pages c WHERE c.parent_id = x.id AND c.status <> 'trashed'),
            x.space_key, x.space_name, x.ts, x.updated_by
     FROM (
         SELECT p.id, p.title, p.slug, p.parent_id, p.position, p.status,
                s.key AS space_key, s.name AS space_name, %s AS ts, p.updated_by, %s AS ord
         FROM pages p ... existing joins and WHERE ...
         ORDER BY %s LIMIT %d
     ) x
     ORDER BY x.ord DESC, x.id DESC
     ```
     (`ord` is `p.updated_at`, `v.viewed_at`, the COALESCE for drafts, `st.created_at` for stars.)
  3. Replace the drafts predicate with a UNION so each arm is index-driven (verified plan, same row set):
     ```sql
     SELECT p.id, COALESCE(pd.updated_at, p.updated_at) AS ts
     FROM pages p LEFT JOIN page_drafts pd ON pd.page_id = p.id AND pd.user_id = $1
     WHERE p.status = 'draft' AND p.created_by = $1
     UNION
     SELECT pd.page_id, pd.updated_at
     FROM page_drafts pd JOIN pages p ON p.id = pd.page_id
     WHERE pd.user_id = $1 AND p.status <> 'trashed'
     ```
     then join this to spaces/me/space_members with the existing access predicates and `ORDER BY ts DESC LIMIT 20`.
  - The same hashed-EXISTS pattern in `pageSummaryCols` (`pages.go:36`) is harmless where `parent_id = $1` bounds the row set (ListChildren, ancestors); it matters only for the dashboard and for `GetPageSummary` callers that scan broadly.

### P2-4 Page purge scans `page_views` once per deleted page (missing FK index)

- `api/migrations/0003_pages.sql`: `page_views` PK is `(user_id, page_id)`, so the cascade from `pages(id)` (`page_views_page_id_fkey ... ON DELETE CASCADE`) has no index on `page_id`. Every page deleted by `PurgePage` (`pages.go:552`), including every descendant removed through the `parent_id` cascade, triggers a seq scan of the largest table in the schema (users x pages).
- Measured: purge of one page with 600 000 page_views took 21.9 ms (trigger-dominated); with `idx_page_views_page` it takes 1.6 ms. A 100-page subtree is ~2 s versus ~0.16 s.
- Fix: migration 0090 below (`CREATE INDEX idx_page_views_page ON page_views(page_id)`).

### P2-5 OIDC auto-links an existing local account by email without verifying the email, and bypasses the last-admin guard

- `api/internal/store/users.go:234` `UpsertOIDCUser`: when no `oidc_identities` row exists it selects `users WHERE email = $1 FOR UPDATE` and attaches the new identity to that user (lines ~253-263), including users that have a password and including admins. `api/internal/http/handlers/oidc.go:127-158` never checks an `email_verified` claim (no such check anywhere in `internal/`). An IdP that lets a user set an unverified email equal to an admin's email yields a session as that admin.
- Same function: `is_system_admin = COALESCE($4, is_system_admin)` (admin-group mapping) can demote the last active system admin at login time, skipping the `ErrLastAdmin` guard in `AdminUpdateUser`.
- Also, two simultaneous first logins for the same new subject both miss the identity lookup and the second `INSERT INTO users` fails with a unique violation (surfaces as `user provisioning failed`).
- Fix: (1) in `oidc.go`, reject when the ID token carries `email_verified: false`, and refuse to link to a pre-existing user that has `password_hash IS NOT NULL` unless the claim is exactly `true`; (2) in `UpsertOIDCUser`, when `isSystemAdmin` is false and the target is a current active admin, run the same count-under-lock check as `AdminUpdateUser` (and log instead of demoting when it would leave zero admins); (3) on unique violation of `users.email` retry the identity lookup once. Flag to the security reviewer as well.

## P3

### Schema, indexes, constraints

1. `notifications` list ordering has no matching index. `idx_notifications_user (user_id, read_at, created_at DESC)` serves the unread filter and the unread count (plans verified), but `ListNotifications` (`notifications.go:86`, `ORDER BY n.created_at DESC, n.id DESC`) without `unread=true` scans and sorts every row for the user; with 5 000 rows per user it is 1.5 ms, growing linearly and with no retention. Add `idx_notifications_user_created` (0090).
2. No retention for `activity`, `notifications`, `page_versions` (every publish stores both `content_json` and `content_html`). Not urgent; consider a scheduled prune of read notifications older than N days and activity older than N days next to `sessionCleanup` in `cmd/server/main.go`.
3. Quick switcher `p.title ILIKE '%x%'` (`search/query.go:233`) is a seq scan of `pages` (24 ms at 50k pages, per keystroke). Add `pg_trgm` and a partial GIN index (0091 below). Verified plan: rare term 0.09 ms, common term 15 ms. `pg_trgm` is a trusted extension on PG13+, so the DB owner can create it on pi-lab's PG15; `postgres:15-alpine` and `postgres:16` both ship contrib.
4. Full-text search with a very common term costs about 284 ms for 15 000 matches because `ts_rank_cd` runs over every match before top-N (`search/query.go` list statement). Count is 4.5 ms. Acceptable for a wiki; if it bites, restrict ranking to the top 1000 by `updated_at` or use `ts_rank_cd(..., 1)` normalisation plus a lower `MaxPage`. `idx_pages_search` also indexes drafts and trashed rows; a partial GIN `WHERE status = 'published'` would shrink it and drop the recheck filter.
5. `ListLabels` (`labels.go:43`) evaluates a correlated `count(*)` per label before applying `LIMIT` (270 ms on a dense dataset; fine on sparse real data). Single-pass rewrite verified at 18 ms: `SELECT l.id, l.name, l.created_at, count(*) FROM labels l JOIN page_labels pl ON pl.label_id = l.id JOIN pages p ON p.id = pl.page_id AND p.status = 'published' JOIN spaces s ON s.id = p.space_id WHERE l.name LIKE $3 AND <AccessClause> GROUP BY l.id ORDER BY count(*) DESC, l.name LIMIT $4`.
6. Missing CHECK/FK defence in depth (all verified to apply on PG15 and keep the suite green, see 0092): space key format (handler-only today: `spaces.key` only checks `= upper(key)`), `pages.version/position >= 0`, `page_drafts.base_version >= 0`, comment parent must be on the same page (enforced only in `handlers/comments.go:178`), page parent must be in the same space (enforced only by `pageCheckParent`). A `status = 'trashed' <=> trashed_at IS NOT NULL` CHECK was also tried and rejected: 13 store and handler tests insert `status = 'trashed'` directly without `trashed_at` (attachments, dashboard, labels, search), so it would need fixture changes; the code already tolerates it with `COALESCE(p.trashed_at, p.updated_at)`.
7. `notifications.type` / `activity.type` have no CHECK. Deliberately open-ended; leave, but note it. `pages.slug` is not unique (as the plan requires; confirmed no code relies on uniqueness).
8. Soft-deleted spaces keep `spaces.key` reserved forever (`UNIQUE`); `CreateSpace` returns `ErrConflict` for a deleted space's key (reproduced). If undelete/reuse is wanted, change to a partial unique index `WHERE deleted_at IS NULL` and the `GetSpaceAccessByKey` lookup to prefer the live row.
9. `citext` is created in 0001 but never dropped in its Down (the down-to-zero test counts only tables). Acceptable and conventional; documented here so nobody adds a `DROP EXTENSION`.

### Transactions and concurrency

10. `AdminUpdateUser` can deadlock (`api/internal/store/users.go:159-186`): it locks the target row, then `SELECT ... WHERE is_system_admin AND disabled_at IS NULL FOR UPDATE` with no `ORDER BY`, so two simultaneous demotions/disables of different admins lock in opposite orders. Reproduced: 37 of 60 simultaneous pairs returned SQLSTATE 40P01 (a 500 for one caller; no corruption, the last-admin guard itself held). Fix: lock the admin set first with `ORDER BY id FOR UPDATE` (include the target in that set when it is an admin), then `SELECT ... WHERE id = $1 FOR UPDATE` for non-admin targets.
11. `RecordPageView` (`pages.go:569`) writes on every `GET /pages/{id}` and, because `viewed_at` is in `idx_page_views_recent`, every upsert is a non-HOT update (index churn on the hottest read path). Throttle: `ON CONFLICT (user_id, page_id) DO UPDATE SET viewed_at = EXCLUDED.viewed_at WHERE page_views.viewed_at < now() - interval '1 minute'`.
12. `deliver` (`notify.go:122`) does `InsertNotification` + `GetNotificationView` + `CountUnreadNotifications` per recipient, sequentially, in the bus goroutine (3 queries x watchers; a 200-watcher space = 600 round trips under a 30 s handler timeout). Batch the insert with `unnest` and compute unread counts with one `GROUP BY user_id` query.
13. Activity and notification writes happen after commit via the in-memory bus (by design in the plan); a crash between commit and handler loses them, and a partial failure in `deliver` leaves some recipients notified. Accepted trade-off; documenting.
14. `goose.Up` in `cmd/server/main.go` takes no migration lock; a rolling deploy with two API pods starting together can race. pi-lab runs one replica today. If that changes, use `goose.NewProvider` with `goose.WithSessionLocker`.
15. `UpsertDraft` / `SaveDraft` racing a purge returns an FK violation as a 500 (rare). Map `IsForeignKeyViolation` to `ErrNotFound`.

### Query semantics and correctness details

16. `GetCommentDepth` (`comments.go:69`): `SELECT max(depth)` over an empty CTE returns NULL, which fails to scan into `int` (reproduced: error is `cannot scan NULL into *int`, not `ErrNotFound`). Not reachable today because the handler loads the parent first. Make the destination `*int` and map nil to `ErrNotFound`. The CTE has no depth cap; add `WHERE chain.depth < 100`. Same absent guard in `TrashPage`/`RestorePage` (`pages.go:470,534`), `ListAttachmentObjectKeysUnderPage`, `ListPublishWatcherIDs`, `ListCommentThreadAuthorIDs`, `deletePageSubtreeWatches`; cycles are prevented by `MovePage`, so this is hygiene only. `listAncestors` and the cycle check in `MovePage` do cap at 1000.
17. Activity and notification pagination uses `created_at < $cursor` only (`activity.go:49`, `notifications.go` list) while sorting by `(created_at, id)`; rows with identical timestamps across a page boundary are skipped. Use a `(created_at, id)` row-value cursor.
18. `has_children` leaks the existence of other users' draft children: `pageSummaryCols` (`pages.go:36`) and `ListSpaceTree` count children that `ListChildren` then hides. Cosmetic.
19. `ListSpaceTree` (`spaces.go:408`) can return a published page whose parent is another user's draft; the parent is filtered out, so the child is an orphan in the payload. The web tree must treat unknown `parent_id` as root.
20. `ListSpaceActivity` shows activity for a never-published page that was trashed (status `trashed`, version 0) to everyone, while `IsPageVisible` hides it. Mirror the visibility rule (`p.status <> 'trashed' OR p.version > 0 OR creator/admin`).
21. `SearchPages` "author" filter uses `p.updated_by` (last editor), not `created_by`. Matches the displayed author, but confirm it is what the plan's "author" means.
22. Times: all columns are `timestamptz`. Go clock is used for `sessions.expires_at` (`handlers/auth.go:30`, `oidc.go:168`) and `users.disabled_at` (`users.go:206`) while validity is checked with DB `now()`; skew between the API pod and Postgres shifts session lifetime by the skew. Use `now() + $ttl` in SQL if exactness matters. Everything else uses `now()` or `COALESCE($n, now())`.
23. `labels` upsert uses `ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name` (`labels.go:175`), which rewrites the label row on every call (dead tuples). Prefer `INSERT ... ON CONFLICT DO NOTHING` followed by `SELECT id, name, created_at FROM labels WHERE name = ANY($1)`. Names are sorted, so no deadlock risk (verified by reading).
24. Unbounded result sets (intentional, all bounded by domain size, listed for completeness): `ListSpaceTree`, `ListSpaceTrash`, `ListSpaceLabelPages`, `ListSpaceLabels`, `ListPageComments`, `ListPageVersions`, `ListChildren`, `ListWatches`, `ListUsers`, `ListSpacePagesForExport` (loads all page bodies into memory for a space export). Everything user-facing and growth-prone (`ListNotifications`, `ListSpaceActivity`, `SearchPages`, quick search, labels, dashboard, user search) has a LIMIT, and the handlers clamp `limit` (30 default, 100 max).
25. Restore fallback differs from the plan wording: when the parent is trashed the page is restored under the space home page (or root if home is unavailable), not always the space root (`pages.go:497-535`). Sensible and consistent with `Create` defaulting to home; keep, and update the plan text.

## Proposed migrations (integration range)

All three were applied to a populated PG15 database and the whole `./internal/...` suite, including the Down-to-0 and Up cycle test, passes with them present in a scratch copy of `api/`.

`api/migrations/0090_integration_indexes.sql`
```sql
-- +goose Up
CREATE INDEX idx_page_views_page ON page_views(page_id);
CREATE INDEX idx_pages_recent ON pages(updated_at DESC, id DESC) WHERE status = 'published';
CREATE INDEX idx_pages_draft_owner ON pages(created_by) WHERE status = 'draft';
CREATE INDEX idx_notifications_user_created ON notifications(user_id, created_at DESC, id DESC);

-- +goose Down
DROP INDEX idx_notifications_user_created;
DROP INDEX idx_pages_draft_owner;
DROP INDEX idx_pages_recent;
DROP INDEX idx_page_views_page;
```

`api/migrations/0091_title_trgm.sql`
```sql
-- +goose Up
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX idx_pages_title_trgm ON pages USING gin (title gin_trgm_ops) WHERE status <> 'trashed';

-- +goose Down
DROP INDEX idx_pages_title_trgm;
```

`api/migrations/0092_integrity_checks.sql`
```sql
-- +goose Up
ALTER TABLE spaces ADD CONSTRAINT spaces_key_format CHECK (key ~ '^[A-Z][A-Z0-9]{1,9}$');
ALTER TABLE pages ADD CONSTRAINT pages_version_nonneg CHECK (version >= 0 AND position >= 0);
ALTER TABLE page_drafts ADD CONSTRAINT page_drafts_base_version_nonneg CHECK (base_version >= 0);

ALTER TABLE comments ADD CONSTRAINT comments_page_id_id_key UNIQUE (page_id, id);
ALTER TABLE comments DROP CONSTRAINT comments_parent_id_fkey;
ALTER TABLE comments ADD CONSTRAINT comments_parent_same_page_fkey
    FOREIGN KEY (page_id, parent_id) REFERENCES comments (page_id, id) ON DELETE CASCADE;

ALTER TABLE pages ADD CONSTRAINT pages_space_id_id_key UNIQUE (space_id, id);
ALTER TABLE pages DROP CONSTRAINT pages_parent_id_fkey;
ALTER TABLE pages ADD CONSTRAINT pages_parent_same_space_fkey
    FOREIGN KEY (space_id, parent_id) REFERENCES pages (space_id, id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE pages DROP CONSTRAINT pages_parent_same_space_fkey;
ALTER TABLE pages ADD CONSTRAINT pages_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES pages (id) ON DELETE CASCADE;
ALTER TABLE pages DROP CONSTRAINT pages_space_id_id_key;

ALTER TABLE comments DROP CONSTRAINT comments_parent_same_page_fkey;
ALTER TABLE comments ADD CONSTRAINT comments_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES comments (id) ON DELETE CASCADE;
ALTER TABLE comments DROP CONSTRAINT comments_page_id_id_key;

ALTER TABLE page_drafts DROP CONSTRAINT page_drafts_base_version_nonneg;
ALTER TABLE pages DROP CONSTRAINT pages_version_nonneg;
ALTER TABLE spaces DROP CONSTRAINT spaces_key_format;
```

Notes: 0092 is optional hardening (P3); 0090 carries the P2-3 and P2-4 fixes and is the one to land. The `-- +goose` lines are goose directives, not comments. Existing deployments with a key that violates the format would fail 0092 on `spaces_key_format`; run `SELECT key FROM spaces WHERE key !~ '^[A-Z][A-Z0-9]{1,9}$'` first (handler validation means there should be none).

## Verified correct

Migrations
- Every file has Up and Down; reverse-order Down drops cleanly, with data, on PG15 and PG16; Up again succeeds. `TestMigrationsDownAndUp` asserts zero tables remain after a full down.
- Goose numbering ranges respected; no gaps that goose rejects.
- `timestamptz` used for every timestamp column; IDs are UUIDv7 generated in Go (no `uuid.New()`, `gen_random_uuid()` or uuid defaults in non-test code); nullable columns are scanned into pointers or `json.RawMessage` consistently (no scan-NULL bugs found other than item 16).

Constraints and FKs
- `spaces.key` unique with upper CHECK; `labels.name` unique with lower CHECK (and `Normalize` lowercases search/label inputs); `page_versions (page_id, version)` unique; enum CHECKs on `spaces.visibility`, `space_members.role`, `pages.status`, `watches.target_type`; `attachments.size >= 0`.
- Cascade behaviour matches the plan: purge cascades through `pages` to versions, drafts, views, stars, labels, comments, attachments; `spaces.home_page_id` is `SET NULL`; users are never hard-deleted (no `DELETE FROM users` anywhere), so the default NO ACTION user FKs are safe; `notifications` and `activity` carry no page/comment FKs and survive purge (`activity.space_id` cascades only with a space, which is soft-deleted in practice); watches for a purged subtree are deleted in the same tx (`deletePageSubtreeWatches`).
- Page slug is not unique and nothing depends on it being unique.

Indexes (each pattern checked against a plan)
- Tree/space listing (`idx_pages_space_status`, bitmap OR, 1 ms for 1 000 pages), children and descendant CTEs (`idx_pages_parent`), ancestor CTE (PK), search (GIN, count 4.5 ms), search filters (space key unique, label via `idx_page_labels_label` + labels unique), labels by page (PK prefix) and by space (pages index + PK), comments by page (`idx_comments_page`), threads (`idx_comments_parent`), attachments by page, unread count and unread list, activity by space, watches by target (`idx_watches_target`) and by user (PK), recently viewed (`idx_page_views_recent`, 0.19 ms), stars by user (PK), drafts by user (`idx_page_drafts_user`), expired sessions (`idx_sessions_expires`).

Transactions and invariants (including concurrent probes on PG15)
- `PublishPage` locks the page row `FOR UPDATE`, compares `base_version`, bumps the version, writes `page_versions`, and deletes the actor's draft in one tx. 12 concurrent publishes with the same base: exactly 1 succeeded, 11 got `VersionConflictError`.
- Position invariants: `pg_advisory_xact_lock(hashtextextended(space_id))` serialises create, move, trash, restore and import per space; 25 concurrent creates under one parent produced no duplicate positions; 20 rounds of simultaneous `A under B` / `B under A` moves never produced a cycle; `pageRenumber` only rewrites rows whose position changed.
- Last space admin: `lockSpaceAdmins` (`ORDER BY user_id FOR UPDATE`) is race-free; 30 rounds of simultaneous "demote A" / "remove B" never left a space without an admin. Last system admin guard in `AdminUpdateUser` is correct apart from the deadlock in item 10.
- `UpsertSpaceMember` insert-vs-update detection via `xmax = 0` is valid on PG15 and PG16.
- Restore semantics: restores exactly the subtree trashed together (`trashed_at` equality; a child trashed earlier stays trashed under the restored parent and restores independently, its own descendants with it), reattaches to the old parent or falls back as noted in item 25, restores never-published pages to `draft` and others to `published`, appends at the end of the sibling list.
- Purge cannot race restore (both `FOR UPDATE`, purge re-reads status after waiting), and non-trashed pages cannot be purged.
- No handler or helper calls the pool while holding a tx (no nested-acquire deadlock with the small default pool); handlers never touch `Store.Pool`.

CTEs and queries
- `listAncestors` and the `MovePage` cycle check have depth caps; trashed and draft filtering in ancestors/children/tree/dashboard/search/quick-search/`FilterNotifiable` is consistent with `IsPageVisible` (draft: creator or space admin; trashed: editors, creators for never-published).
- Search: `websearch_to_tsquery` with the same `english` config as the generated column; ranking `ts_rank_cd` with title weight A over content B; headline computed only for the page's rows; random per-process marker tokens are HTML-escaped around, so snippets cannot inject markup; access clause excludes soft-deleted spaces and private spaces without membership; count and list share the same filter snapshot.
- Dashboard access subquery: system admin or member or internal; deleted spaces excluded; trashed excluded; drafts only for creator, system admin, space admin; the LEFT JOINs on `space_members` and `page_drafts` are 1:1 (PK), so no duplicate rows; user refs resolved with one batch query (no N+1).
- Notification recipient dedupe within an event: mention beats reply beats watcher type, actor excluded, one row per user; `FilterNotifiable` re-checks account disabled, space deleted/membership/visibility and draft visibility at delivery time.
- No N+1 loops in handlers (grep for store calls inside `range` loops: none); batch helpers exist for user refs, labels-for-pages, comment counts.

Postgres 15 compatibility
- Whole `./internal/...` suite green on `postgres:15.19`; no 16+/17+ syntax or functions used (`MERGE ... RETURNING`, `JSON_*`, `any_value`, `pg_input_*`, `uuidv7` all absent). `hashtextextended` (11+), generated stored columns (12+), `Memoize` plans all fine on 15. Note the testcontainers default image in `internal/testutil/db.go` is `postgres:16-alpine`; consider making it honour an env var or running CI once against 15 since pi-lab runs 15.

## Test coverage to add with the fixes

- Home page: move home under child then trash child returns `ErrHomePage`; moving home under another page returns `ErrHomePage`.
- Mentions: publish twice with the same `@user`; assert one `page.mentioned` notification.
- `AdminUpdateUser`: simultaneous demotion of two of three admins never returns an error other than `ErrLastAdmin`.
- Dashboard: assert `EXPLAIN` of the drafts query does not seq scan `pages` on a 5 000-row fixture (or just keep the existing correctness tests and add the indexes).
- OIDC: login with `email_verified=false` against an existing password user is rejected.
