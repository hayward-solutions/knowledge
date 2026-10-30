# C2 spaces and pages UI notes

Gate (clean, run in `web/`): `npm run lint` (no warnings or errors), `npm run typecheck`, `npm run build` all pass. New build routes: `/` (dashboard), `/spaces`, `/spaces/[key]`, `/spaces/[key]/pages/[id]`, `/spaces/[key]/pages/[id]/[slug]`, `/spaces/[key]/pages/[id]/history`, `/spaces/[key]/pages/[id]/attachments`, `/spaces/[key]/trash`, `/spaces/[key]/settings`, `/spaces/[key]/settings/general`, `/spaces/[key]/settings/members`. No dependencies added, no comments in code, no lib/api changes, no git.

## Runtime verification

Run against a real API (own Postgres container on 55440, API on 8090, web production build on 3100 and `next dev` on 3101, all from a copy of `web/` in the scratchpad so no `.next` clobbering; all stopped afterwards). MinIO could not be pulled, so attachment upload success and list rendering with real rows were NOT exercised (only the empty state, the drop zone and the 503 "object storage not configured" error path).

Exercised OK: login, dashboard sections, space directory, create-space dialog (key suggestion, 409 key error), space home, page view (byline, TOC rail, code highlight, child pages, labels and comments slots, star, watch), slug canonicalisation (wrong slug, missing slug, query and hash preserved), tree (expand/collapse persisted in localStorage, keyboard arrows/Home/End/Shift+F10/Escape, context menu, drag onto, before, after and after-expanded-parent with server state matching the optimistic state), New page dialog with starter templates then edit page inside the layout, Move dialog, trash from the page and from the tree, trash view with Restore, trashed page view (badges, Restore, Delete permanently), history (diff, restore to a new version, redirect to the page), settings general (save, icon), members (user search, add, last-admin 409 toast), archive banner and read-only state, viewer role (no Create/Edit/Trash/Settings, trash and settings EmptyStates, private space 404 EmptyState), 390px mobile layout with drawer, no horizontal overflow. Not exercised: Purge, Copy dialog submit, delete space, Unarchive via the UI button, print output, light theme visuals.

## Routes

- `app/spaces/[key]/layout.tsx` (client): `RequireAuth` then loads `["space", key]` and `["tree", key]`, renders `AppShell sidebar={<SpaceSidebar/>}`, provides `SpaceProvider`, archived banner, 404/403/error `EmptyState`. All `/spaces/[key]/**` routes (including C1's edit page and C4's activity and templates pages) render inside it and must not wrap themselves in `AppShell`.
- `app/spaces/page.tsx` directory (own `AppShell`), `app/page.tsx` dashboard (own `AppShell`).
- `app/spaces/[key]/page.tsx` renders `PageBody` for `space.home_page_id` with `canonical={false}` (URL stays `/spaces/KEY`).
- `.../pages/[id]/[slug]/page.tsx` and `.../pages/[id]/page.tsx` both render `PageBody`, which `router.replace`s to `/spaces/{space_key}/pages/{id}/{slug}` (query and hash kept) when the URL differs.
- `history/page.tsx`, `attachments/page.tsx`, `trash/page.tsx`, `settings/page.tsx` (redirects to `settings/general`), `settings/general/page.tsx`, `settings/members/page.tsx`.

## Context hook

`import { useSpace } from "@/components/spaces/SpaceContext"` returns `{ space, role, tree, treeLoading, treeError, isAdmin, isEditor, archived, canEdit, refresh }`. `role` is `admin` for system admins, else `space.role ?? "viewer"`. `isAdmin` means role admin, `isEditor` means admin or editor, `canEdit = isEditor && !archived` (use it for write actions). `refresh()` invalidates `["space", key]` and `["tree", key]`. `useOptionalSpace()` returns null outside the layout. `tree` is the flat `PageSummary[]` from `GET /spaces/{key}/tree`.

## Components

