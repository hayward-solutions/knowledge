# B1 spaces: notes

## What was built

- `api/internal/store/spaces.go`: all SQL for spaces, members, tree and trash.
- `api/internal/store/spaces_test.go`: store tests against the shared testcontainers Postgres.
- `api/internal/http/handlers/spaces.go`: `SpaceHandler` (keeps `type SpaceHandler struct{ Deps }` and `Routes`). Also registers `GET /admin/spaces`.
- `api/internal/http/handlers/spaces_test.go`: handler tests (`TestSpaces*`) through the full router. Helpers are prefixed `sp`.
- No migration 0010: the existing indexes (`idx_pages_space_status`, `idx_pages_tree`, `idx_space_members_user`) cover every query.

## Routes (all flat, under /api/v1)

| Route | Middleware | Notes |
|---|---|---|
| `GET /spaces` | RequireAuth | accessible, non-deleted, archived included, sorted by lower(name), `role` is the effective role |
| `POST /spaces` | RequireAuth | 201, creator becomes admin member, home page created in the same tx |
| `GET /spaces/{key}` | SpaceByKey ViewSpace | role from `SpaceCtx` |
| `PATCH /spaces/{key}` | ManageSpace | partial; `icon: null` clears, `home_page_id: null` clears; omitted keys are untouched |
| `DELETE /spaces/{key}` | DeleteSpace | soft delete, 204 |
| `POST /spaces/{key}/archive`, `/unarchive` | ManageSpace | idempotent, return Space |
| `GET /spaces/{key}/members` | ViewSpace | sorted by display name |
| `PUT /spaces/{key}/members/{userID}` | ManageMembers | 200 SpaceMember (also on insert), 404 unknown user, 409 disabled user, 409 `last_admin` |
| `DELETE /spaces/{key}/members/{userID}` | ManageMembers | 204, 404 not a member, 409 `last_admin` |
| `GET /spaces/{key}/tree` | ViewSpace | flat list |
| `GET /spaces/{key}/trash` | ViewSpace | newest first |
| `GET /admin/spaces` | RequireAuth + RequireSystemAdmin | every space incl. deleted and archived, sorted by key, `role` is null |

`/spaces/{key}/activity` belongs to B7 and is not registered here.

## Exported store API others can use

- `store.SlugFromTitle(title string) string`: lowercases, collapses every run of non letter/digit (Unicode aware) into one `-`, trims `-`, caps at 80 runes, returns `"untitled"` for an empty result. The pages agent should use this for page slugs.
- `store.ListSpacesAccessible(ctx, userID, isSystemAdmin) ([]SpaceView, error)`: same query as `GET /spaces`; useful for dashboard and search permission scoping if they want ids (use `SpaceView.ID`).
- `store.GetSpaceView(ctx, spaceID) (SpaceView, error)`: space + member_count + page_count (Role nil).
- `store.SpaceView` embeds `domain.Space` and adds `Role *domain.Role`, `MemberCount`, `PageCount` with the JSON names the web `Space` and `AdminSpace` types expect.
- `store.ListAdminSpaces`, `CreateSpace(ctx, CreateSpaceParams)`, `UpdateSpace(ctx, id, SpaceUpdate)`, `SoftDeleteSpace`, `SetSpaceArchived`.
- Members: `ListSpaceMembers`, `GetSpaceMemberView`, `UpsertSpaceMember(ctx, spaceID, userID, role) (SpaceMemberView, inserted bool, error)`, `RemoveSpaceMember`. Both mutators lock the space's admin rows with `FOR UPDATE` and return `store.ErrLastAdmin` when the target is the only `role = 'admin'` member and would lose it.
- `store.ListSpaceTree(ctx, spaceID, viewerID, viewerIsAdmin) ([]SpacePageSummary, error)` and `ListSpaceTrash(ctx, spaceID) ([]SpaceTrashedPage, error)`.
- `store.ErrInvalidHomePage` (new sentinel).
- Type names are prefixed (`SpacePageSummary`, `SpaceTrashedPage`) so B2 can define its own `PageSummary` in `store/pages.go` without a clash.

