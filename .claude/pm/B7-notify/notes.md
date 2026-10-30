# B7 notify: notes

## What was built

- `api/migrations/0070_notify.sql`: `watches`, `notifications`, `activity` (Up and Down). No FKs from notifications/activity to pages or comments; `activity.space_id` cascades with the space, user FKs as specified.
- `api/internal/notify/notify.go`: `notify.New(store, bus, hub).Subscribe()`; one bus handler type-switching on the seven domain events. `NotificationHandler.Subscribe()` (called by `NewRouter`) constructs and subscribes it.
- `api/internal/store/{watches,notifications,activity,dashboard}.go` plus tests.
- `handlers/{watch,notifications,activity,dashboard}.go` plus tests (package `handlers_test`). One surgical edit in `handlers/pages.go`: `pageWatching` now returns `Store.IsWatching(user, page)` (direct page watch only; a space or ancestor watch does not set it).

## Routes (flat, under /api/v1)

| Route | Middleware | Result |
|---|---|---|
| `GET/PUT/DELETE /watch/{type}/{id}` | RequireAuth + in-handler access check | `{"watching": bool}` (200 for all three) |
| `GET /notifications?unread=&limit=&before=` | RequireAuth | `Notification[]`, header `X-Unread-Count` |
| `POST /notifications/read` `{ids}` or `{all:true}` | RequireAuth | 204 (400 when neither given, bad uuid, or over 500 ids) |
| `GET /notifications/stream` | RequireAuth | SSE |
| `GET /spaces/{key}/activity?before=&limit=` | SpaceByKey(PermViewSpace) | `Activity[]` |
| `GET /me/dashboard` | RequireAuth | `Dashboard` |

Watch access: type must be `page|space` (400), id a uuid (400). Page: effective role resolved like the middleware (system admin -> admin, member, internal -> viewer, else 404), deleted space 404 unless system admin, then `store.IsPageVisible` (drafts creator/admin only; trashed pages need editor). Space: same role logic. Failing any check is 404 for GET, PUT and DELETE.

List semantics: `unread=true` unread only, `unread=false` read only, absent = all; anything else 400. `limit` 1-100 default 30. `before` is an RFC3339 `created_at` cursor (strictly older; ties broken by id internally). Activity uses the same cursor and limit rules. Timestamps are emitted in UTC.

`X-Unread-Count` is also listed in `Access-Control-Expose-Headers` by the handler (the global CORS middleware does not expose it; same-origin deployments do not need it).

## SSE format

`Content-Type: text/event-stream`. Every event's `data:` is the JSON payload only (the event name is the kind):

- `event: hello` data `{"unread_count": N}` (sent first)
- `event: notification` data = a full Notification (same shape as the list, including `actor`, `space_key`, `page_title`)
- `event: unread_count` data `{"count": N}` (sent after every new notification and after `POST /notifications/read`)
- `: ping` comment every 30 s; at each ping the session is re-validated (`SessionTokenFrom` + `GetValidSession`, and the user must still be enabled) and the stream ends when invalid. `NotificationHandler.Heartbeat` (zero = 30 s) exists so tests can shorten it.

## Notification types and payload fields

Top-level Notification fields: `id, user_id, type, actor_id, actor (UserRef), space_id, space_key, page_id, page_title, comment_id, payload, read_at, created_at`. `page_title` comes from `payload.page_title`; `space_key` from the live space row.

Every payload has `actor_name`. Types:

| type | recipients | payload keys (besides `actor_name`) |
|---|---|---|
| `page.published` | watchers of the page, any ancestor, or the space | `page_title, page_slug, space_key, version, message?` (`version == 1` means a new page) |
| `page.mentioned` | `MentionedUserIDs` of a publish | same as above |
| `comment.created` | watchers of the page | `page_title, page_slug, space_key, comment_excerpt, inline` |
| `comment.replied` | parent comment author and every other participant (root and reply authors) of the thread | same |
| `comment.mentioned` | mentioned users in a comment | same |
| `comment.resolved` | author of the resolved root comment | same (excerpt of the root comment) |
| `space.added` | the user added to a space | `space_key, space_name, role` (no page) |

`comment_excerpt` is the first 140 runes of the comment text, whitespace collapsed. Link target for comments: `#comment-<comment_id>`.

Priority per user per event (one notification each, never the actor): mention > reply (parent, then other participants) > watch. Recipients are also filtered through `Store.FilterNotifiable`: user active, space not deleted, and the user is a system admin, member, or the space is internal; for a draft page only the creator, space admins and system admins (so watchers cannot learn about draft comments). `space.added` is not sent when the actor adds themself.

Auto-watch: the commenter watches the page on `CommentCreated`; the actor watches the page on `PagePublished` when `version == 1` and they created the page.

## Activity

Rows: `page.published {title, page_title, version, message?}`, `page.moved {title, page_title, old_parent_id, new_parent_id}`, `page.trashed {title, page_title}`, `page.restored {title, page_title}`, `comment.created {title, page_title, comment_id, excerpt, comment_excerpt, inline, reply}`. Response adds `actor`, `page_title` (from payload), `space_key`. Rows whose page is currently a draft are hidden from everyone but its creator and space admins; rows for purged pages remain. Comment resolve, space membership changes and watcher-only events do not write activity.

## Dashboard

`recent_updates` (20, published pages by `updated_at`), `recently_viewed` (10 by `viewed_at`), `my_drafts` (20; pages with status draft created by the caller, plus pages with a `page_drafts` row for the caller; `updated_at` is the draft row's when present), `starred` (20 by star time). All exclude trashed pages, hidden drafts and inaccessible or deleted spaces (stale views/stars are filtered out too). Each item is PageSummary fields + `space_key, space_name, updated_at, author` (author = `updated_by`; the caller for `my_drafts`). Sections are never null.

## Notes for other agents

- Events are handled asynchronously (one goroutine per bus handler); tests must call `Bus.Wait()`. Notification handling runs nested publishes (`NotificationCreated`) inside the handler goroutine, so `Wait()` covers them.
- Because `NewRouter` subscribes the notify service, every handler test that publishes events now also writes notification/activity rows. Late writes after a test's DB is dropped only log errors.
- Watches are not cleaned up when a page is purged (no FK, `target_id` is polymorphic); they are harmless but accumulate. A purge-time `DELETE FROM watches WHERE target_type = 'page' AND target_id = ...` could be added by the pages owner.
- No dependency changes, no comments, no git.

## Gate

```
$ cd api && gofmt -l . && go build ./... && go vet ./... && go test -count=1 ./...
```
All packages ok (handlers, notify, store included, real Postgres testcontainer). `gofmt -l` lists only `internal/content/markdown_test.go`, another agent's in-progress export file; none of mine. `go test -race` passes on notify and the handler tests.