- `components/spaces/`: `SpaceContext` (+ `SpaceProvider`, `useSpaceData`), `SpaceSidebar`, `PageTree`, `TreeNode`, `TreeContextMenu`, `useTreeState`, `useMovePage` (optimistic move with rollback, shared by drag, menu and Move dialog), `tree.ts` (`buildTree`, `flattenAll`, `flattenVisible`, `applyMove`, `descendantIds`, `ancestorIds`, `siblingsOf`, `pageIdFromPath`), `BodyPortal`, `SpaceCard`, `SpaceIcon`, `CreateSpaceDialog` (exports `suggestKey`), `SettingsShell` (title, tabs General | Members | Templates | Import and export, admin gate; C4's templates and import-export pages may wrap themselves in it with `adminOnly={false}` if they want matching tabs, they currently do not).
- `components/pages/`: `PageBody` (data load, canonical URL, whole page layout), `PageHeader`, `PageActions`, `Breadcrumbs` (`PageBreadcrumbs`), `ChildPages`, `TableOfContents`, `MoveDialog`, `CopyDialog`, `NewPageDialog`, `ConfirmTrash`, `ExportMenu`, `ParentSelect`, `PageRow`, `VersionDiff`, `Skeletons`, `RelativeTime`, `MenuLink`, `utils.ts` (`pageHref`, `pageEditHref`, `pageHistoryHref`, `pageAttachmentsHref`, `slugSegment`, `formatBytes`, `htmlToText`, `absoluteUrl`, `errorMessage`).
- `ExportMenu({ page, onSelect? })` renders an "Export" label plus Markdown and HTML `<a download>` menu items (`pageExportUrl`) meant to sit inside the actions `Menu`.

## Slots for other agents (all three already existed, nothing overwritten, no placeholder files created)

`PageBody` renders exactly these, so changing their props breaks the build:
- `<PageLabels pageId={page.id} canEdit={canEdit && !trashed} initial={page.labels} />`
- `<PageComments key=... page={page} canComment={!archived} canEdit={isAdmin} contentRef={ref to the div wrapping ContentView} activeCommentId onActiveCommentChange />`
- `<WatchToggle type="page" id={page.id} initial={page.watching} />` in `PageActions`, and `<WatchToggle type="space" id={space.id} size="sm" />` in `SpaceSidebar`.

## Query keys

`["space", key]`, `["tree", key]`, `["page", id]` (staleTime 0 so each visit refetches and records a view), `["versions", id]` and `["versions", id, "content", n]`, `["attachments", id]`, `["trash", key]`, `["members", key]`, `["dashboard"]`, `["spaces"]` (shared with Sidebar and C4), `["templates"]` (shared with C4, `listTemplates`), `["user-search", q]`. Mutations invalidate the relevant subset; move, trash, restore and publish-like actions also invalidate `["page"]` (breadcrumbs) and `["dashboard"]`.

## Behaviour decisions and deviations

- Content is rendered with `ContentView` (`.kn-content`) and not inside a Tailwind `prose` wrapper, because C1's `.kn-content` styles already cover typography and `prose` would only add colour conflicts. The page column is `max-w-[860px]` with a sticky TOC rail from `xl`.
- Drag and drop uses `@dnd-kit/core` only (MouseSensor distance 6, TouchSensor 250 ms hold) with `DragOverlay`. The top 28 percent of a row means before, bottom 28 percent after, middle onto (child, appended). After an expanded parent means first child. Root-level rows (the home page) only accept onto and the home page is not draggable. Keyboard users move pages with the Move dialog (menu, page actions menu), which is the accessible fallback; there is no dnd-kit KeyboardSensor.
- Tree is ARIA `tree`/`treeitem` (flat, `aria-level`) with roving tabindex; ArrowUp/Down/Left/Right, Home, End, Enter, Shift+F10/ContextMenu key. Draft pages link to the edit route for editors, to the view route otherwise.
- Dialogs launched from the sidebar and the tree context menu are rendered through `BodyPortal`: `Sidebar`'s `aside` has `transform` classes, which makes `position: fixed` children relative to the aside, so a `Modal` rendered inside it was trapped in the sidebar.
- `PageComments` is given `key={status-archived}`: C1's compact editor (comment composer) uses a TipTap/tippy BubbleMenu that detaches its element itself, so unmounting only the composer (page flips to trashed, `commentingAllowed` false) throws `NotFoundError: removeChild` and crashes the page. Remounting the whole `PageComments` subtree avoids it. The underlying bug is in the compact editor BubbleMenu and affects any parent that conditionally unmounts a compact `Editor` while keeping its own DOM; C1/C3 should fix it.
- Slugs `edit`, `history`, `attachments` collide with the static route segments under `/pages/[id]/`. `pageHref`/`slugSegment` appends `-page` for those slugs (`PageBody` canonicalises to that). C1's publish redirect uses the raw slug, so publishing a page titled "Edit" lands on the editor; the API's slug generator (or C1) should avoid these three words.
- Print: `PageBody` injects a small `<style>` with `@media print` hiding the sidebar `aside` and sticky top bar (A2's layout components have no `print:` classes and are not mine); actions, banners, TOC, child pages and comments use `print:hidden`.
- Settings: `/settings` redirects to `/settings/general`. Tabs live in `SettingsShell` (per page), not a `settings/layout.tsx`, to avoid touching C4's routes. The tabs link to `/settings/import-export` (C4) as "Import and export" beside the three requested ones.
- Space role for the UI comes from `GET /spaces/{key}` `role` (null becomes viewer) and system admin status; the API remains the authority.
- Not implemented from the brief: none dropped. Additions: side-by-side rendered comparison tab in history, space "Watch" toggle and Activity link in the sidebar, copy-link in the tree menu, hash scroll after load.

## Open issues for integration

- C1/C3 compact editor BubbleMenu unmount bug (above).
- Reserved slug collision (above).
- Attachment success path and list rendering need a run with MinIO.
- The TopBar "Create" button (A2) still links to `/spaces`; it could open the new-page dialog.
