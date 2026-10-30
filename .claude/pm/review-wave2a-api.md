# Wave 2a API review (B1 spaces, B2 pages, B3 comments, B4 labels+search, B5 attachments)

State at review time: `go vet ./...` clean, `go test -count=1 ./internal/...` all green (Docker up, integration tests ran), `gofmt -l` clean. No Go or SQL comments in any reviewed file, no handler touches `Store.Pool`, every route is registered flat with `r.With(...)`. Paths below are relative to `api/`.

No P1 found. Every route's middleware perm matches the plan matrix, every SQL statement is parameterised, LIKE/ILIKE inputs are escaped, snippets are escaped before `<mark>` substitution, and publish/move/trash/restore/copy/space-create are single transactions with events published after commit. The P2 items are the ones to fix before the wave is committed.

## P2 (should fix)

### P2-1 Never-published draft pages leak once trashed
- `store/pages.go:424-457` (`TrashPage` overwrites `status` with `trashed`), `store/pages.go:99-104` (`IsPageVisible` only protects `status = 'draft'`), `store/spaces.go:432-462` (`ListSpaceTrash` has no draft filter), `handlers/spaces.go:44,340-348` (trash is `PermViewSpace`).
- A page that was created but never published (`version = 0`) is private to its creator and space admins. When the creator (or an admin) trashes it, its status becomes `trashed`, so `GET /spaces/{key}/trash` lists its title to every viewer and `GET /pages/{id}` returns it (title plus the template content copied in at create time). `RestorePage` turns it back into a draft, so the exposure is silent and temporary.
- Fix: in `ListSpaceTrash` take `viewerID, viewerIsAdmin` and add `AND (p.version > 0 OR p.created_by = $2 OR $3)`. In `IsPageVisible` treat `status = trashed AND version = 0` like a draft. Also see P2-2 which removes most of the exposure.

### P2-2 Trashed pages are fully readable by viewers
- `handlers/pages.go:95-121` (`pageLoad`), `handlers/versions.go:38-66`, `handlers/attachments.go:255-284`, `handlers/comments.go:88-109`, route perm `PermViewSpace` on `spaces/{key}/trash`.
- `pages_test.go:752` asserts "viewers can open trashed pages", so this is a deliberate choice, but the plan only says draft pages are restricted and a trashed page's content, history, comments and attachments remain downloadable by anyone with view. Trash is normally an editor/admin surface.
- Fix (decision for Matt): in `pageLoad` return 404 when `page.Status == trashed && !RoleAllows(sc.Role, PermEditPages)`, and make `/spaces/{key}/trash` `PermEditPages`. Apply the same page-status check in `attachmentLoad` and `commentPageVisible`/`CommentHandler.List`. Update `pages_test.go:752` and `spaces_test.go:632` accordingly.

### P2-3 Write endpoints on trashed pages are inconsistent
- Comments: `handlers/comments.go:111-127` (Create), `223-251` (Update), `270-302` (Resolve/Reopen) never check `info.Status`; a viewer can add and resolve comments on a trashed page. Labels: `handlers/labels.go:73-97` (`labelGate`) lets `SetPageLabels` run on a trashed page. Attachments: Upload rejects trashed (`attachments.go:98`, with a bare 400) but Delete (`329-349`) does not.
- Pages/versions already answer `409 page_trashed`.
- Fix: in `CommentHandler.Create`, `labelGate` (for PUT only) and `Upload`/`Delete` return `httpErrCode(w, 409, "page_trashed", "page is trashed")` when `Status == trashed`. For comments, `GetCommentWithPage` already returns `info.Status`; reject in Update/Resolve/Reopen too. Change `attachments.go:99` from 400 to the same 409 code.

### P2-4 Template lookup has no space scoping hook
- `store/template_content.go:9-17`, `handlers/pages.go:226-247`.
- `TemplateContent` carries only title/json/html. When B6 fills `GetTemplateContent` by id, `POST /spaces/{key}/pages` with `template_id` of a template in another (private) space will copy that template's content into the caller's space. The handler cannot check because the struct has no `SpaceID`.
- Fix now, before B6: add `SpaceID *uuid.UUID` to `TemplateContent`, and in `PageHandler.Create` after the lookup do `if tpl.SpaceID != nil && *tpl.SpaceID != sc.ID { httpErr(w, 404, "template not found"); return }`. Add the field to the B6 brief.

