# B2 pages: notes

## What was built

Files (all under `api/`): `internal/store/{pages,versions,drafts,template_content}.go`, `internal/store/pages_test.go`, `internal/http/handlers/{pages,versions}.go`, `internal/http/handlers/{pages_test,versions_test}.go`. No migration was needed: `0003_pages.sql` already has everything. `go.mod`/`go.sum` untouched, no comments in code, no git.

### Routes (flat, under `/api/v1`)

| Route | Middleware | Result |
|---|---|---|
| `POST /spaces/{key}/pages` | `SpaceByKey(PermEditPages)` | 201 Page |
| `GET /pages/{pageID}` | `SpaceByPage(PermViewSpace)` | PageDetail, upserts `page_views` |
| `PATCH /pages/{pageID}` | edit | Page (title and slug only, no version) |
| `DELETE /pages/{pageID}` | `SpaceByPage(PermPurgeTrash)` | 204, trashed pages only |
| `PUT/GET/DELETE /pages/{pageID}/draft` | edit | PageDraft / 404 / 204 |
| `POST /pages/{pageID}/publish` | edit | Page, 409 `version_conflict` |
| `POST /pages/{pageID}/move`, `/trash`, `/restore`, `/copy` | edit | Page / 204 / Page / 201 Page |
| `GET /pages/{pageID}/children` | view | PageSummary[] |
| `POST/DELETE /pages/{pageID}/star` | view | 204 |
| `POST /pages/{pageID}/editing` | edit | `{editors}` without the caller |
| `GET /pages/{pageID}/versions`, `GET .../versions/{version}`, `POST .../versions/{version}/restore` | view, view, edit | PageVersionSummary[] / PageVersion / Page |

Non-detail page responses are `domain.Page` plus an extra `space_key` (the web `Page.space_key` is optional). `PageDetail` has every field the TypeScript type declares plus `my_role`.

### Behaviour worth knowing

- Draft-status visibility is enforced in `pageLoad` (handlers/pages.go), which every handler (including all of versions.go) goes through: creator or effective-admin role, otherwise 404. Trashed pages are viewable by anyone who can view the space; mutating endpoints on a trashed page answer 409 `{"code":"page_trashed"}`; restore and purge are the exceptions.
- Create: parent defaults to the space home page (`PageSpaceHomeID`), or root when the space has none. Explicit `null` parent is treated like "absent" (so a new root page next to the home page cannot be created through the API; restore and move can still produce roots). Empty title becomes "Untitled" (the template title wins when a template is used). New page content is `{"type":"doc","content":[{"type":"paragraph"}]}` with empty html. Positions are 0-based: first sibling gets 0, then max+1. Position counts non-trashed siblings only.
- Publish: sanitise html, `content_text` = `ExtractText(json)` then `ExtractTextFromHTML(html)` fallback, `version++`, `published_at` set once, `page_versions` row, caller's draft deleted, all in one `WithTx` with `FOR UPDATE`. Slug is regenerated from the title on publish and on PATCH. `PagePublished` is published after commit with `content.MentionedUserIDs(page.ContentJSON)`. `content_json` must be a JSON object or null (400 otherwise). Request bodies over 1 MiB answer 413.
- Version restore reuses `PublishPage` with `Force: true`, message `Restored version n`, content re-sanitised and `content_text` re-extracted; it also fires `PagePublished`.
- Move: advisory lock per space (`pg_advisory_xact_lock`) plus row lock; parent must be same space and non-trashed; cycle check with a recursive CTE; `after_id` wins over `position`; neither means append; `position` is clamped. Both the new and the old sibling lists are renumbered 0..n-1. `PageMoved` carries old and new parent. Moving does not touch `updated_at`.
- Trash: one recursive CTE sets status/`trashed_at`/`trashed_by` on the page and every non-trashed descendant (same `now()` for all rows; already-trashed descendants keep their own timestamp). Home page: 400. Already trashed: 409 `page_trashed`.
- Restore: restores the page plus descendants with the same `trashed_at`; version 0 pages come back as `draft`, others as `published`. If the parent is trashed or missing, the page goes under the space home page (when the home exists, is live and is not the page itself) or root; the restored root is appended at the end of its siblings.
- Copy: new draft-status page (version 0) with the source's title (or `title`, default "Copy of <title>"), content_json/html/text; parent = `parent_id` if given, otherwise the source's parent; same space only. Body is optional.
- Presence: `POST /editing` touches `Presence` with `user.Ref()`; both it and `GET /pages/{id}` return `Presence.Editors` minus the caller, never null.

