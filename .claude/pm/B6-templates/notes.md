# B6 templates: notes

## Files
`api/migrations/0060_templates.sql`, `api/internal/store/{templates,template_content}.go`, `api/internal/store/templates_test.go`, `api/internal/http/handlers/{templates,templates_test}.go`. No go.mod/go.sum change, no comments in code, no git.

## Migration 0060
`templates(id, space_id null fk spaces on delete cascade, name, description default '', content_json jsonb null, content_html default '', created_by fk users, created_at, updated_at)` plus `idx_templates_space_name(space_id, name)`. Up and Down. The migration seeds nothing: `created_by` is NOT NULL and no user exists when migrations run on a fresh database.

## Starter seeding (lazy)
`GET /templates` called by a system admin runs `Store.SeedTemplatesIfEmpty(ctx, adminID, starters)`. It inserts the four global starters (Meeting notes, Decision record, How-to guide, Retrospective, `created_by` = that admin) only when the whole `templates` table is empty. It does a cheap count first, then re-checks inside a transaction holding `pg_advisory_xact_lock(6006)`, so concurrent first calls seed once. Non-admins never seed, so until an admin has opened the templates list (the admin console's templates page or `/templates`) starters do not exist. Caveat: if an admin deletes every template, the next admin `GET /templates` re-seeds the starters (no settings table to remember the deletion).

Starters are built in Go from a tiny node tree (`templateNode` in handlers/templates.go) that renders both the ProseMirror JSON and the matching HTML, so they cannot drift. They use headings, panels (info/note/warning/success), ordered/bullet lists, task lists, tables (`<table><tbody><tr><th colspan="1" rowspan="1">`) and a toc. HTML passes through `content.SanitizeHTML`; a test asserts it is stable under the sanitiser.

## Endpoints (all under /api/v1; JSON matches web `Template`, `space_key` null for global)
| Route | Guard | Result |
|---|---|---|
| `GET /templates` | RequireAuth | global + templates of spaces where caller is editor/admin member; system admin: all non-deleted spaces. Sorted globals first, then spaces, each by `lower(name)` |
| `GET /spaces/{key}/templates` | SpaceByKey(view) | that space's templates by name (viewers allowed) |
| `POST /spaces/{key}/templates` | SpaceByKey(manage_templates) | 201 Template |
| `POST /admin/templates` | RequireAuth + RequireSystemAdmin | 201 global Template |
| `GET /templates/{templateID}` | RequireAuth + in-handler | Template (global: any signed-in user; space: anyone who can view the space, else 404) |
| `PATCH /templates/{templateID}` | RequireAuth + in-handler | Template |
| `DELETE /templates/{templateID}` | RequireAuth + in-handler | 204 |

In-handler authorisation (`templateLoad` / `templateLoadForManage`): global requires system admin to modify (403 otherwise); space template: role computed like the middleware (system admin -> admin, member role, internal -> viewer, else 404; soft-deleted space 404 for non-system-admins), then `RoleAllows(role, PermManageTemplates)` else 403, and archived/deleted space gives 403 `space is read-only` unless role is admin.

Validation: name trimmed, 1-120 runes; description trimmed, max 2000 runes (defaults to ''); `content_json` must be a JSON object or null; `content_html` is sanitised with `content.SanitizeHTML`; body limit 1 MiB (413). PATCH is partial: absent fields are untouched, `content_json: null` clears it, `updated_at` is bumped. Create from a space route ignores any `space_id` in the body.

## Page creation
`Store.GetTemplateContent(id)` returns `SpaceID` (uuid.Nil for global), name as `Title`, `ContentJSON` (nil when null) and `ContentHTML`. `PageHandler.Create` already 404s a template whose space differs from the target space; tests cover cross-space 404, same-space and global templates, title copied from the template name when the request title is blank (explicit title wins), content copied.

## Judgement calls
- Soft-deleted spaces are excluded from `GET /templates` even for system admins.
- A viewer can read a space template via `GET /templates/{id}` but it does not appear in their `GET /templates` (editor/admin spaces only, per brief).
- Store helpers are prefixed `Template*`; handler helpers `template*`.

## Gate
`cd api && gofmt -l . && go build ./... && go vet ./... && go test -count=1 ./...`: gofmt clean, everything ok against the real Postgres testcontainer (handlers 21s, store 6.5s).
