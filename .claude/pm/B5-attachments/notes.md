# B5 attachments: notes

## What was built

- `api/migrations/0050_attachments.sql`: `attachments(id, page_id -> pages cascade, space_id -> spaces cascade, uploader_id -> users, filename, content_type, size bigint >= 0, s3_key, created_at, deleted_at)` with `idx_attachments_page(page_id, created_at)`. Up and Down.
- `api/internal/store/attachments.go`: SQL with `$n` params.
- `api/internal/http/handlers/attachments.go`: `AttachmentHandler{Deps}` and `Routes` (flat `r.With(...)` chains).
- Tests: `store/attachments_test.go` (testutil.NewDB) and `handlers/attachments_test.go` (package `handlers_test`, map-backed fake `storage.ObjectStore`).

## Endpoints (under /api/v1)

| Route | Middleware | Behaviour |
|---|---|---|
| `POST /pages/{pageID}/attachments` | `SpaceByPage(PermManageAttachments)` | multipart, field `file`; 201 `Attachment` with `uploader` |
| `GET /pages/{pageID}/attachments` | `SpaceByPage(PermViewSpace)` | `Attachment[]`, newest first, `[]` when empty |
| `GET /attachments/{attachmentID}` | `SpaceByResolver(GetAttachmentSpaceID, PermViewSpace)` | streams bytes, always `Content-Disposition: attachment` |
| `GET /attachments/{attachmentID}/content` | same | inline only for png, jpeg, gif, webp, avif and pdf; everything else (including svg, html, text) is `attachment` |
| `DELETE /attachments/{attachmentID}` | `SpaceByResolver(..., PermManageAttachments)` | soft delete row, then delete object (failure is logged, still 204) |

Response JSON matches `web/lib/api/types.ts` `Attachment`: `id, page_id, space_id, uploader_id, uploader{id,display_name,email}, filename, content_type, size, created_at, deleted_at` (null). `s3_key` is never serialised.

## Upload details

- `h.Storage == nil` -> 503 `{"error":"object storage not configured"}`, checked first inside the handler (after the permission middleware).
- Limit is `Config.AttachmentMaxBytes` (0 or unset falls back to 25 MiB). The request body is wrapped in `http.MaxBytesReader(limit + 1 MiB)` to cover multipart framing, the file itself is capped at exactly `limit` bytes. Anything over -> 413; exactly `limit` bytes is accepted.
- The multipart body is read with `MultipartReader` (no in-memory form parsing). The `file` part is spooled to a temp file under `os.TempDir()` to learn the size, then `Put(ctx, key, contentType, tempFile, size)`; the temp file is removed afterwards. Empty file, missing `file` field, and non-multipart bodies -> 400.
- Content type: `http.DetectContentType` on the first 512 bytes, parameters stripped (`text/plain; charset=utf-8` is stored as `text/plain`). Only when the sniff result is `application/octet-stream` is the part's declared type used (if it parses as a media type and is more specific). So a file named `x.png` declared `image/png` but containing HTML is stored as `text/html`.
- Filename sanitising: invalid UTF-8 dropped, backslashes treated as separators, `path.Base`, control characters and Unicode format characters (e.g. bidi override U+202E) stripped, trimmed, `""`/`.`/`..`/dots-only -> `file`, shortened to 255 bytes on a rune boundary while keeping the extension (up to 32 bytes).
- Key: `spaces/{spaceID}/pages/{pageID}/{attachmentID}/{filename}`. If the DB insert fails the object is deleted (best effort, logged).
- Page checks: trashed page -> 400 `page is trashed`; draft page -> only the page creator or a space admin (system admins are admin) may upload.

## Download details

Headers on both read endpoints: `Content-Type` (stored type), `Content-Length`, `Content-Disposition: <inline|attachment>; filename="<ascii fallback>"; filename*=UTF-8''<RFC 5987 encoded>`, `X-Content-Type-Options: nosniff`, `Cache-Control: private, max-age=3600`. The ASCII fallback replaces non-ASCII, quotes, backslashes, `%` and control characters with `_`. Missing row (or soft-deleted) -> 404, missing object (`storage.ErrNotFound`) -> 404, no storage configured -> 503.

## Exported store helpers