### P2-5 `PageDetail.labels` and `watching` are hard-coded stubs
- `handlers/pages.go:193-199`; used at `299-308`.
- `GET /pages/{id}` always returns `"labels": []` and `"watching": false`. `store.ListPageLabels` already exists (`store/labels.go:108`). Labels set via `PUT /pages/{id}/labels` therefore never show on the page view, which C3's label editor and C2's page view both depend on.
- Fix: `labels, err := h.Store.ListPageLabels(ctx, pageID)`. Leave `watching` for B7 but note it in the B7 brief.

### P2-6 Purging a page orphans its attachment objects in S3
- `handlers/pages.go:538-548`, `store/pages.go:530-542`, `migrations/0050_attachments.sql:3` (`page_id ... ON DELETE CASCADE`).
- `DELETE /pages/{id}` removes the page and cascades to descendants and their `attachments` rows, so the S3 keys are lost with no way to garbage-collect them. `AttachmentHandler.Delete` does remove the object, purge does not.
- Fix: add `Store.ListSubtreeAttachmentKeys(ctx, pageID)` (recursive CTE over `pages`, join `attachments`), call it in `Purge` before `PurgePage`, then after a successful purge delete each key via `h.Storage` (best effort, log failures, skip when `h.Storage == nil`).

### P2-7 Missing indexes: `pages(parent_id)` (and no 0010 / 0020 migrations exist)
- `migrations/0003_pages.sql:30-32` has `idx_pages_tree (space_id, parent_id, position)`, which cannot serve `WHERE parent_id = $1` without `space_id`. No file in the 0010 or 0020 ranges was created even though the plan assigns them to B1/B2.
- Queries that filter on `parent_id` alone: `pageSummaryCols` EXISTS subquery (`store/pages.go:36-37`, run once per row in children, ancestors, search, quick search, labels), `ListChildren` (`store/pages.go:131-143`), recursive CTEs in `TrashPage`/`RestorePage` (`448-453`, `512-517`), and the `ON DELETE CASCADE` FK on `parent_id`.
- Fix: add `migrations/0020_page_indexes.sql` with `CREATE INDEX idx_pages_parent ON pages(parent_id)` (Up and Down). Optionally make it `(parent_id, status)`.

### P2-8 Existence oracle: 403 vs 404 reveals private spaces and ids (middleware, outside the reviewed files but required by checklist item 7)
- `middleware/space.go:105-111` returns 404 for an unknown key/page/comment/attachment and 403 for one that exists in a space the caller cannot see. `spaces_test.go:224,237` locks that in.
- Space keys are 2-10 alphanumerics, so private space keys are trivially enumerable; ids are UUIDv7 so less so but the oracle still exists on every `{pageID}`, `{commentID}`, `{attachmentID}` route.
- Fix: in `resolveSpace`, when `effectiveRole` fails return `404 not found` instead of 403 (the caller cannot see the space, so the space does not exist for them). Keep 403 only for the post-resolution `RequireSpacePerm` failure for callers who do have a role. Update the tests that expect 403 for outsiders.

### P2-9 PDFs are served inline, contradicting the plan
- `handlers/attachments.go:34-41` (`application/pdf` in `attachmentInlineTypes`), `320-322` (CSP skipped for PDFs).
- Plan: "images only inline; everything else forced to attachment". An inline PDF on the app origin runs in the browser's PDF viewer under the app origin with no CSP sandbox.
- Fix: drop `application/pdf` from `attachmentInlineTypes` and always send `Content-Security-Policy: default-src 'none'; sandbox`. If Matt wants PDF preview, serve it from `/attachments/{id}/content` only and keep the sandbox header.

## P3 (nits and hardening)

