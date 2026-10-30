# Final web review

Scope: `web/app`, `web/components`, `web/lib` (not `e2e`). Read in full against `docs/PLAN.md` and the A2, C1, C2, C3, C4 and D1 notes. No files were edited and no git commands were run.

How it was checked: static read of every file in scope; Go handlers read for every route the web calls; `npm run lint` and `npm run typecheck` (both clean); the production build of the compose stack (`docker compose up -d --build --wait`, default ports were free, `.env` created from `.env.example` and removed afterwards, stack removed with `down -v`); then driven in the built-in browser as admin and as a viewer, at desktop and 375px, with crafted data created through the API. "Observed" below means reproduced in that run. "Reasoned" means found by reading code only.

## Verdict

Ship-ready for the stated v1 scope, with one fix I would make first. Every core flow I exercised works (login, spaces, page tree, view, edit and autosave, publish, copy, history and restore, trash, comments, labels, search, viewer gating, admin gating). Server-sanitised HTML is the only thing rendered through `dangerouslySetInnerHTML` and I found no XSS. The defects are one open redirect on login (P1), then a cluster of UX, resilience and accessibility gaps (P2). There is no contract drift that breaks anything.

Counts: P1 1, P2 8, P3 22.

## P1

### P1-1 Open redirect through `next` on the login page

- Files: `web/lib/auth.tsx:22-27` (`safeNext`), used at `web/app/login/page.tsx:19` and `:41` (`router.push(next)`).
- Issue: `safeNext` rejects `//` and `/\` but accepts `/` followed by a tab or newline. The URL parser strips tab and newline, so `/\t/evil.example.com` becomes `//evil.example.com` and `router.push` leaves the origin.
- Reproduction (observed): open `/login?next=%2F%09%2Fevil.example.com%2Fx`, sign in with valid credentials. The tab navigated to `evil.example.com`. A phishing link that looks like a Knowledge login URL is enough.
- Fix: parse and compare origin instead of pattern matching.
  ```ts
  export function safeNext(raw, fallback = "/") {
    if (!raw || /[\u0000-\u001f\u007f\\]/.test(raw) || !raw.startsWith("/") || raw.startsWith("//")) return fallback;
    try {
      const u = new URL(raw, window.location.origin);
      return u.origin === window.location.origin ? `${u.pathname}${u.search}${u.hash}` : fallback;
    } catch { return fallback; }
  }
  ```
  `safeNext` also feeds `oidcLoginUrl(next)`; the API's `next` handling on `/auth/oidc/login` was not part of this web review and should get the same rule.

## P2

### P2-1 No global 401 handling after the first `me` call

- Files: `web/lib/api/client.ts:42-56`, `web/lib/auth.tsx:13-20` (`me` is fresh for 60 s) and `:41-47`.
- Issue: only `useMe` redirects on 401. When the session expires or is revoked mid-use, every other query or mutation shows its own error.
- Reproduction (observed): sign in, call `POST /auth/logout` from the console, click a space from `/spaces`. Result: "Could not load this space / unauthenticated / Try again" and no redirect. The notification stream also retried 401 repeatedly (about a dozen 401s in the console).
- Fix: in `api()`/`apiUpload()` on 401 (except `/auth/login`) call a registered handler that clears `["me"]` and does `router.replace('/login?next=...')`; or add a `QueryCache`/`MutationCache` `onError` in `lib/providers.tsx`. Make `useEventStream` stop reconnecting on a 401 (EventSource cannot see the status, so probe `/auth/me` on error and stop if 401).

### P2-2 Autosave failure on a trashed page is not explained and cannot recover

- Files: `web/app/spaces/[key]/pages/[id]/edit/page.tsx:212-226` (`SaveIndicator`), `:328-332`.
- Issue: any save error, including 409 `page_trashed`, renders only "Save failed Retry". The server message sits in a `title` attribute. The editor stays editable, Retry fails forever, and Publish also fails with a toast.
- Reproduction (observed): open the editor, trash the page from another session (or the API), type. Visible text is just "Save failed Retry"; `title` is "page is trashed".
- Fix: in the `.catch`, branch on `ApiError.code === "page_trashed"`, set a blocking banner ("This page was moved to the trash. Copy your changes, then restore the page"), `setEditable(false)`, disable Publish and Retry. Render the error text visibly for other failures.

### P2-3 Inline comments always anchor to the first occurrence of the quoted text