### Exported store helpers other agents can reuse

```
type PageSummary struct{ID, Title, Slug, ParentID, Position, Status, HasChildren}   // json = web PageSummary
type DraftRef struct{UpdatedAt, BaseVersion}
GetPage(ctx, id) (domain.Page, error)
GetPageSummary(ctx, id) (PageSummary, error)
ListAncestors(ctx, id) ([]PageSummary, error)              // root-first, excludes the page, never nil
ListChildren(ctx, parentID, viewerID, viewerIsAdmin) ([]PageSummary, error)   // non-trashed, draft-aware, by position
IsPageVisible(page domain.Page, userID uuid.UUID, role domain.Role) bool      // package-level func
PageSpaceHomeID(ctx, spaceID) (*uuid.UUID, error)
CreatePage(ctx, NewPage) / UpdatePageTitle / PublishPage(ctx, PublishParams) / MovePage(ctx, MoveParams) (MoveResult, error)
TrashPage(ctx, id, actorID) / RestorePage(ctx, id) / PurgePage(ctx, id)
RecordPageView(ctx, userID, pageID) / StarPage / UnstarPage / IsPageStarred
UpsertDraft(ctx, DraftParams) / GetDraft / GetDraftRef / DeleteDraft
ListPageVersions(ctx, pageID) ([]PageVersionSummary, error) / GetPageVersion(ctx, pageID, n) (PageVersionView, error)
store.PageEmptyDocJSON
errors: ErrInvalidParent, ErrPageCycle, ErrInvalidAnchor, ErrHomePage, ErrPageTrashed, ErrNotTrashed, *VersionConflictError{Current}
```
Slugs come from `store.SlugFromTitle` (defined by the spaces agent in `store/spaces.go`).

### Integration call sites to wire later

Both live in `handlers/pages.go` on `PageHandler` and are called once, from `Get`:

1. Labels: `func (h *PageHandler) pageLabels(ctx context.Context, pageID uuid.UUID) ([]domain.Label, error)` currently returns `[]domain.Label{}`. The labels agent replaces its body with the store call. `PageDetail.labels` serialises as `[]`.
2. Watching: `func (h *PageHandler) pageWatching(ctx context.Context, pageID, userID uuid.UUID) (bool, error)` currently returns `false`. The watch agent replaces its body.

Templates: `store/template_content.go` holds the stub `GetTemplateContent` (returns `ErrNotFound`, so `template_id` currently answers 404 "template not found"). The templates agent replaces that whole file keeping `type TemplateContent{Title string; ContentJSON []byte; ContentHTML string}` and the method signature. The handler sanitises `ContentHTML` itself and extracts text. The function takes only an id, so the templates agent must make sure a caller cannot read a template from a space they cannot see (the create handler does no scoping check of its own).

## Deviations and judgement calls

1. No `store.SlugFromTitle` fallback was needed; it existed when I built.
2. `IsPageVisible` restricts only `status == draft`. A never-published page that was later trashed is visible to viewers like any trashed page (spec: trashed pages are viewable). Restoring it puts it back to `draft`, which re-hides it.
3. `ListAncestors` does not filter by draft visibility; a published child under another user's unpublished draft parent would show that parent's title in breadcrumbs. Not reachable through the normal UI flow (children of a draft are created by its owner).
4. Trashed-page mutations answer 409 with code `page_trashed` rather than 400.
5. Purge does not delete attachment objects from S3; if the attachments table cascades, the attachments agent needs to clean objects (or sweep orphans). `page_versions`, `page_drafts`, `page_views`, `page_stars` and child pages cascade via FKs.
6. `GET /pages/{id}` logs and continues if the `page_views` upsert fails (reading is not blocked by a bookkeeping write).
7. `my_role` is returned in `PageDetail` (declared optional in `types.ts`).
8. Tests build their own router (`newPagesEnv` in `pages_test.go`) because `helpers_test.go`'s `newEnv` does not expose the event bus; everything else reuses `env`/`client` from it. The test helpers are all prefixed `pages`.

