# C4 notifications, templates, admin, export UI notes

Gate (clean): `cd web && npm run lint && npm run typecheck && npm run build`. New build routes: `/admin`, `/admin/labels`, `/admin/spaces`, `/admin/templates`, `/notifications`, `/templates`, `/spaces/[key]/activity`, `/spaces/[key]/settings/import-export`, `/spaces/[key]/settings/templates`. No dependencies added. Not exercised at runtime: the notify, watch, activity and export Go endpoints did not exist yet (empty `Routes`), so everything was coded against the PLAN contract and compiled only.

## Components

- `components/notifications/NotificationBell.tsx` (overwritten, same export). Badge from `["unread"]` (`getUnreadCount`), live via `useEventStream(NOTIFICATIONS_STREAM_PATH, {unread_count, notification})`. `unread_count` data may be a number or an object with `count`/`unread_count`/`unread`/`total`. A `notification` event invalidates `["notifications"]` and `["unread"]` and shows an info toast (text from the event if it looks like a Notification, else a generic message). Dropdown lists the latest 8 (`["notifications","latest"]`), Mark all read, link to `/notifications`; full-width sheet under the top bar on mobile.
- `NotificationItem.tsx` (row, used by bell and page; exports `relativeTime`, `notificationSummary`), `format.ts` (type verbs, href builder, defensive payload readers), `useSpaceKeys.ts` (space id to key via the shared `["spaces"]` query, used when a notification lacks `space_key`).
- `WatchToggle.tsx`: `{type, id, initial?, size?, showLabel?, className?}`. Query key `["watch", type, id]`, optimistic PUT/DELETE, toast on change. `showLabel` defaults to true for `type="space"` ("Watch space" / "Watching") and false for pages (icon only, eye when watching, eye-off when not).
- `components/templates/`: `TemplatePicker({spaceKey, value, onChange, disabled?, id?, className?})` (select with Blank page, this space's templates, global templates; also exports `useSpaceTemplateOptions`), `TemplateEditor` (name, description, full `Editor`; no image upload because there is no page to attach to), `TemplateManager({scope: {kind:"global"}|{kind:"space",key}, canManage})` (list, preview, create, edit, delete), `TemplatePreviewModal` (ContentView), `UseTemplateDialog({template, onClose, defaultSpaceKey?})` (space with edit role, parent page from the tree, title; `createPage` with `template_id`, then navigates to `/spaces/KEY/pages/ID/edit`), `pageOptions.ts` (`flattenPages`, `indentLabel`), `index.ts`.
- `components/activity/ActivityItem.tsx`: rows per type (`page.published` with `vN` and message, `page.moved`, `page.trashed` linking to the trash, `page.restored`, `comment.created`, plus generic fallback).
- `components/importexport/`: `ExportSpaceButton({spaceKey, className?, label?})` (anchor to `spaceExportUrl`), `ImportDialog({spaceKey, onClose, onImported?})` (files and folder via `webkitdirectory` set in a ref callback, non-markdown skipped with a notice, parent page select, result list of created pages). `components/admin/` has no files; the admin pages are self-contained.

## Routes

- `/notifications`: All/Unread tabs, `useInfiniteQuery(["notifications","list",filter])`, Load more, click marks read, Mark all read; rows link to the page and `#comment-<id>`.
- `/templates`: gallery grouped Global then per space, Preview modal, Use template, Manage links (`/admin/templates` for system admins, `/spaces/KEY/settings/templates` for spaces).
- `/spaces/[key]/settings/templates`: `TemplateManager` for the space; `canManage` for space admin/editor or system admin. Fetches its own space (`["templates","space-meta",key]`), no `SpaceContext` dependency.
- `/spaces/[key]/activity`: day-grouped feed, `useInfiniteQuery(["activity", key])`, Load more.
- `/spaces/[key]/settings/import-export`: Export and Import cards (import disabled for viewers and archived spaces). **The settings tabs owned by the spaces agent should link here** (label "Import and export").
- `/admin` redirects to `/admin/users`. `/admin/spaces` (key link, name, visibility, members, pages, status, created; filter), `/admin/labels` (read-only, sorted by page count, links to `/search?label=`; rename and merge are out of scope), `/admin/templates` (global `TemplateManager`). `admin/layout.tsx` and `admin/users/page.tsx` untouched; the layout's four tab links were already correct.
- `/spaces/[key]/*` pages do not wrap themselves in `AppShell`; they rely on the spaces layout. `/notifications` and `/templates` wrap themselves.

## Query keys

`["notifications"]` (prefix; `["notifications","latest"]`, `["notifications","list",filter]`), `["unread"]`, `["watch",type,id]`, `["templates"]` (gallery; also `["templates","global"]`, `["templates","space",key]`, `["templates","space-meta",key]`, `["templates","tree",key]`), `["activity",key]`, `["admin","spaces"]`, `["admin","labels"]`, `["importexport","space",key]`, `["import","tree",key]`. `["spaces"]` is shared with the Sidebar.

## lib/api changes (additive)

- `notifications.ts`: `listNotifications({unread?, before?, limit?})`, new `getUnreadCount()` (reads header `X-Unread-Count` from `GET /notifications?unread=true&limit=1`, falls back to the array length), `ListNotificationsParams`.
- `export.ts`: `importMarkdown` now sends each file under `webkitRelativePath || name` so folder structure reaches the server.

## Assumptions the integration agent must reconcile

- Cursor: `before` is the `created_at` of the last item on the previous page (RFC3339 string as received), `limit=30` on notifications and activity. If the Go side expects the item id (UUIDv7) instead, change `getNextPageParam` in `app/notifications/page.tsx` and `app/spaces/[key]/activity/page.tsx`. A page shorter than the limit ends pagination; duplicate ids are de-duplicated client side.
- Notification/activity display data is read from top-level `actor`, `page_title`, `space_key` when present, otherwise from `payload`: `actor_name`/`actor_display_name`, `page_title`/`title`, `space_key`, `page_slug`, `comment_excerpt`/`excerpt`/`snippet`, and for activity `version`, `message`, `comment_id`. When the page slug is unknown the link uses `slugify(title)`; the page route should ignore the slug segment.
- `unread_count` and `notification` SSE data shapes: see the bell above.
- Multipart import: Go's `FileHeader.Filename` is base-named by `mime/multipart`, so nested folder paths sent as the filename are lost unless the B8 handler reads the raw `Content-Disposition` filename. Flagged for D1/B8.
- `GET /templates` is expected to include `space_key` (the Go `TemplateView` does); `TemplatePicker` also queries `GET /spaces/{key}/templates` and merges.
- Create-from-template is wired only in `UseTemplateDialog`; the new-page dialog of the pages agent can embed `TemplatePicker` from `@/components/templates`.