- Files: `web/components/comments/anchors.ts:92-118`, `:136-146`; quote captured in `SelectionCommentButton.tsx:52`; API stores only `{quoted_text}`.
- Reproduction (observed): page text "Yes appears. Yes again. Yes thrice." Comment on any "Yes" (created through the API with quote "Yes"). The highlight lands on the first "Yes". Selecting the third "Yes" in the UI behaves the same.
- Fix: send disambiguation with the quote (prefix and suffix text, or occurrence index) in `inline_anchor` and match with it; that needs a small API change (`inline_anchor` is free JSON on the Go side, so it is backward compatible). The longer-term fix is to write the comment mark into `content_json` on creation (the `anchorCommentInContent` helper in `components/editor/commentAnchor.ts` exists for that and is currently unused).

### P2-4 Notification badge is not resynced after the stream reconnects

- Files: `web/components/notifications/NotificationBell.tsx:74-85`, `web/lib/sse.ts:36-51`.
- Issue: the server sends `hello {unread_count}` when the stream opens, and the client ignores it (handlers are only `unread_count` and `notification`). Anything created while the stream was down (API restart, laptop sleep, network drop) never reaches the badge until the 30 s stale time and a remount.
- Fix: register a `hello` handler that sets `["unread"]`, and in `useEventStream` call an optional `onOpen` that invalidates `["unread"]` and `["notifications"]` on every (re)open after the first.

### P2-5 App shell is remounted on every top-level navigation, which tears down and recreates the SSE connection

- Files: `app/page.tsx:64`, `app/spaces/page.tsx:38`, `app/search/page.tsx:335`, `app/notifications/page.tsx:65`, `app/templates/page.tsx`, `app/account/page.tsx:82`, `app/admin/layout.tsx:22`, `app/spaces/[key]/layout.tsx:81`. Each wraps its own `<AppShell>`.
- Reproduction (observed): wrap `EventSource`, click Spaces, Templates, Search, Notifications, Home. Five navigations created five EventSources and closed four. TopBar, Sidebar, bell and the space switcher also remount each time.
- Fix: move the shell into a route-group layout (`app/(app)/layout.tsx`, with `/login` outside it) and let the space layout nest inside it. That also removes the `RequireAuth` duplication.

### P2-6 Mobile navigation drawer is not a real dialog

- File: `web/components/layout/Sidebar.tsx:73-86`.
- Issue: when closed on phones the `aside` is only translated off-screen. It is not `inert` or `aria-hidden`, so keyboard and screen-reader users tab into 9 invisible links (observed). The open drawer has no Escape handling, no focus trap and no `role="dialog"`/`aria-modal`.
- Fix: below `md`, set `inert` (or `visibility: hidden` via a transition-end class) when closed; when open add `role="dialog" aria-modal="true"`, Escape to close, focus the close button on open and return focus to the menu button.

### P2-7 Modals opened inside the sticky inline-comment panel do not cover the page chrome

- Files: `web/components/comments/PageComments.tsx:247-262` (the `lg:sticky lg:overflow-y-auto` wrapper), `CommentItem.tsx:155-165` (`ConfirmDialog` rendered inline), `components/ui/Modal.tsx:68`.
- Issue: a sticky element creates a stacking context, so the dialog's `z-50` is relative to the panel. The TopBar (`z-30`) and sidebar paint above the backdrop.
- Reproduction (observed, 1280px wide): open a page with an inline comment, click Delete. `document.elementFromPoint` at the top bar returns the header search input while the dialog is open; the bar is not dimmed and stays clickable.
- Fix: render `Modal` through `createPortal(..., document.body)` (the sidebar already needed `BodyPortal` for the same class of problem). One change in `Modal.tsx` fixes every current and future call site.

### P2-8 Version history "Changes" view ignores everything except text

- Files: `web/app/spaces/[key]/pages/[id]/history/page.tsx:118-125`, `web/components/pages/VersionDiff.tsx`, `components/pages/utils.ts:70-104` (`htmlToText`).
- Reproduction (observed): publish v2 that only bolds a word and adds a warning panel and a table. History reports "+0 words -0 words / No text changes between these versions."
- Fix: say so plainly ("Text is identical; formatting or structure changed, see Side by side") when the HTML differs but text does not, or diff a structural text rendering that includes node markers (panel colour, table cells, image, link href). At minimum compare `content_html` and show a notice.