## Behaviour decisions and deviations

1. `ErrLastAdmin` is reused as the brief asked, but its message is about system administrators, so the handler does not echo it. The 409 body is `{"error":"a space must keep at least one admin member","code":"last_admin"}`.
2. Re-asserting `admin` on the only admin is allowed (no-op). Removing or demoting a non-admin never trips the guard, so a space that somehow has zero admin members (fixtures) can still have members changed.
3. Soft-deleted spaces keep their key reserved (creating the same key again is 409) because `spaces.key` is globally unique. Mutating store calls on a deleted space return `ErrNotFound`; the middleware already hides deleted spaces from everyone except system admins.
4. Tree: published pages plus `draft` pages that the caller created, or all drafts when the effective role is admin (space admin or system admin). `has_children` is computed in the same query against the same visible set. Order is `parent_id NULLS FIRST, position, title, id`, so the root level (including the home page) comes first and sibling groups follow in parent-uuid order; the client builds the tree.
5. Trash `has_children` means "has trashed children". `trashed_by_user` is null when `trashed_by` is null.
6. `page_count` counts `status = 'published'` only (drafts and trashed excluded). `member_count` counts all `space_members` rows (disabled users included).
7. Validation limits: name 1-200 runes, description up to 2000 runes, icon up to 64 runes (empty string stored as null). Invalid `home_page_id` (other space, trashed, missing) is 400.
8. `PUT members` returns 200 for both insert and update. `SpaceMemberAdded` is published only on insert, after the transaction commits.
9. The "archived space blocks editor writes" requirement is enforced by A1's `RequireSpacePerm`. Within the spaces routes every write needs space admin, so the archive test asserts editor PATCH/PUT/DELETE/unarchive are 403 and viewers can still read (tree/get), while the admin can patch and unarchive. Page-level editor writes on archived spaces belong to B2's tests.
10. Error bodies from this handler are `{"error": ...}` (plus `code` only for `last_admin`), consistent with the existing `httpErr` helper.
11. Audit log lines (`audit space.create`, `audit space.delete`) follow the pattern in `admin.go`.

## Gate output

See the final section appended after the last run.

```
$ cd api && gofmt -l . && go build ./... && go vet ./... && go test -count=1 ./...
(gofmt: no output)
ok  	.../internal/auth, bootstrap, config, content, domain, events, http, http/handlers, http/middleware, search, storage, store
?   	.../cmd/server, internal/deps, internal/testutil	[no test files]
```
Integration tests ran against a real postgres:16-alpine testcontainer (no skips). One earlier run failed in another agent's in-progress `TestAttachmentFilenameSanitising`; it passed on retry without any change from this task.

## Remediation round 1

- `GET /spaces/{key}/trash` now uses `PermEditPages` (viewers and implicit internal-space viewers get 403).
- `store.ListSpaceTrash(ctx, spaceID, viewerID, viewerIsAdmin)`: trashed pages with `version = 0` (never published) are listed only for their creator or when the caller's effective role is admin; previously published trashed pages are listed for everyone who can open the trash. The handler passes `sc.Role == admin`.
- Tests: store test covers other user / admin / creator and the version flip; `TestSpacesTrash` covers viewer and outsider 403, anon 401, other editor (only published-then-trashed), creator (own never-published too), space admin and system admin (everything), newest-first order.
- The shared middleware now answers 404 (not 403) for non-members of a private space; `TestSpacesPrivateVisibility` was updated to expect 404 for outsiders, and `/trash` was dropped from that loop since it is covered in `TestSpacesTrash`.
- Gate re-run: `gofmt -l .` empty, `go build ./...`, `go vet ./...`, `go test -count=1 ./...` all pass (an intermediate run briefly failed in other agents' in-progress pages/comments/labels tests and passed on retry with no change from this task).