1. `handlers/pages.go:274`, `store/pages.go:114-129`: breadcrumbs include ancestors the caller cannot see (a published child under another user's draft parent exposes that draft's title). Filter ancestors with `status <> 'trashed' AND (status <> 'draft' OR created_by = $2 OR $3)` or drop hidden ones.
2. `store/pages.go:36-37`: `has_children` counts other users' hidden drafts, so the UI shows an expander over an empty list. `ListChildren`/tree filter drafts, the summary column does not. Pass viewer info or compute `has_children` in the same visibility terms.
3. `handlers/pages.go:217-225`: `parent_id: null` (or omitted) creates the page under the home page, so a root-level page cannot be created although `CreatePageInput.parent_id?: string | null` suggests it can. Decide and document; the move endpoint does allow `null` = root.
4. `store/pages.go:156-169` (`pageCheckParent`), used by create, move, copy: a hidden draft page of another user is accepted as a parent and confirms its existence. Reject parents the caller cannot see.
5. `store/spaces.go:233-244`: `home_page_id` can be set to a draft page (invisible to everyone but its creator and admins); require `status = 'published'`. Also nothing stops moving the home page under another page (`store/pages.go:326`).
6. `store/pages.go:530-542`: purge cascades via `parent_id ON DELETE CASCADE` to descendants that were trashed separately and earlier, hard-deleting items still sitting in trash. Either document or restrict to descendants sharing `trashed_at`.
7. `handlers/pages.go:570-582` (Copy): content is copied without `content.SanitizeHTML`. Stored HTML was sanitised on write so this is defence in depth, but the checklist wants every path to sanitise; wrap `req.page.ContentHTML`. Copied images still point at the original page's attachment ids and break if the original is purged.
8. `handlers/pages.go:330-356` (PATCH title) and `store/pages.go:253-264`: a rename changes `pages.title` but writes no `page_versions` row, so history shows the old title for the current version.
9. `handlers/pages.go:178-191`, `handlers/comments.go:184-192`: `MentionedUserIDs` is taken from client JSON without checking the user exists or can see the space; B7 must filter by space access before notifying, or filter here.
10. `handlers/pages.go:149-158` and `util.go:25`: `content_json` is stored as unvalidated JSON (only "starts with `{`"), so link hrefs and image srcs in the JSON bypass the HTML sanitiser for any consumer of the JSON (editor reload, B8 markdown export). B8 and C1 must treat it as untrusted.
11. `handlers/util.go:25-27`: 1 MiB cap applies to draft/publish bodies that carry both `content_json` and `content_html`, so effective document size is about 500 KB; autosave of a large table-heavy page returns 413. Give the draft/publish routes their own limit (for example 8 MiB).
12. `handlers/comments.go:129,237`, `spaces.go:76`, `labels.go:137`: oversize bodies return 400 `bad body` rather than 413 (pages.go handles it correctly via `pageDecode`).
13. `handlers/comments.go:151-165`: replies may nest to any depth (parent can itself be a reply), so `commentNode.finish` recursion and the JSON response are attacker-controlled in depth. Require `parent.ParentID == nil`.
14. `store/comments.go:184-198` (`GetCommentView`) loads and rebuilds the whole page tree to return one comment; `ListPageComments` is unbounded. Fine at wiki scale, but cap or paginate later.
15. `handlers/versions.go:29-36` and `handlers/pages.go:367` (`base_version`): integers above int32 reach pgx and fail with "greater than maximum value for int4" (500). Clamp in `pageVersionParam` (`n > math.MaxInt32` -> 400) and in `SaveDraft`.
16. `handlers/pages.go` `store.ErrNotTrashed` maps to 400; 409 `not_trashed` is more consistent with `page_trashed`.
17. `store/spaces.go:403-414`: `EXISTS (SELECT 1 FROM visible c WHERE c.parent_id = v.id)` over a twice-referenced CTE is O(n^2) for large spaces; use `v.id IN (SELECT parent_id FROM visible WHERE parent_id IS NOT NULL)` or `bool_or` via a grouped join.
18. `search/query.go:230-234`: `p.title ILIKE '%q%'` cannot use an index. Acceptable now; add `pg_trgm` + GIN on `title` if quick search gets slow. `count(*)` on every search page is also unbounded.
19. `store/labels.go:49-60`: the per-label correlated `count(*)` runs for every label matching the prefix before `LIMIT`; filter first or aggregate once.
20. `handlers/labels.go:36-50`: names restricted to ASCII `[a-z0-9-_.]`; non-English labels are rejected.
21. `handlers/attachments.go:197-203`: cleanup `Storage.Delete` uses `r.Context()`, which is cancelled if the client has already gone, leaving an orphan; use `context.WithoutCancel(r.Context())`. Empty files are rejected (`146-149`). `Cache-Control: private, max-age=3600` is also set on the `Content-Disposition: attachment` download endpoint (`319`), where the plan only asked for it on `/content`.
22. Unbounded list endpoints without paging: `ListSpaceTrash`, `ListSpaceLabelPages`, `ListPageVersions`, `ListPageAttachments`, `ListSpaceMembers`.
23. `cmd/server/main.go:122` (outside scope): only `ReadHeaderTimeout` is set; slow upload bodies hold a temp file and a goroutine indefinitely. Add a body read deadline via `http.NewResponseController(w).SetReadDeadline` in `Upload`.
24. `handlers/spaces.go:129` (member list): any viewer of an internal space sees every member's email. Matches the plan but worth a conscious yes.