## P3

Security and hardening
1. `content_json` is stored unvalidated (`api/.../pages.go` `pageContentJSON` only checks it is an object). A link mark with `href: "javascript:..."` round-trips into the editor, and the "Open link" anchors in `components/editor/BubbleMenus.tsx:124` and `LinkPopover.tsx:161` render `href={href}` from it. Observed: React 18.3.1 rewrites it to a blocked URL, so it is not exploitable today, but safety depends on that React behaviour. Fix: run hrefs through `normalizeHref`/a scheme allowlist before rendering the anchors, and strip non-http(s)/mailto link attrs from `content_json` server-side.
2. `next.config.js:16`: `script-src 'self' 'unsafe-inline'` makes the CSP provide no script-injection protection. The only inline script is the theme bootstrap; a nonce or hashed script would let you drop `'unsafe-inline'`.
3. `app/search/page.tsx:32` `safeSnippet` escapes `<` but not `&`, so page text containing `&lt;b&gt;` is shown decoded as `<b>`. Not exploitable (verified: any `<` not part of `<mark>`/`</mark>` is escaped, and the server markers are exactly `<mark>`), but escape `&` first and then restore only the two mark tags.

State, caching and races
4. `components/templates/UseTemplateDialog.tsx:62-65`: after creating a page it navigates without invalidating `["tree", key]` or `["dashboard"]`; the new draft can be missing from the sidebar for up to the 10 s stale time.
5. Images uploaded from the editor never invalidate `["attachments", pageId]` (`edit/page.tsx:407-413`), so the attachments page can be stale for up to 10 s.
6. `components/importexport/ImportDialog.tsx:62` calls `qc.invalidateQueries()` with no key, refetching every active query (bell, tree, spaces, dashboard). Invalidate `["tree", key]`, `["dashboard"]`, `["search"]` only.
7. `edit/page.tsx:458-494`: edits typed while a publish request is in flight are neither published nor kept (`dirtyRef` is reset on success), and a new autosave timer started during that window can recreate a draft after the server deleted it.
8. `components/pages/PageActions.tsx:57-72`: star is optimistic and `onSettled` only invalidates `["dashboard"]`; a failed-then-ignored race leaves `["page", id]` unrefreshed. Add `qc.invalidateQueries(["page", id])` in `onSettled` (as `WatchToggle` does).
9. `components/spaces/useTreeState.ts:42-46` with `PageTree.tsx:84-90` (reasoned, not reproduced): the ancestor auto-expand runs `commit()` from the default set on first load and can overwrite the persisted localStorage set that `readStored` just queued.
10. Notification and activity pagination cursor is `created_at` with a strict `<` on the server, so two rows with identical timestamps at a page boundary can be skipped.
11. History page shows Restore buttons for a trashed page (`history/page.tsx:256`, `:349`); the click ends in a `page_trashed` toast. Hide when `page.status === "trashed"`.

Accessibility
12. Light mode: `text-ink-500` (96 uses, `#758195` on white = 3.9:1, 3.7:1 on `ink-50`) and `text-ink-400` (22 uses, 3.2:1) fail WCAG AA for small text. Darken `--ink-500` to about `#667389` (4.6:1).
13. Dark mode: `placeholder:text-ink-300` on the editor title (`edit/page.tsx:596`) is 2.0:1; `ink-400` icons are 3.8:1. Other dark pairs checked (`ink-500` 6.2:1, `ink-600`+) pass.
14. `components/ui/ConfirmDialog.tsx:39`: `data-autofocus` is on the confirm button, including for `tone="danger"`, so Enter or Space immediately confirms a destructive action. Focus Cancel for danger dialogs.
15. `components/ui/Modal.tsx:35-39` and `Dropdown.tsx:39-43`: Escape handlers are on `document` and `stopPropagation` does not stop sibling document listeners, so one Escape closes a menu and the modal under it together; Escape also closes a modal while its request is `busy`.
16. History tabs (`history/page.tsx:275-297`) use `role="tab"` without `aria-controls`, tabpanel or arrow-key handling; `AddMember` (`members/page.tsx:116-140`) uses `role="listbox"` with buttons inside options.
17. `components/search/QuickSearch.tsx:79-86`: the global Cmd/Ctrl+K handler also fires inside the editor. Observed: Cmd+K in the editor opens the link popover and the quick-search dropdown at once. Skip when `e.defaultPrevented` or the target is inside `.ProseMirror`.

