# B3 comments: notes

## What was built

- `api/migrations/0030_comments.sql`: `comments` table exactly per the plan (`body_json` nullable jsonb, `inline_anchor` nullable jsonb, `parent_id` and `page_id` cascade, `author_id`/`resolved_by` FK users) with indexes `idx_comments_page(page_id, created_at)` and `idx_comments_parent(parent_id)`. Down drops the table; `TestMigrationsDownAndUp` still passes.
- `api/internal/store/comments.go`:
  - `CommentView{domain.Comment; Author *domain.UserRef "author"; Replies []CommentView "replies"}` is the API shape (flattened JSON, replies always `[]`, never null).
  - `CreateComment(ctx, domain.Comment) (domain.Comment, error)`, `GetComment`, `GetCommentWithPage(ctx, id) (domain.Comment, CommentPageInfo, error)`, `GetCommentPageInfo(ctx, pageID) (CommentPageInfo{PageID, SpaceID, Status, CreatedBy}, error)`.
  - `ListPageComments(ctx, pageID) ([]CommentView, error)`: one query joining `users` for authors, tree built in Go from `parent_id` (arbitrary depth), ordered `created_at, id`. Soft-deleted comments are kept only while they have a surviving descendant and are returned blanked (`body_html ""`, `body_json null`, `deleted_at` set).
  - `GetCommentView(ctx, id)` returns one comment with its (pruned) reply subtree. `UpdateCommentBody`, `SoftDeleteComment`, `ResolveComment(ctx, id, userID) (changed bool, err)`, `ReopenComment(ctx, id) (changed bool, err)`.
  - Exported for other agents: `GetCommentSpaceID(ctx, commentID) (uuid.UUID, error)` (`store.ErrNotFound` when missing) and `ListPageCommentCounts(ctx, pageIDs) (map[uuid.UUID]int, error)` (non-deleted comments, including resolved and replies; pages with none are absent from the map).
- `api/internal/http/handlers/comments.go`: `CommentHandler{Deps}` with flat routes
  - `GET /pages/{pageID}/comments` (SpaceByPage, PermViewSpace)
  - `POST /pages/{pageID}/comments` (SpaceByPage, PermComment) -> 201
  - `PATCH /comments/{commentID}`, `DELETE /comments/{commentID}` (204), `POST /comments/{commentID}/resolve|reopen` (all SpaceByResolver with a resolver over `GetCommentSpaceID`, PermComment).
- Tests: `store/comments_test.go` (tree order and nesting, author join, soft-delete shape, update/resolve/reopen, space id and counts, cascade) and `handlers/comments_test.go` (viewer can comment, private non-member 403 on all routes, implicit internal viewer, id/404 handling, validation, threading, author-only edit including system admin, delete permissions, soft-delete thread shape, resolve/reopen plus event, inline anchor and Inline flag, mentions in `CommentCreated` via `Bus.Subscribe` + `Bus.Wait`, drafts hidden, trashed page readable, sanitiser, archived space read-only). The handler tests build their own env (copy of `newEnv`) because `helpers_test.go` does not expose the Bus.

## Behaviour details and decisions

- Draft rule: `pages.status = 'draft'` is visible only to `created_by` or an effective admin (space admin or system admin). Enforced on list and create via the page row, and on patch/delete/resolve/reopen via the comment's page. Invisible means 404. Trashed pages are readable and still accept writes (not special-cased).
- Body validation: `body_html` goes through `content.SanitizeHTML`; the body must contain non-blank text in either the sanitised HTML (`ExtractTextFromHTML`) or `body_json` (`ExtractText`). `body_json` must be `null`/absent or a JSON object. The sanitised HTML is what is stored and returned.
- Inline anchor: only on top-level comments (400 on replies); `quoted_text` trimmed, non-empty, at most 2000 runes (400 otherwise); stored as `{"quoted_text": ...}` only.
- `parent_id` must exist, be on the same page and not be soft-deleted (400 `invalid parent comment`). Replies to replies are allowed.
- Edit: author only, 403 for everyone else including system admins; 404 for a soft-deleted comment. Delete: author or effective admin; idempotent 204 on an already deleted comment. Resolve/reopen: any commenter, top-level only (400 on replies, 404 on deleted). `CommentResolved` is published only on an actual transition.
- PATCH, resolve and reopen return the full `CommentView` including `replies`, so the web cache can swap the thread in place.
- Events are published after the DB write (single statements, no multi-statement transaction needed).

## Deviations

- `ListPageCommentCounts` returns `(map, error)` rather than a bare map, following the repo's error convention.
- Error responses use `httpErr` (`{"error": ...}` without `code`), as the existing handlers do.
- No event is published on edit or delete (none specified).

## Gate output

```
$ cd api && gofmt -l . && go build ./... && go vet ./... && go test -count=1 ./...
(gofmt clean)
ok  	github.com/hayward-solutions/knowledge/api/internal/auth
ok  	github.com/hayward-solutions/knowledge/api/internal/bootstrap
ok  	github.com/hayward-solutions/knowledge/api/internal/config
ok  	github.com/hayward-solutions/knowledge/api/internal/content
ok  	github.com/hayward-solutions/knowledge/api/internal/domain
ok  	github.com/hayward-solutions/knowledge/api/internal/events
ok  	github.com/hayward-solutions/knowledge/api/internal/http
ok  	github.com/hayward-solutions/knowledge/api/internal/http/handlers
ok  	github.com/hayward-solutions/knowledge/api/internal/http/middleware
ok  	github.com/hayward-solutions/knowledge/api/internal/search
ok  	github.com/hayward-solutions/knowledge/api/internal/storage
ok  	github.com/hayward-solutions/knowledge/api/internal/store
```

Last full run: everything above passes except `TestAttachmentFilenameSanitising` in `handlers/attachments_test.go` (B5's file, in progress when I finished). All comment tests (`go test ./internal/http/handlers -run Comment`, `go test ./internal/store`) pass.

## Remediation round

- Trashed pages: POST, PATCH, DELETE, resolve and reopen return 409 `{"error":"page is trashed","code":"page_trashed"}` for effective editors and admins. Reading comments of a trashed page (and any write attempt) requires effective role editor or admin; viewers get 404 (`commentPageVisible` now switches on page status: draft = creator or admin, trashed = editor or admin). The earlier "trashed pages are readable by everyone" and "trashed pages accept writes" notes are superseded.
- Reply depth: root is depth 0, a direct reply depth 1, reply to a reply depth 2; a reply whose depth would exceed 2 gets 400 `reply nesting too deep`. Depth comes from the new store method `GetCommentDepth(ctx, id)` (recursive CTE up the `parent_id` chain). Siblings at allowed levels are unaffected.
- Tests added or changed in `handlers/comments_test.go`: `TestCommentTrashedPage` (viewer 404 on read and writes, editor/space admin/system admin can read), `TestCommentWritesOnTrashedPageConflict` (409 body for all five writes), `TestCommentReplyNestingCap`. The private-space non-member test now expects 404, matching the shared middleware change made by someone else mid-task.
- Full gate rerun: gofmt clean, build, vet and `go test -count=1 ./...` all pass.