## Gate

```
$ cd api && gofmt -l . && go build ./... && go vet ./... && go test -count=1 ./...
(gofmt: no output)
?   	github.com/hayward-solutions/knowledge/api/cmd/server	[no test files]
ok  	github.com/hayward-solutions/knowledge/api/internal/auth	0.502s
ok  	github.com/hayward-solutions/knowledge/api/internal/bootstrap	1.726s
ok  	github.com/hayward-solutions/knowledge/api/internal/config	0.160s
ok  	github.com/hayward-solutions/knowledge/api/internal/content	0.199s
?   	github.com/hayward-solutions/knowledge/api/internal/deps	[no test files]
ok  	github.com/hayward-solutions/knowledge/api/internal/domain	0.164s
ok  	github.com/hayward-solutions/knowledge/api/internal/events	0.165s
ok  	github.com/hayward-solutions/knowledge/api/internal/http	0.092s
ok  	github.com/hayward-solutions/knowledge/api/internal/http/handlers	18.342s
ok  	github.com/hayward-solutions/knowledge/api/internal/http/middleware	2.165s
ok  	github.com/hayward-solutions/knowledge/api/internal/search	0.068s
ok  	github.com/hayward-solutions/knowledge/api/internal/storage	0.234s
ok  	github.com/hayward-solutions/knowledge/api/internal/store	6.176s
?   	github.com/hayward-solutions/knowledge/api/internal/testutil	[no test files]
```
Integration tests ran against the real Postgres testcontainer (no skips). `go test -race` on the pages handler tests also passes.

## Remediation round (coordinator feedback)