Performance
18. `components/editor/Editor.tsx:151-153`: every keystroke runs `getJSON()`, `getHTML()` and `getText()`. Measured on a 318 KB page (1,500 paragraphs): `getHTML` about 50 ms per call, so roughly 55 ms per keystroke before React work. The edit page uses only json and html, and reads the live value at publish anyway; keep only a dirty flag in `onUpdate` and call `getValue()` lazily in the debounced save.
19. `components/spaces/PageTree.tsx` / `TreeNode.tsx`: no `React.memo`, inline handler props, and each node registers `useDraggable` and `useDroppable`, so every drag move re-renders every visible node. Fine for hundreds of pages; memoise `TreeNode` and pass stable handlers if spaces grow to thousands.
20. `components/comments/anchors.ts:92-96`: `anchorQuote` rebuilds the whole-document text corpus once per thread. Observed fine with 40 inline threads on a 318 KB page (no long task); it is O(threads x document size), so build the corpus once per sync pass.

UX
21. Space settings: `SettingsShell` shows the General and Members tabs to editors on the Templates and Import and export pages, and they land on "Space settings are for admins". Editors have no sidebar entry to the templates page (they reach it from `/templates`). Hide admin-only tabs when `!isAdmin`, and link template management for editors.
22. Smaller UX gaps: `LabelPicker` Enter picks the first suggestion over the literal typed text; members page applies role changes immediately with no confirmation when demoting yourself; account page offers password change to SSO-only users; `hasContent` in `PageBody.tsx:120` is true for an empty `<p></p>` so the "no content yet" prompt never shows; the history, edit and attachments routes do not canonicalise a mismatched space key the way the view route does; `GET /pages/{id}/draft` returns 404 on every first edit (console noise, handled correctly by `loadDraft`).

Code hygiene
- No comments anywhere in `app`, `components`, `lib` (CSS included); the only comment-like line is the allowed `eslint-disable-next-line` in `SafeBubbleMenu.tsx:52`.
- `any` appears only in `lib/sse.ts`.
- Placeholders: `QuickSearch.tsx` (258 lines), `NotificationBell.tsx` (184), `WatchToggle.tsx` (102), `PageLabels.tsx` (136) and `PageComments.tsx` (281) are all real implementations.
- Dead exports: `AvatarGroup` (`ui/Avatar.tsx`), `EMPTY_DOC`, `emptyDoc`, `docToPlainText` (`lib/content/doc.ts`), `isApiStatus` (`pages/utils.ts`), `listChildren` and `updatePage` (`lib/api/pages.ts`), and the whole of `components/editor/commentAnchor.ts` (`anchorCommentInContent`, `removeCommentFromContent`, re-exported but never called).
- Dead props: `AppShell.contentClassName`, `MoveDialog.onMoved`, `ConfirmTrash.onTrashed`, `ImportDialog.onImported` (none passed by any caller). `components/editor/__contract__.ts` is referenced only by the barrel (the Go test embeds its own copy).
- `lib/content/slugify.ts` is a heading-id slugger reused as a page-slug fallback in `notifications/format.ts:72-86`, `ActivityItem.tsx` and `ImportDialog.tsx:109`; it differs from the API slug for empty or non-ASCII titles (returns `section`, drops accents). Harmless because the view route canonicalises, but the links then go through a redirect.

## Contract table

Routes were compared against every `Routes()` registration and handler body in `api/internal/http/handlers`. All paths, methods, request body fields and query parameters used by `lib/api/*.ts` exist and match.