```
store.AttachmentWithUploader{domain.Attachment; Uploader domain.UserRef `json:"uploader"`}
store.AttachmentPageMeta{SpaceID uuid.UUID; Status domain.PageStatus; CreatedBy uuid.UUID}
CreateAttachment(ctx, *domain.Attachment) error                     // fills CreatedAt
GetAttachment(ctx, id) (domain.Attachment, error)                   // ErrNotFound when deleted
GetAttachmentSpaceID(ctx, id) (uuid.UUID, error)                    // resolver; ErrNotFound when deleted
GetAttachmentPageMeta(ctx, pageID) (AttachmentPageMeta, error)
ListPageAttachments(ctx, pageID) ([]AttachmentWithUploader, error) // newest first, never nil
CountPageAttachments(ctx, pageID) (int, error)                      // for page views / export
SoftDeleteAttachment(ctx, id) (domain.Attachment, error)            // returns the row incl. S3Key; ErrNotFound if already deleted
```

## Deviations and judgement calls

1. Draft visibility is a 404 (not 403) for non-creators on list, upload, download, content and delete, matching the "404 otherwise" rule for list so draft existence is not leaked. It also covers `GET /attachments/{id}[/content]` and `DELETE` for attachments on someone else's draft page, which the brief did not spell out.
2. `GetAttachmentSpaceID` filters out soft-deleted rows, so a deleted attachment 404s at the middleware and a second `DELETE` is a 404.
3. `helpers_test.go`'s `newEnv(t, mutate)` only mutates `Config` and builds `Deps` without `Storage`, so the mutate hook cannot inject a store. The test file defines `newAttachmentEnv(t, mutate, storage.ObjectStore)` which builds the same router with `Deps.Storage` set and returns the shared `*env`, so the existing `client`, `do`, `mustLogin` helpers are reused. `helpers_test.go` is untouched. Other test files in the package can reuse `fakeObjectStore`.
4. Extra defence in depth not in the brief: `Content-Security-Policy: default-src 'none'; sandbox` on every attachment response except `application/pdf` (a sandbox CSP breaks the browser PDF viewer). Images in `<img>` are unaffected.
5. The `Attachment` response type is `store.AttachmentWithUploader` (embedding `domain.Attachment`), since `domain.Attachment` has no `Uploader` field and `domain/` is not mine to edit.
6. Not handled: objects are not removed from storage when a page or space is hard-deleted (the DB rows cascade). The purge path in the pages/spaces work could list attachments with `ListPageAttachments`-style queries first if it wants to clean up; there is no `ListSpaceAttachments` helper yet.
7. Soft-deleted attachment rows are never purged from the table; only the object is deleted.

## Gate output

```
$ cd api && gofmt -l . && go build ./... && go vet ./... && go test -count=1 ./...
(gofmt: no output)
?   	github.com/hayward-solutions/knowledge/api/cmd/server	[no test files]
ok  	github.com/hayward-solutions/knowledge/api/internal/auth	0.349s
ok  	github.com/hayward-solutions/knowledge/api/internal/bootstrap	1.515s
ok  	github.com/hayward-solutions/knowledge/api/internal/config	0.104s
ok  	github.com/hayward-solutions/knowledge/api/internal/content	0.114s
?   	github.com/hayward-solutions/knowledge/api/internal/deps	[no test files]
ok  	github.com/hayward-solutions/knowledge/api/internal/domain	0.108s
ok  	github.com/hayward-solutions/knowledge/api/internal/events	0.106s
ok  	github.com/hayward-solutions/knowledge/api/internal/http	0.136s
ok  	github.com/hayward-solutions/knowledge/api/internal/http/handlers	11.662s
ok  	github.com/hayward-solutions/knowledge/api/internal/http/middleware	2.016s
ok  	github.com/hayward-solutions/knowledge/api/internal/search	0.068s
ok  	github.com/hayward-solutions/knowledge/api/internal/storage	0.144s
ok  	github.com/hayward-solutions/knowledge/api/internal/store	4.161s
?   	github.com/hayward-solutions/knowledge/api/internal/testutil	[no test files]
```
Tests ran against a real postgres testcontainer (no skips). The other agents' packages were building at the time of the final run.