## Verified correct (no action)

- Authorization matrix: spaces (list/create = auth; get/members/tree = view; patch/archive = manage_space; delete = delete_space; members write = manage_members; admin list = system admin), pages (create/patch/draft/publish/move/trash/restore/copy/editing = edit_pages; get/children/star = view; purge = purge_trash), versions (view; restore = edit), comments (view; create/patch/delete/resolve = comment, author-only edit, author-or-admin delete), labels (view; put = manage_labels), search (auth), attachments (upload/delete = manage_attachments; list/download/content = view). Archived and deleted space behaviour comes from `RequireSpacePerm`.
- Draft-status visibility is enforced in `pageLoad`, tree, children, comments, labels, attachments, quick search and versions. Same-space checks hold: `parent_id` (`pageCheckParent`), `after_id` (sibling list), comment `parent_id` (`parent.PageID != pageID`), attachment `space_id` taken from the page row, and each id-keyed route resolves the space from the entity itself, so no id can ride on another space's permission.
- SQL: all parameterised; `search.AccessClause` and `HeadlineOptions` are constants; `EscapeLike` escapes `\`, `%`, `_`; `SafeSnippet` escapes before substituting the per-process random markers.
- Sanitising: drafts, publish, version restore, template content, comments (create and update) all call `SanitizeHTML`; only copy does not (P3-7).
- Transactions: `CreateSpace`, `PublishPage` (page + version + draft delete under `FOR UPDATE`), `MovePage`, `TrashPage`, `RestorePage`, `CreatePage` (copy) each run in one tx with a per-space advisory lock, so positions cannot be duplicated by concurrent operations; events are published after commit in every handler. Last-admin uses `FOR UPDATE` on admin rows and re-evaluates correctly under READ COMMITTED. Cycle detection walks the new parent's ancestors and rejects self and descendants.
- Resource handling: multipart streamed with `MaxBytesReader(limit+1MiB)`, spooled once to a 0600 temp file removed by `defer`, passed to `Put` as an `*os.File` (no second spool), `Storage.Get` body closed, S3 object removed if the insert fails, filename sanitised, `Content-Disposition` built without header injection, `nosniff` plus sandbox CSP on non-PDF.
- Error envelope: all handler and middleware errors are `{"error": ...}` with optional `code` (`version_conflict`, `page_trashed`, `last_admin`).

## Contract mismatches (Go vs `web/lib/api/types.ts`)

| Type | Result |
|---|---|
| Space, AdminSpace | Match (`role` null on admin list, `member_count`, `page_count` always present) |
| SpaceMember | Match (`space_id`, `user_id`, `user`, `role`, `created_at`) |
| PageSummary, TrashedPage | Match |
| Page | Match (`space_key` added by handler; `content_text` correctly absent; `content_json` serialises `null`) |
| PageDetail | Shape matches, but `labels` is always `[]` (P2-5) and `watching` always `false` (B7 stub) |
| PageVersion, PageVersionSummary | Match (`author` added) |
| PageDraft | Match |
| Comment | Match (`author`, `replies` always an array; deleted comments with replies have `body_json: null`, `body_html: ""`) |
| Label | Match (`page_count` on list endpoints) |
| Attachment | Match (`uploader` added, `s3_key` hidden) |
| SearchResult, SearchHit, QuickSearchHit | Match |
| CreatePageInput | `parent_id: null` is treated as "home page", not root (P3-3) |
| Search `author` param | Filters on `pages.updated_by` (last editor), accepts a UUID only |

No required TS field is missing or renamed on the Go side.

## Verdict

The wave is in good shape: the authorization model is applied consistently through the middleware chain plus `pageLoad`, the SQL is parameterised and index-aware except for `pages(parent_id)`, transactions and event ordering are correct, and the web contract matches field for field apart from the stubbed `labels`/`watching`. Nothing blocks a commit on security grounds, but I would fix the P2 set first: close the trashed-page and never-published-draft exposure (P2-1 to P2-3) with one product decision, add the template space hook before B6 starts (P2-4), wire `ListPageLabels` into `GET /pages/{id}` (P2-5), clean up S3 on purge (P2-6), add the missing `parent_id` index migration (P2-7), stop the 403/404 existence oracle in the middleware (P2-8), and stop serving PDFs inline (P2-9). The P3 list is hardening that can be folded into the integration wave.