| Area | Web (`lib/api`) | Go | Result |
|---|---|---|---|
| Watch PUT/DELETE | `Promise<void>` (`watch.ts:11-17`) | returns `200 {watching}` (`watch.go`) | Harmless; body ignored. Type could be `WatchState`. |
| `GET /pages/{id}/draft` | `loadDraft` maps 404 to null | 404 `not found` when no draft | Works; noisy console error on first edit. |
| `Page.updated_by` | `string \| null` | non-null uuid | Superset, fine. |
| `Page.content_text` | optional | `json:"-"`, never sent | Fine; remove from type. |
| `PageDetail.my_role` | `Role \| null` | always a `Role` | Fine. |
| `GET /notifications` | reads `X-Unread-Count` | header set and exposed for CORS | Matches. |
| SSE events | handles `unread_count`, `notification` | also sends `hello {unread_count}` | `hello` unused (see P2-4). |
| `POST /notifications/read` | `{ids}` or `{all:true}` | same; 204 | Matches. |
| `before` cursor (notifications, activity) | `created_at` RFC3339 | `RFC3339Nano`, strict `<` | Works; tie risk (P3-10). |
| `POST /spaces/{key}/import` | multipart `files` + `parent_id`, filename `webkitRelativePath` | `{pages: PageSummary[]}` 201 | Matches. |
| `PATCH /pages/{id}` | `updatePage` (unused) | title only | Unused client function. |
| `GET /pages/{id}/children` | `listChildren` (unused) | exists | Unused client function. |
| `GET /templates/{id}` | none | exists | No client; unused server route. |
| `GET /spaces/{key}/trash` | editor-only in UI | `PermEditPages` | Matches. |
| Empty collections | arrays assumed | `[]` for every list endpoint probed (comments, labels, attachments, children, versions, trash, activity, notifications, search, quick, labels index, templates, users, dashboard sections) | Matches; no `null` lists. |
| Dashboard / search / template / version / activity / notification shapes | `types.ts` | `store/*.go` view structs | Field names and nullability match (checked each struct). |
| Archived spaces | UI treats archived as read-only for everyone (`canEdit = isEditor && !archived`) | middleware lets space admins and system admins still write | UI is stricter than the API; consistent with the product copy, noted for clarity. |

No mismatch was found that breaks a call. Plan deltas worth knowing: `PATCH /pages/{id}` is allowed for any non-trashed page (plan said published only), and `POST /spaces/{key}/pages` with `parent_id: null` means "under the space home page".

## Verified working

Build and hygiene
- `npm run lint` and `npm run typecheck` clean; no comments, no `any` outside `lib/sse.ts`.
- Production stack came up healthy; CSP, `X-Frame-Options`, `nosniff`, `Referrer-Policy` and `Permissions-Policy` headers present.

Security
- Every `dangerouslySetInnerHTML` use renders sanitised content: page, version, template and comment HTML pass through `content.SanitizeHTML` on every write path (pages, drafts, versions restore, templates, comments, import, copy); the only other use is the static theme script. Search snippets are escaped client-side as well.
- `ContentView` link handler only handles same-origin paths beginning `/` (not `//`), skips modified clicks and `target` links, and cannot reach `javascript:` or other schemes (sanitiser strips them; CSP `img-src` blocks external image beacons).
- Crafted `javascript:` link in `content_json` rendered as a React-blocked URL in the editor bubble (see P3-1).

Auth and gating
- Unauthenticated deep link redirects to `/login?next=<path+query>`; sign-out lands on `/login` and clears the cache.
- Viewer: no Edit, Create-page, Add label, Trash, Settings or Attachments upload; editor route shows "You can't edit this page"; viewer can still comment, resolve, star and watch (matches API).
- Admin routes gated by `RequireAdmin`; space settings and purge gated by `isAdmin`; trash by `isEditor`; 403 and 404 states render for space and page.

Core flows
- Slug canonicalisation (wrong or missing slug, unicode slug `über-café`, no redirect loop); reserved slugs `edit`, `history`, `attachments` become `-page`; lowercase space key resolves.
- Editor autosave: debounced PUT, in-flight saves serialised, unmount flush sent a PUT 49 ms after a keystroke and the content was stored; copy dialog creates a draft copy and lands in the editor with the toast.
- Large page: 318 KB, 1,500 paragraphs, 40 inline anchors renders about 5,250 DOM nodes with no long tasks.
- 375px: no horizontal scroll on `/`, `/spaces`, a page with a wide table, long code line, long URL and 2000px image, the editor, history and `/admin/users` (the users table scrolls inside its own container).
- Focus ring is global (`*:focus-visible`); dialogs close on Escape and label themselves; tree is ARIA `tree` with roving tabindex and arrow-key navigation (by code; keyboard drag is replaced by the Move dialog).
- React Query invalidation reviewed per mutation (create, move, trash, restore, purge, publish, restore version, labels, comments, members, templates, watch, star); the gaps are listed in P3-4 to P3-8.
- Mutations toast on failure; `version_conflict` opens the conflict dialog with "Publish anyway" (`force`); `page_trashed` is toasted for comments, labels, trash and publish (the autosave case is P2-2).