1. Trashed pages: `store.IsPageVisible` now hides trashed pages from viewers (they need effective role editor or admin) and keeps never-published trashed pages (`version == 0` and `published_at IS NULL`) creator-or-admin only. Because every page and version handler goes through `pageLoad`, `GET /pages/{id}`, `/children`, `/versions`, `/versions/{n}`, draft endpoints and star all answer 404 to viewers. This supersedes deviation 2 above. Test: `TestIsPageVisible` (store) and `TestPagesTrashRestorePurge` (viewer 404, editor 200 on detail/children/versions; viewer star 404). Note `testutil.CreatePage` pages (published, version 0, no `published_at`) count as never-published once trashed.
2. `store.TemplateContent` gained `SpaceID uuid.UUID` (Nil = global). `PageHandler.Create` answers 404 "template not found" when the template's `SpaceID` is neither Nil nor the current space. The stub still returns `ErrNotFound`, and the templates table does not exist yet, so this branch has no test; the templates agent's integration test should cover a foreign-space template.
3. `pageLabels` now returns `h.Store.ListPageLabels`. Test: `TestPagesDetailLabels`. `pageWatching` is unchanged.
4. Purge lists `ListAttachmentObjectKeysUnderPage` (only when `h.Storage != nil` and the page is trashed) before `PurgePage`, then calls `attachmentPurgeObjects` after a successful delete. A rejected purge deletes nothing. Test: `TestPagesPurgeRemovesAttachmentObjects` (own map-backed `pagesObjectStore`; descendants' objects removed, unrelated ones kept). `newPagesEnv` now always wires that fake as `Deps.Storage`.
5. Migration `api/migrations/0020_pages_parent_index.sql`: `idx_pages_parent` on `pages(parent_id)`, Up and Down.
6. Breadcrumbs: new `Store.ListVisibleAncestors(ctx, id, viewerID, role)` filters hidden drafts, hidden never-published trashed pages, and trashed ancestors for viewers, keeping the rest of the chain. `ListAncestors` stays unfiltered. `PageHandler.Get` uses the visible variant. Tests: `TestPageVisibleAncestors` (store) and `TestPagesBreadcrumbsSkipHiddenDrafts` (handler). This supersedes deviation 3 above. Deviation 5 (S3 cleanup on purge) is resolved by item 4.

Gate rerun after these changes: `gofmt -l .` clean; `go build ./...`, `go vet ./...`, `go test -count=1 ./...` all ok (handlers 25.1s, store 10.5s).

## Final review fixes (P2-1, P2-2)

1. Home page: `TrashPage` now runs the descendant CTE as a read inside the transaction and returns `ErrHomePage` if `spaces.home_page_id` is anywhere in the subtree (nothing is written on refusal). `MovePage` reads `home_page_id` inside its transaction and returns `ErrHomePage` for any move of the home page, including a same-parent reorder. `ErrHomePage` is reused (message now "the space home page cannot be trashed or moved") and maps to 400. Tests: `TestPageHomeIsNeverTrashedOrMoved` (store, reparents home with raw SQL to reach the legacy bad state) and `TestPagesHomeCannotBeMovedOrTrashedViaAncestor` (handler).
2. Mentions: new `Store.PublishPageWithPrevious` returns the page's `content_json` from before the update (read under `FOR UPDATE`; nil when the page was version 0, so a first publish notifies every mention). `PublishPage` is kept as a wrapper with the old signature (export tests use it). `pagePublishNow`, used by both publish and version restore, sets `MentionedUserIDs` to mentions in the new content minus mentions in the previous content (`pageNewMentions`). Test `TestPagesPublishEventOnlyCarriesNewMentions`: first publish, identical republish (none), added mention (exactly one), restore of identical content (none), restore of older version (none), restore that re-adds a mention (that one).

Gate: `gofmt -l .` clean; `go build ./...`, `go vet ./...`, `go test -count=1 ./...` all ok.

## Security fix: tree depth and cycle-proof CTEs (review P1-2)

- `ErrTreeTooDeep` ("page tree too deep", mapped to 400). Max depth 64 (`pageMaxDepth`): a page's depth is its ancestor count, and a parent whose ancestor count is 64 or more is refused. Enforced inside the transaction by `CreatePage` (and so by copy), `MovePage` and `RestorePage`. `MovePage` additionally runs `pageSubtreeFits`: a bounded level-by-level walk (`parent_id = ANY(frontier)`, never more than 64 iterations, counts trashed descendants too) that fails when new depth plus subtree height exceeds 64. `RestorePage` checks the final parent's depth and the restored subtree height. `ImportPages` lives in another agent's file and is not covered here.
- Every recursive CTE in `store/pages.go` now walks id-only columns with `UNION` (ancestor walk in `listAncestors` and `pageAncestorCount`, the move cycle check, both `TrashPage` CTEs and the `RestorePage` CTE), so a corrupted cycle dedupes and terminates. No depth column can be carried (it would defeat the dedupe), so the "depth guard of 1000" is applied where it still works: `listAncestors` caps its row count and chain length at `pageMaxWalk = 1000`, and the move cycle check no longer has a depth cutoff at all (it was the bug). `listAncestors` now fetches the ancestor set unordered, orders it root-first in Go by following parent links, and applies draft/trash visibility in Go via `IsPageVisible` (the SQL `CASE` filter is gone; behaviour is unchanged). On a corrupted cycle it returns the bounded set in arbitrary order. `versions.go` and `drafts.go` contain no recursive queries.
- The same `UNION ALL` patterns in other agents' files (`attachments.go` `ListAttachmentObjectKeysUnderPage`, `watches.go`) are outside my files and still need the same change.
- Tests: `TestPageTreeDepthLimit`, `TestPageMoveHonoursDepthAndSubtreeHeight`, `TestPageRestoreHonoursDepthLimit`, `TestPageRecursiveQueriesTerminateOnCorruptCycle` (raw-SQL cycle; ancestors, visible ancestors, move, create, trash and restore all return inside a 10 s deadline with bounded results) in the store, and `TestPagesTreeDepthLimit` (create, copy, move return 400 "page tree too deep", move at the limit succeeds) in the handlers.

Gate: `gofmt -l .` clean; `go build ./...`, `go vet ./...`, `go test -count=1 ./...` all ok.
