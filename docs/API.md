# API Reference

All endpoints are mounted under `/api/v1`, except `GET /health`. Authentication is a session cookie (`sid`) set by `/auth/login` (or the OIDC callback); send it with `credentials: "include"`. Bodies and responses are JSON with snake_case fields, except where an endpoint says it streams bytes or multipart. Server timeouts and size limits are under [Limits and timeouts](#limits-and-timeouts).

All IDs are UUIDv7. Timestamps are RFC 3339 UTC. JSON request bodies are capped at 1 MiB. Response shapes are named after the TypeScript types in [`web/lib/api/types.ts`](../web/lib/api/types.ts) (`Space`, `PageDetail`, `Comment`, and so on); that file is the client contract.

This reference is written from the handlers' `Routes` functions in `api/internal/http/handlers/`. There are 81 routes under `/api/v1` plus `/health`.

---

## Access control

Each route declares one of these in the **Access** column.

| Access | Meaning |
|---|---|
| `public` | No session needed. |
| `auth` | `RequireAuth`: a valid session for an enabled user, else `401`. |
| `sysadmin` | `RequireAuth` then `RequireSystemAdmin`: `is_system_admin` must be true, else `403`. |
| `space:<perm>` | A space-scoped route. `SpaceByKey`, `SpaceByPage`, or `SpaceByResolver` runs `RequireAuth`, resolves the space and the caller's effective role, then `RequireSpacePerm(<perm>)`. |

The effective role in a space is: `admin` for a system admin; otherwise the caller's membership role; otherwise `viewer` when the space is `internal`; otherwise no access, which is reported as `404`, not `403`. See [CONFIGURATION.md](CONFIGURATION.md#roles-permissions-and-visibility) for the matrix and visibility rules.

Space permissions and the lowest role that holds each:

| Permission | Lowest role | Used for |
|---|---|---|
| `view_space` | viewer | Read pages, history, comments, labels, attachments, tree, members, activity, search. Star and export. |
| `comment` | viewer | Create comments and replies, resolve and reopen threads. |
| `edit_pages` | editor | Create, draft, publish, move, trash, restore, copy pages. View the trash. Restore a version. Import Markdown. Presence heartbeat. |
| `manage_attachments` | editor | Upload and delete attachments. |
| `manage_labels` | editor | Replace a page's labels. |
| `manage_templates` | editor | Create, edit, delete a space's templates. |
| `manage_members` | admin | Add, change, and remove space members. |
| `manage_space` | admin | Edit, archive, and unarchive a space. |
| `purge_trash` | admin | Permanently delete a trashed page. |
| `delete_space` | admin | Soft-delete a space. |

Rules on top of the matrix:

- **Archived spaces** are read-only for everyone except space and system admins. Any permission other than `view_space` answers `403 {"error":"space is read-only"}` for an editor or viewer, comments included. Starring, watching, and exporting still work.
- **Soft-deleted spaces** answer `404` to everyone except system admins.
- **Private spaces** answer `404` to everyone who is not a member or a system admin, for the space and for every page, comment, attachment, template, and watch target inside it. The response is the same as for an id that does not exist.
- **Draft pages** (never published) answer `404` to everyone except the creator and space admins, on every page, version, comment, label, attachment, watch, export, and activity route.
- **Trashed pages** answer `404` to viewers on every one of those routes; editors and admins can read them. A never-published page that was trashed stays visible only to its creator and space admins.

---

## Errors

Errors are `{"error": "message"}`. A `code` field is added only to the cases below; clients should branch on `status` plus `code`, not on the message text.

| Status | `code` | When |
|---|---|---|
| 400 | | Bad or missing field (`bad body`, `invalid id`, validation messages). |
| 401 | | No valid session (`unauthenticated`), or wrong credentials on login (`invalid credentials`, `account disabled`). |
| 403 | | Signed in but the role lacks the permission (`forbidden`), the space is archived (`space is read-only`), or an edit or delete is limited to the author. |
| 404 | | Not found **or not visible**. A private space you are not a member of, and every page, comment, attachment, and template inside it, answer `404`, indistinguishable from a missing id. |
| 405 | | Method not allowed for the path. Unknown paths answer `404 {"error":"not found"}`. |
| 409 | `version_conflict` | `POST /pages/{id}/publish` with a stale `base_version`. The body also has `current_version`. |
| 409 | `page_trashed` | A write to a page that is in the trash: publish, draft, patch, move, copy, trash, presence, labels, comments, attachments. Restore it first. |
| 409 | `last_admin` | Removing or demoting the last `admin` member of a space (`PUT` or `DELETE` on `/spaces/{key}/members/{userID}`). |
| 409 | | Other conflicts, without a code: email already in use, space key already in use, disabled user added to a space, and removing or disabling the last active system administrator (`cannot remove the last active system administrator`). |
| 413 | | Request too large. Attachment upload: `file too large` (cap is `ATTACHMENT_MAX_BYTES`, default 25 MiB). Markdown import: `file too large` (a file over 2 MiB or `ATTACHMENT_MAX_BYTES` if smaller) or `upload too large` (body over 32 MiB); see [Import limits](#import-limits). JSON bodies over 1 MiB on pages and templates: `request body too large` (other handlers answer `400 bad body`). |
| 429 | | Auth rate limit hit on `/auth/login` or `/auth/change-password`: per client IP (burst 10, refilling 10 per minute) or per account email (burst 5, refilling 5 per minute). `Retry-After: 60`. See [Rate limits](#rate-limits). |
| 503 | | `object storage not configured`: `STORAGE_S3_BUCKET` is unset. Affects attachment upload, download, and inline content only. |

Middleware errors use the same envelope, so a `401` from a missing cookie is `{"error":"unauthenticated"}`.

---

## Auth

| Method | Path | Access | Body / Notes |
|---|---|---|---|
| POST | `/auth/login` | public | `{email, password}`. Sets the `sid` cookie and returns `User`. Rate-limited per IP and per email (every attempt, successful or not, uses a token of the email's bucket). SSO-only accounts (no local password) answer `401 invalid credentials`. |
| POST | `/auth/logout` | optional session | Deletes the session and clears the cookie. Always `204`. |
| GET | `/auth/me` | auth | Returns `User`. |
| PATCH | `/auth/me` | auth | `{email?, display_name?}`. Returns `User`. `409` if the email is taken. Display name 1-200 characters. |
| POST | `/auth/change-password` | auth | `{current_password, new_password}`. `204`. Rate-limited per IP and per the caller's email. On success **every other session of the user is deleted**; the session that made the call stays signed in. `401` if the current password is wrong; `400` for a weak new password or an SSO-only account. |
| GET | `/auth/oidc/config` | public | Returns `OIDCConfig`: `{enabled: false}` or `{enabled: true, provider_name}`. |
| GET | `/auth/oidc/login` | public | `?next=/relative/path`. Redirects `302` to the IdP. `404` when OIDC is disabled. `next` is kept only if it is a plain relative path: it must start with a single `/`, be at most 2048 bytes, and contain no control characters, backslashes, scheme, host, or userinfo; otherwise it is ignored and the user lands on `/`. |
| GET | `/auth/oidc/callback` | public | Completes login, sets `sid`, and redirects to `PUBLIC_BASE_URL` plus `next` (default `/`). Failures redirect to `/login?error=<reason>`. When the IdP's email is not verified and would have to be linked to an existing local account, it answers `403` with an HTML "Sign-in blocked" page instead (see `OIDC_REQUIRE_EMAIL_VERIFIED` in [CONFIGURATION.md](CONFIGURATION.md#how-accounts-are-provisioned)). |

Password policy (everywhere a password is set): at least 12 characters, with an upper-case letter, a lower-case letter, a digit, and a symbol; at most 256 bytes.

## Users

| Method | Path | Access | Notes |
|---|---|---|---|
| GET | `/users` | auth | `?q=&limit=`. Active users whose display name or email contains `q` (case-insensitive), by display name. `limit` 1-50, default 20. Returns `UserRef[]`. Powers mention suggestions and member pickers. |
| GET | `/users/{userID}` | auth | Returns `UserRef`. `404` if unknown. |

## Admin (server-wide, system admin only)

| Method | Path | Access | Body / Notes |
|---|---|---|---|
| GET | `/admin/users` | sysadmin | Returns `User[]`, including disabled users. |
| POST | `/admin/users` | sysadmin | `{email, password, display_name?, is_system_admin?}`. `201` `User`. Display name defaults to the email. `409` if the email exists. |
| PATCH | `/admin/users/{userID}` | sysadmin | `{email?, display_name?, password?, is_system_admin?, disabled?}`. Returns `User`. Setting `password` also revokes all of that user's sessions. `409` (no code) if it would leave no active system admin. |
| GET | `/admin/spaces` | sysadmin | Returns `AdminSpace[]`: every space including archived and soft-deleted, sorted by key, with `role` null. |
| POST | `/admin/templates` | sysadmin | Creates a global template. See [Templates](#templates). |

## Spaces

| Method | Path | Access | Body / Notes |
|---|---|---|---|
| GET | `/spaces` | auth | Spaces the caller can see (members, plus all `internal` spaces; system admins see every non-deleted space), archived included, sorted by name. Returns `Space[]` with `role`, `member_count`, `page_count`. |
| POST | `/spaces` | auth | `CreateSpaceInput`: `{key, name, description?, icon?, visibility?}`. Any signed-in user may create a space and becomes its `admin`. `201` `Space`. Also creates the published home page and sets `home_page_id`. |
| GET | `/spaces/{key}` | space:view_space | Returns `Space`; `role` is the caller's effective role. The key is case-insensitive. |
| PATCH | `/spaces/{key}` | space:manage_space | `UpdateSpaceInput`, all optional: `{name?, description?, icon?, visibility?, home_page_id?}`. `icon: null` and `home_page_id: null` clear. Omitted keys are untouched. The key cannot change. Returns `Space`. |
| DELETE | `/spaces/{key}` | space:delete_space | Soft delete. `204`. The key stays reserved. |
| POST | `/spaces/{key}/archive` | space:manage_space | Idempotent. Returns `Space`. |
| POST | `/spaces/{key}/unarchive` | space:manage_space | Idempotent. Returns `Space`. |
| GET | `/spaces/{key}/members` | space:view_space | Returns `SpaceMember[]` sorted by display name. Includes member emails. |
| PUT | `/spaces/{key}/members/{userID}` | space:manage_members | `{role}` (`admin`, `editor`, `viewer`). Adds or updates; always `200` `SpaceMember`. `404` unknown user, `409` disabled user, `409 last_admin`. A new member is notified (`space.added`). |
| DELETE | `/spaces/{key}/members/{userID}` | space:manage_members | `204`. `404` if not a member, `409 last_admin`. |
| GET | `/spaces/{key}/tree` | space:view_space | Flat `PageSummary[]` of every non-trashed page the caller may see, ordered by parent then position. Drafts appear only for their creator and admins. The client builds the tree from `parent_id`. |
| GET | `/spaces/{key}/trash` | space:edit_pages | Returns `TrashedPage[]`, newest first. Never-published trashed pages are listed only for their creator and admins. |

Key rules: 2 to 10 characters, starting with a letter, then letters or digits; input is upper-cased. `name` 1-200 characters, `description` up to 2000, `icon` up to 64 (an empty string stores null). `visibility` is `internal` (default) or `private`. `home_page_id` must be a published page in the same space, else `400`.

## Pages

| Method | Path | Access | Body / Notes |
|---|---|---|---|
| POST | `/spaces/{key}/pages` | space:edit_pages | `CreatePageInput`: `{title?, parent_id?, template_id?}`. Creates a page with `status: "draft"` and `version: 0`, under `parent_id` or, when omitted or null, under the space home page. An empty title becomes the template's name, else `Untitled`. `201` `Page`. A `template_id` from another space (or unknown) is `404 template not found`; global templates work everywhere. |
| GET | `/pages/{pageID}` | space:view_space | Returns `PageDetail`: the page plus `labels`, `breadcrumbs`, `children`, `my_draft`, `editors` (others currently editing), `starred`, `watching`, `my_role`, and `created_by_user` / `updated_by_user`. Records a view for the dashboard. |
| PATCH | `/pages/{pageID}` | space:edit_pages | `{title}`. Renames without creating a version and regenerates the slug. Returns `Page`. |
| DELETE | `/pages/{pageID}` | space:purge_trash | Permanently deletes a **trashed** page, its descendants, and their attachment objects. `204`. `400 page is not trashed` otherwise. |
| PUT | `/pages/{pageID}/draft` | space:edit_pages | `DraftInput`: `{title, content_json, content_html, base_version}`. Upserts the caller's own draft (one per user per page). Returns `PageDraft`. |
| GET | `/pages/{pageID}/draft` | space:edit_pages | Returns the caller's `PageDraft`, or `404` if there is none. |
| DELETE | `/pages/{pageID}/draft` | space:edit_pages | Discards the caller's draft. `204`. |
| POST | `/pages/{pageID}/publish` | space:edit_pages | `PublishInput`: `{title, content_json, content_html, base_version, message?, force?}`. Returns `Page`. See [Publishing](#publishing-and-conflicts). |
| POST | `/pages/{pageID}/move` | space:edit_pages | `MovePageInput`: `{parent_id, after_id?, position?}`. `parent_id: null` moves to the space root. `after_id` (a sibling under the new parent) wins over `position`; with neither the page is appended. Returns `Page`. `400` for a parent outside the space, a trashed parent, a cycle, a non-sibling `after_id`, a move that would make the tree deeper than 64 levels (`page tree too deep`), or the space home page (`the space home page cannot be trashed or moved`). |
| POST | `/pages/{pageID}/trash` | space:edit_pages | Trashes the page and every descendant. `204`. `400` if the space home page is the page or anywhere in its subtree (`the space home page cannot be trashed or moved`). |
| POST | `/pages/{pageID}/restore` | space:edit_pages | Restores the page and the descendants trashed with it. Returns `Page`. If the old parent is trashed it goes under the home page. Pages never published return as `draft`. `400` if not trashed or if restoring would exceed the 64-level depth cap. |
| POST | `/pages/{pageID}/copy` | space:edit_pages | Optional body `{title?, parent_id?}`. Copies the content into a new draft-status page in the same space, titled `Copy of <title>` by default. `201` `Page`. `400 page tree too deep` past 64 levels. |
| GET | `/pages/{pageID}/children` | space:view_space | Returns `PageSummary[]` in position order. |
| POST | `/pages/{pageID}/star` | space:view_space | `204`. |
| DELETE | `/pages/{pageID}/star` | space:view_space | `204`. |
| POST | `/pages/{pageID}/editing` | space:edit_pages | Presence heartbeat; the editor sends it every 15 seconds. Returns `EditingState` (`{editors: UserRef[]}`) excluding the caller. Entries expire after 30 seconds. |

Notes:

- `content_json` is a ProseMirror document (an object) or `null`; any other JSON type is `400`. `content_html` is sanitised on write (see [DEVELOPMENT.md](DEVELOPMENT.md#content-contract)); `content_text` for search is derived server-side and never returned.
- Titles are limited to 500 characters. `base_version` must be between 0 and 2147483647, else `400 invalid base_version`. Request bodies over 1 MiB answer `413`.
- Slugs are regenerated from the title on publish and on rename. A title that would slug to `edit`, `history`, or `attachments` gets `-page` appended, because those are route segments under `/pages/{id}/`. The web app treats the slug segment in `/spaces/KEY/pages/ID/slug` as cosmetic.
- Every write on a page in the trash answers `409 page_trashed`, except restore and purge.

### Publishing and conflicts

`POST /pages/{pageID}/publish` bumps `version` by one, writes a `page_versions` row, deletes the caller's draft, and notifies watchers. A never-published page has `version: 0`, so its first publish sends `base_version: 0`.

If `base_version` differs from the page's current `version`, and `force` is not true, the response is:

```json
{
  "error": "page was updated by someone else",
  "code": "version_conflict",
  "current_version": 4
}
```

(`VersionConflict` in `types.ts`.) The editor offers to reload the latest content or to resend with `"force": true`. The message is optional, trimmed, and limited to 1000 characters; `title` is required.

## Versions

| Method | Path | Access | Notes |
|---|---|---|---|
| GET | `/pages/{pageID}/versions` | space:view_space | Returns `PageVersionSummary[]` (no content), newest first, each with `author`. |
| GET | `/pages/{pageID}/versions/{version}` | space:view_space | Returns `PageVersion` with content. `400` for a version outside 1 to 2147483647; `404` if absent. |
| POST | `/pages/{pageID}/versions/{version}/restore` | space:edit_pages | Publishes that version's title and content as a new version with the message `Restored version <n>`, ignoring conflicts. Returns `Page`. |

## Comments

| Method | Path | Access | Body / Notes |
|---|---|---|---|
| GET | `/pages/{pageID}/comments` | space:view_space | Returns `Comment[]`: top-level threads with nested `replies`, oldest first, inline and page comments together. A deleted comment that still has replies is kept with an empty body and `deleted_at` set. |
| POST | `/pages/{pageID}/comments` | space:comment | `CommentInput`: `{body_json, body_html, parent_id?, inline_anchor?}`. `201` `Comment`. The body must contain text. `inline_anchor` is `{quoted_text, prefix?, suffix?}` and is allowed on top-level comments only: `quoted_text` is 1 to 2000 characters, and `prefix` and `suffix` are up to 64 characters each of the text before and after the quote, used to pick the right occurrence when the quote repeats (empty values are not stored). The returned `id` is what the editor stores in the `comment` mark. |
| PATCH | `/comments/{commentID}` | space:comment | `{body_json, body_html}`. Author only (`403` for anyone else, system admins included). Returns `Comment`. |
| DELETE | `/comments/{commentID}` | space:comment | Soft delete. The author or a space admin; `403` otherwise. Idempotent `204`. |
| POST | `/comments/{commentID}/resolve` | space:comment | Top-level comments only (`400` for a reply). Returns `Comment`. |
| POST | `/comments/{commentID}/reopen` | space:comment | Top-level comments only. Returns `Comment`. |

Replies nest at most two levels below a top-level comment (`400 reply nesting too deep`). `parent_id` must be a live comment on the same page. `body_html` is sanitised on write.

## Labels

| Method | Path | Access | Body / Notes |
|---|---|---|---|
| GET | `/labels` | auth | `?q=&limit=`. Label names starting with `q`, only those on at least one page the caller can see, with `page_count`. `limit` 1-50, default 20. Returns `Label[]`. |
| GET | `/pages/{pageID}/labels` | space:view_space | Returns `Label[]` sorted by name. |
| PUT | `/pages/{pageID}/labels` | space:manage_labels | `{labels: string[]}` replaces the whole set; `[]` clears it. Returns `Label[]`. `400` if `labels` is missing, a name is invalid, or there are more than 20. |
| GET | `/spaces/{key}/labels` | space:view_space | Returns `Label[]` with `page_count` for the space. |
| GET | `/spaces/{key}/labels/{name}/pages` | space:view_space | Returns `PageSummary[]` ordered by title. An unknown or invalid name returns `[]`. |

Names are lower-cased, whitespace runs become `-`, and the result must match `[a-z0-9._-]{1,50}`. Duplicates collapse after normalising.

## Search

| Method | Path | Access | Notes |
|---|---|---|---|
| GET | `/search` | auth | `?q=&space=&label=&author=&page=&limit=`. Returns `SearchResult`: `{hits, total, page, limit}`. See [Search syntax](#search-syntax). |
| GET | `/search/quick` | auth | `?q=&limit=`. Title substring match for the quick-switcher and the page-link picker. Returns `QuickSearchHit[]`. |

Both search only spaces the caller can see (archived included, soft-deleted excluded). Offset pagination: `page` 1-10000 and `limit` 1-50 (default 20) on `/search`; `limit` 1-20 (default 8) on `/search/quick`.

## Attachments

Uploads are proxied: the browser sends multipart to the API, which streams the file to S3, and downloads stream back through the API. There are no presigned URLs.

| Method | Path | Access | Body / Notes |
|---|---|---|---|
| POST | `/pages/{pageID}/attachments` | space:manage_attachments | `multipart/form-data` with a `file` part. `201` `Attachment` (with `uploader`). Stored under `spaces/{spaceID}/pages/{pageID}/{attachmentID}/{filename}`. |
| GET | `/pages/{pageID}/attachments` | space:view_space | Returns `Attachment[]`, newest first. Works even when storage is not configured. |
| GET | `/attachments/{attachmentID}` | space:view_space | Streams the bytes with `Content-Disposition: attachment`. |
| GET | `/attachments/{attachmentID}/content` | space:view_space | Streams the bytes; `inline` for `image/png`, `image/jpeg`, `image/gif`, `image/webp`, and `image/avif` only, `attachment` for everything else. This is the URL embedded in `<img src>`. |
| DELETE | `/attachments/{attachmentID}` | space:manage_attachments | Soft-deletes the row and deletes the object. `204`. |

Upload rules and errors:

- The file is capped at `ATTACHMENT_MAX_BYTES` (default 26214400); larger is `413 file too large`. An empty file, a missing `file` part, or a non-multipart body is `400`.
- `Content-Type` is sniffed from the first 512 bytes; the declared type is used only when the sniff is `application/octet-stream`. Filenames are sanitised (path parts, control characters, and bidi controls removed; at most 255 bytes).
- Responses carry `X-Content-Type-Options: nosniff`, `Cache-Control: private, max-age=3600`, and `Content-Security-Policy: default-src 'none'; sandbox`.
- `503 object storage not configured` on upload, download, and content when `STORAGE_S3_BUCKET` is unset. `404` if the row or the object is gone.
- Uploading to or deleting from a trashed page is `409 page_trashed`. Draft pages follow the visibility rule above, so only the creator and space admins can upload to or list a draft page's attachments.

## Templates

| Method | Path | Access | Body / Notes |
|---|---|---|---|
| GET | `/templates` | auth | Global templates plus those of spaces where the caller is editor or admin (system admins: all non-deleted spaces). Returns `Template[]`, globals first. The four built-in starters (Meeting notes, Decision record, How-to guide, Retrospective) are created at API start, and again when a system admin calls this endpoint, whenever the `templates` table is empty. |
| GET | `/spaces/{key}/templates` | space:view_space | Returns that space's `Template[]` by name. |
| POST | `/spaces/{key}/templates` | space:manage_templates | `{name, description?, content_json?, content_html?}`. `201` `Template`. |
| POST | `/admin/templates` | sysadmin | Same body, creates a global template (`space_id` null). |
| GET | `/templates/{templateID}` | auth, checked in handler | Returns `Template`. Global: any signed-in user. Space template: anyone who can view the space, else `404`. |
| PATCH | `/templates/{templateID}` | auth, checked in handler | Partial `{name?, description?, content_json?, content_html?}`. `content_json: null` clears. Returns `Template`. Global templates need a system admin; space templates need `manage_templates` in that space (`403` otherwise, `space is read-only` when archived). |
| DELETE | `/templates/{templateID}` | auth, checked in handler | `204`. Same rules as PATCH. |

`name` is 1-120 characters, `description` up to 2000, `content_json` an object or null, `content_html` is sanitised. Create a page from a template with `template_id` on `POST /spaces/{key}/pages`.

## Watch

| Method | Path | Access | Notes |
|---|---|---|---|
| GET | `/watch/{type}/{id}` | auth | `type` is `page` or `space`; `id` is the page or space UUID (not the key). Returns `WatchState` `{watching}`. |
| PUT | `/watch/{type}/{id}` | auth | Starts watching. Returns `{watching: true}`. |
| DELETE | `/watch/{type}/{id}` | auth | Stops watching. Returns `{watching: false}`. |

The caller must be able to see the target; otherwise `404` for all three. A bad `type` or `id` is `400`. Publishing a page you created for the first time, or commenting on a page, watches that page for you automatically.

## Notifications

| Method | Path | Access | Notes |
|---|---|---|---|
| GET | `/notifications` | auth | `?unread=&limit=&before=`. Returns `Notification[]`, newest first. `unread=true` for unread only, `unread=false` for read only, omitted for all. Header `X-Unread-Count` carries the total unread. See [Cursor pagination](#cursor-pagination). |
| POST | `/notifications/read` | auth | `{ids: string[]}` (at most 500) or `{all: true}`. `204`. `400` if neither is given. |
| GET | `/notifications/stream` | auth | Server-Sent Events. See [Notification stream](#notification-stream). |

Notification `type` values: `page.published`, `page.mentioned`, `comment.created`, `comment.replied`, `comment.mentioned`, `comment.resolved`, `space.added`. Every `payload` has `actor_name`; most also have `page_title`, `page_slug`, and `space_key`. A user is notified at most once per event and never for their own actions. Recipients must be active and able to access the space; for a draft page only its creator and space admins are notified. `page.mentioned` fires only for users newly mentioned by that publish (mentions already in the previous version do not repeat), and not again while the user still has an unread `page.mentioned` for the same page.

## Activity and dashboard

| Method | Path | Access | Notes |
|---|---|---|---|
| GET | `/spaces/{key}/activity` | space:view_space | `?before=&limit=`. Returns `Activity[]`, newest first. `type` is `page.published`, `page.moved`, `page.trashed`, `page.restored`, or `comment.created`. Rows about a page are returned only while the page still exists and is visible to the caller: draft pages and never-published trashed pages only to their creator and space admins, other trashed pages only to editors and admins, and rows about purged pages never. See [Cursor pagination](#cursor-pagination). |
| GET | `/me/dashboard` | auth | Returns `Dashboard`: `recent_updates` (20), `recently_viewed` (10), `my_drafts` (20), and `starred` (20). Each item is a `DashboardPage`. Sections are never null. |

## Export and import

| Method | Path | Access | Notes |
|---|---|---|---|
| GET | `/pages/{pageID}/export` | space:view_space | `?format=md` (default) or `html`. A file download named after the slug. `400` for any other format. The HTML is a standalone document with inline CSS, served with a sandboxing CSP. |
| GET | `/spaces/{key}/export` | space:view_space | `application/zip` named `<KEY>-export.zip`: the published pages as Markdown, a page with children becoming `<slug>/index.md` plus its children, and a `README.md` link list. Drafts and trashed pages are excluded. |
| POST | `/spaces/{key}/import` | space:edit_pages | `multipart/form-data` with one or more `files` parts and an optional `parent_id` field (default: the space home page). Returns `201` `ImportResult` `{pages: PageSummary[]}`. |

Import behaviour: only `.md` and `.markdown` files are accepted, and any other file rejects the whole request with `400`. The filename in each part may carry a relative path (`guides/setup.md`); folders become empty parent pages, and an `index.md` in a folder becomes that folder page's content. Absolute paths and `..` segments are `400`. All pages are created published at version 1 in a single transaction, and an import that would make any page deeper than 64 levels is refused before anything is created (`400 page tree is too deep`). Raw HTML in Markdown is dropped.

#### Import limits

Each limit is checked before conversion and answers `400` unless noted.

| Limit | Value |
|---|---|
| Files per request | 100 |
| Pages created per request (files, minus `index.md` files folded into folders, plus folder pages) | 500 |
| Directory depth in a filename | 10 segments |
| One directory segment | 120 characters |
| One filename | 255 characters |
| Whole path | 4096 bytes |
| One file | 2 MiB, or `ATTACHMENT_MAX_BYTES` if that is smaller (`413`) |
| Request body | 32 MiB plus 1 MiB of multipart framing (`413`) |
| Lines per file | 20000 (`400 file too complex: <name>`) |
| Nested blockquote or list markers on one line | 32 (`400 file too complex: <name>`) |
| Conversion time per file | 5 seconds (`400 file too complex: <name>`) |
| Concurrent conversions | 4 per API process; when all slots are busy the request answers `503 import is busy, try again` |
| Page tree depth | 64 levels (`400 page tree is too deep`) |

A file that goldmark cannot convert answers `400 unsupported markdown structure: <name>`.

## Health

| Method | Path | Access | Notes |
|---|---|---|---|
| GET | `/health` | public | Outside `/api/v1`. Returns `{"ok": true, "version": "<VERSION build arg>"}`. Used by the container healthcheck and the Kubernetes liveness probe. |

---

## Limits and timeouts

| Limit | Value |
|---|---|
| JSON request body | 1 MiB |
| Request headers | 64 KiB |
| Reading request headers | 10 seconds |
| Reading a whole request | 60 seconds |
| Reading `POST .../attachments` and `POST .../import` | 5 minutes, which is also the deadline on the request's context, so a slower upload is cut off |
| Writing a response | no limit, so downloads and the SSE stream are not cut |
| Idle keep-alive connection | 120 seconds |
| Database statement | 30 seconds (`statement_timeout` on every pooled connection); a longer query is cancelled and answers `500` |
| Page tree depth | 64 levels, enforced on create, copy, move, restore, and import (`400 page tree too deep`) |
| Reply depth | two levels below a top-level comment |

### Rate limits

Credential endpoints use two token buckets, both kept in memory per API process:

- **Per client IP** on `POST /auth/login` and `POST /auth/change-password`: burst 10, refilling 10 per minute.
- **Per email** (lower-cased; the caller's own email for change-password): burst 5, refilling 5 per minute. It is checked before any password hashing. Every attempt uses a token, whether or not the password was right, so five attempts in a short window lock that email out until tokens refill (one every 12 seconds), **including the legitimate owner**. That is the accepted trade-off against password guessing; it also means a correct password can answer `429` while the bucket is empty.

Both answer `429 too many requests` with `Retry-After: 60`. Each bucket table holds at most 10,000 keys (least recently used are evicted), and no more than four password hashes run at once, so parallel logins queue.

The client IP is the TCP peer address unless `TRUSTED_PROXIES` is set. When the peer is inside that list, the IP is the right-most `X-Forwarded-For` entry that is not itself trusted (falling back to `X-Real-IP` when there is no `X-Forwarded-For`). With `TRUSTED_PROXIES` empty these headers are ignored, so behind a proxy every client shares the proxy's bucket. See [CONFIGURATION.md](CONFIGURATION.md#cookies-and-transport-security).

## Cursor pagination

`GET /notifications` and `GET /spaces/{key}/activity` page backwards in time with a cursor:

1. Request without `before` to get the newest `limit` items (`limit` 1-100, default 30).
2. Take `created_at` of the **last** item returned and pass it as `before` (RFC 3339, URL-encoded) to get the next, strictly older, page.
3. A page shorter than `limit` is the last one.

An unparsable `before` is `400 invalid before`. Search results use `page` and `limit` instead (offset pagination, with `total` in the response).

## Notification stream

`GET /notifications/stream` is a long-lived `text/event-stream` response authenticated by the `sid` cookie; the web client opens one per tab. Cross-origin clients must construct `EventSource` with `withCredentials: true`. Each frame is `event: <name>` followed by `data: <JSON>`:

```
event: hello
data: {"unread_count":1}

event: notification
data: {"id":"...","type":"page.published","actor":{...},"space_key":"ENG","page_id":"...","page_title":"Runbook","payload":{...},"read_at":null,"created_at":"..."}

event: unread_count
data: {"count":2}

: ping
```

| Event | Data | Sent |
|---|---|---|
| `hello` | `{"unread_count": N}` | Once, first. |
| `notification` | A full `Notification`, the same shape as the list endpoint. | When a new notification is created for the user. |
| `unread_count` | `{"count": N}` | After every new notification and after `POST /notifications/read`. |
| `: ping` | SSE comment, no event. | Every 30 seconds. At each ping the server re-validates the session and ends the stream if it has expired or the user is disabled. |

The response sets `Cache-Control: no-cache` and `X-Accel-Buffering: no`. The hub is in memory and per process, and each subscriber has a 16-event buffer; events that arrive while it is full are dropped, so clients should refetch the list after reconnecting. The web client reconnects with exponential backoff from 1 to 30 seconds.

## Search syntax

`GET /search?q=` runs Postgres `websearch_to_tsquery('english', q)` against page titles (weight A) and body text (weight B), and ranks with `ts_rank_cd`, then most recently updated.

| Query | Meaning |
|---|---|
| `deploy rollback` | Both words (AND), after English stemming (`deploying` matches `deploy`). |
| `"rollback plan"` | The exact phrase. |
| `deploy or rollback` | Either word. |
| `deploy -staging` | `deploy` but not `staging`. |

Common stop words and punctuation are ignored; a query that reduces to nothing returns zero hits rather than an error. `q` is required (`400 q is required`) and limited to 200 characters.

Filters, all optional and combined with AND: `space` is a space key (case-insensitive), `label` is a label name, `author` is a user ID and matches the page's **last editor**, not its creator (an invalid UUID is `400`). Only **published** pages are searched; drafts and trashed pages are not.

Each hit is `{page: PageSummary, space_key, snippet, updated_at, author}`. The `snippet` is HTML-safe: the body text is entity-escaped, and the only markup is `<mark>` and `</mark>` around matches, with ` … ` between fragments. It can be rendered with `dangerouslySetInnerHTML`.

`GET /search/quick?q=` is not full-text: it matches `q` as a case-insensitive substring of the title (LIKE wildcards in `q` are escaped), lists prefix matches first, then the most recently updated. An empty `q` returns the most recently updated pages. It includes the caller's own draft pages (`status: "draft"`) and never other people's.