## Remediation round 1

1. PDF is no longer inline. The inline allowlist is png, jpeg, gif, webp, avif only. Every other type (pdf, svg, html, text, ...) gets `Content-Disposition: attachment`. The `Content-Security-Policy: default-src 'none'; sandbox` header is now set on every attachment response (the PDF exception is gone). Deviation 4 above is superseded.
2. Writes on a trashed page return 409 `{"error":"page is trashed","code":"page_trashed"}` for upload and for `DELETE /attachments/{id}` (previously upload returned 400). The visibility check runs first, so a non-visible page still reads as 404.
3. Read visibility: draft pages stay creator-or-admin; pages with status `trashed` are readable (list, download, content) only with effective role editor or admin, viewers get 404. Deviation 1 above still holds for drafts.
4. Added `Store.ListAttachmentObjectKeysUnderPage(ctx, pageID) ([]string, error)`: recursive CTE over `pages.parent_id` from the given page, includes trashed descendants, returns `s3_key` for non-deleted attachments ordered by `created_at, id`, never nil. Added `attachmentPurgeObjects(ctx, st storage.ObjectStore, keys []string)` in `handlers/attachments.go` (package-private, usable by pages.go in the same package): deletes each key, logs failures, ignores a nil store. It has no direct test because `handlers_test` is an external package; it is a trivial loop over `ObjectStore.Delete`. Intended use: call the store helper before the purge (rows cascade away), call the purge func after.
5. Test changes: pdf now asserts attachment; `TestAttachmentTrashedPage` covers 409 on upload and delete, editor and admin reads, viewer 404; `TestListAttachmentObjectKeysUnderPage` covers root, child, trashed grandchild, soft-deleted and sibling exclusion, unknown page.
6. The space middleware now answers 404 (not 403) to non-members of private spaces. The permission test accepts 403 or 404 for those denials (`attachmentDenied`) so it does not depend on which the middleware settles on.

### Gate (after remediation)
```
?   	github.com/hayward-solutions/knowledge/api/cmd/server	[no test files]
ok  	github.com/hayward-solutions/knowledge/api/internal/auth	0.476s
ok  	github.com/hayward-solutions/knowledge/api/internal/bootstrap	2.059s
ok  	github.com/hayward-solutions/knowledge/api/internal/config	0.066s
ok  	github.com/hayward-solutions/knowledge/api/internal/content	0.079s
?   	github.com/hayward-solutions/knowledge/api/internal/deps	[no test files]
ok  	github.com/hayward-solutions/knowledge/api/internal/domain	0.070s
ok  	github.com/hayward-solutions/knowledge/api/internal/events	0.066s
ok  	github.com/hayward-solutions/knowledge/api/internal/http	0.092s
ok  	github.com/hayward-solutions/knowledge/api/internal/http/handlers	21.074s
ok  	github.com/hayward-solutions/knowledge/api/internal/http/middleware	2.424s
ok  	github.com/hayward-solutions/knowledge/api/internal/search	0.061s
ok  	github.com/hayward-solutions/knowledge/api/internal/storage	0.134s
ok  	github.com/hayward-solutions/knowledge/api/internal/store	7.429s
?   	github.com/hayward-solutions/knowledge/api/internal/testutil	[no test files]
```

## Remediation round 2: cycle-safe subtree walk

`ListAttachmentObjectKeysUnderPage` now walks `tree(id, depth)` with `UNION` (not `UNION ALL`) and `WHERE t.depth < 1000`, then selects keys with `a.page_id IN (SELECT id FROM tree)`. Because the depth column is part of the row, a parent cycle is not deduplicated by `UNION` alone; the depth guard is what bounds it (at most 1000 iterations), and the `IN` subquery collapses duplicate page ids so each key is returned once. New test `TestListAttachmentObjectKeysUnderPageSurvivesCycle` builds an A <-> B parent cycle with raw SQL, runs under a 10 s context, and asserts both keys come back once and promptly.

Gate: all attachment tests, store tests and every package of mine pass. Other agents' in-progress work was failing at the time of the last full run (`TestImportHardLimits` in the export work; earlier a transient run had OIDC callback tests and a gofmt complaint in `store_test.go` that cleared on retry); none touch attachment code.
