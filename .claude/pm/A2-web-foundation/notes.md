# A2 web-foundation notes

Gate (clean `rm -rf node_modules .next`, then): `npm ci`, `npm run lint`, `npm run typecheck`, `npm run build` all pass. `docker build -t knowledge-web:dev .` succeeds. Smoke run with `--read-only --tmpfs /app/.next/cache` as uid 1000: `/login` 200, unknown path 404, CSP and X-Frame-Options headers present.

Build output routes: `/`, `/_not-found`, `/account`, `/admin/users`, `/login`.

## Dependency versions (all exact pins)

next 15.5.28, react/react-dom 18.3.1, typescript 5.9.3, tailwindcss 3.4.19, postcss 8.5.32, autoprefixer 10.6.2, @tailwindcss/typography 0.5.20, @tanstack/react-query 5.105.0, all @tiptap/* 2.27.3 (core, pm, react, starter-kit, extension-table/-table-row/-table-cell/-table-header/-task-list/-task-item/-code-block/-code-block-lowlight/-image/-link/-placeholder/-mention/-underline/-highlight/-text-align/-typography/-bubble-menu/-floating-menu/-character-count, suggestion), lowlight 3.3.0, highlight.js 11.12.0, tippy.js 6.3.7, @dnd-kit/core 6.3.1, @dnd-kit/sortable 10.0.0, @dnd-kit/utilities 3.2.2, diff 9.0.0, @types/diff 8.0.0, date-fns 4.4.0, lucide-react 1.65.0, clsx 2.1.1, eslint 9.39.5, eslint-config-next 15.5.28, @types/node 22.20.5, @types/react 18.3.31, @types/react-dom 18.3.7.

No agent after A2 may add dependencies. If one is needed, report it.

## Deviations and decisions

- Extra deps beyond the brief: `@tiptap/extension-code-block` and `highlight.js` (peer deps of code-block-lowlight, pinned explicitly so `npm ci` is deterministic).
- `@types/diff@8.0.0` is a deprecated stub (diff 9 ships its own types). Installed because the brief required it; harmless. Import types from `diff` directly.
- eslint is 9.39.5 (not 10) because eslint-config-next 15.5 peers `^7 || ^8 || ^9`. `next lint` prints a deprecation notice (removed in Next 16); it still works. Config is `.eslintrc.json` (`next/core-web-vitals`, `next/typescript`), with `no-explicit-any` disabled for `lib/sse.ts` only (the handler signature uses `any`, per brief).
- `lucide-react` is the icon set (no hand-rolled icons.tsx as in Backlog).
- `.gitignore` ignores `next-env.d.ts` and `*.tsbuildinfo`. `npm run typecheck` works on a fresh checkout without `next-env.d.ts`.
- `RequireAuth` is built into `AppShell`, so any page rendered inside `AppShell` is auth-gated and needs no extra wrapper. `/login` does not use `AppShell`.
- `app/admin/layout.tsx` itself renders `AppShell` + `RequireAdmin` + the Users/Spaces/Templates/Labels tabs (route links). Admin pages therefore must NOT wrap themselves in `AppShell`; they render only their content (header row, toolbar, table) as a fragment. `app/admin/users/page.tsx` follows this. `/admin/spaces|templates|labels` do not exist yet (C4).
- Same pattern is recommended for space routes: a `app/spaces/[key]/layout.tsx` renders `<AppShell sidebar={<PageTree/>}>` once so the tree persists across navigation.
- Content-Type `application/json` is only sent when the request has a body (avoids needless CORS preflight on GETs).
- `ApiError` carries `status`, `code`, `message` and the parsed JSON `body` (needed for `current_version` on 409).
- Type shapes for fields the plan did not pin down are my best guess; D1 must reconcile against the Go handlers. Assumptions: `Space.role` is the caller's role in `GET /spaces`; `SpaceMember` is `{user_id, user: UserRef, role, created_at}`; `Page.created_by_user` / `updated_by_user` are optional resolved `UserRef`s beside the id fields; `Page.space_key` and `PageDetail.my_role` are optional; `PUT /pages/{id}/labels` body is `{labels: string[]}` (names); `GET /notifications` returns `Notification[]`; `GET /watch/{type}/{id}` returns `{watching}`; `POST /spaces/{key}/import` multipart field is `files` plus `parent_id`; `GET /search` returns `{hits,total,page,limit}`; `GET /search/quick` returns `QuickSearchHit[]` (`{page, space_key, space_name?}`); `/me/dashboard` lists are `DashboardPage[]` (PageSummary plus `space_key`, `updated_at`, `author`).
- Password policy helper (`lib/password.ts`) enforces the plan's 12+ chars with upper/lower/digit/symbol on the account, create-user and reset-password forms.

## editor.css

`/Users/matt/Projects/hayward-solutions/knowledge/web/app/editor.css` is an empty file, imported from `app/globals.css` via `@import "./editor.css";`. Verified behaviour: the import is hoisted by css-loader to the very top of the emitted stylesheet, i.e. BEFORE Tailwind base/components/utilities. `@apply` and `theme()` work inside it, and the `.dark` variables from globals work. `@layer components { ... }` does NOT work inside editor.css (Tailwind errors: no matching `@tailwind components` directive), so use plain selectors. Because it is emitted first, use selectors with at least class specificity (for example `.ProseMirror h1`, `.prose .x`) so preflight and utilities do not override them.

## Placeholder components to be overwritten

- `/Users/matt/Projects/hayward-solutions/knowledge/web/components/search/QuickSearch.tsx` exports `QuickSearch()` (no props). Currently an input that routes to `/search?q=` on Enter. C3 overwrites it.
- `/Users/matt/Projects/hayward-solutions/knowledge/web/components/notifications/NotificationBell.tsx` exports `NotificationBell()` (no props). Currently a bell link to `/notifications`. C4 overwrites it.
- `/Users/matt/Projects/hayward-solutions/knowledge/web/app/page.tsx` is the placeholder home dashboard (C2 replaces it).

TopBar imports both by those exact paths and named exports.

## Exports for later agents

### lib/api (import from `@/lib/api`)

- `client.ts`: `API_BASE`, `ApiError {status, message, code?, body?}`, `api<T>(path, init?)` (path is relative to `/api/v1`, credentials included, JSON, 204 -> undefined), `apiUpload<T>(path, formData, init?)` (POST multipart, no Content-Type header), `apiUrl(path)` (absolute URL: a path starting `/api/` becomes `${API_BASE}${path}`, anything else becomes `${API_BASE}/api/v1${path}`; use for `<img src>`, downloads and SSE), `jsonBody(value)`, `qs(params)` (drops empty values).
- `types.ts`: `User, UserRef, Space, SpaceMember, Role, Visibility, PageStatus, Page, PageSummary, PageDetail, MyDraftRef, TrashedPage, PageVersion, PageVersionSummary, PageDraft, Label, InlineAnchor, Comment, Attachment, Template, Notification, Activity, SearchHit, SearchResult, QuickSearchHit, Dashboard, DashboardPage, VersionConflict, OIDCConfig, EditingState, WatchState, WatchTargetType, AdminSpace, PMDoc` and inputs `DraftInput, PublishInput, MovePageInput, CreatePageInput, CreateSpaceInput, UpdateSpaceInput, TemplateInput, CommentInput, SearchParams, ImportResult, ExportFormat`. All snake_case.
- `auth.ts`: `login, logout, getMe, updateMe, changePassword(current,new), oidcConfig, oidcLoginUrl(next?)`.
- `users.ts`: `searchUsers(q), getUser(id)`.
- `admin.ts`: `listAdminUsers, createAdminUser, patchAdminUser(id, patch), listAdminSpaces` (+ `CreateAdminUserInput`, `PatchAdminUserInput`).
- `spaces.ts`: `listSpaces, createSpace, getSpace, updateSpace, deleteSpace, archiveSpace, unarchiveSpace, listSpaceMembers, putSpaceMember(key,userId,role), removeSpaceMember, getSpaceTree, getSpaceActivity(key,{before?,limit?}), getSpaceTrash`.
- `pages.ts`: `createPage, getPage, updatePage, saveDraft, getDraft, discardDraft, publishPage, isVersionConflict(err), movePage, trashPage, restorePage, purgePage, copyPage, listChildren, starPage, unstarPage, sendEditingHeartbeat`.
- `versions.ts`: `listVersions, getVersion, restoreVersion`.
- `comments.ts`: `listComments, createComment, updateComment, deleteComment, resolveComment, reopenComment`.
- `labels.ts`: `listLabels(q?), getPageLabels, setPageLabels(pageId, names), listSpaceLabels, listLabelPages(key,name)`.
- `search.ts`: `search(params), quickSearch(q)`.
- `attachments.ts`: `listAttachments, uploadAttachment(pageId, file), deleteAttachment, attachmentContentPath(id)` (the `/api/v1/attachments/{id}/content` path stored in HTML), `attachmentContentUrl(id)`, `attachmentDownloadUrl(id)`.
- `templates.ts`: `listTemplates, listSpaceTemplates, createSpaceTemplate, createGlobalTemplate, updateTemplate, deleteTemplate`.
- `watch.ts`: `getWatchState(type,id), watchTarget(type,id), unwatchTarget(type,id)`.
- `notifications.ts`: `NOTIFICATIONS_STREAM_PATH`, `listNotifications({unread?}), markNotificationsRead({ids}|{all:true})`.
- `dashboard.ts`: `getDashboard`.
- `export.ts`: `pageExportUrl(id, "md"|"html"), spaceExportUrl(key), importMarkdown(key, files, parentId?)`.

### lib

- `lib/sse.ts`: `useEventStream(path: string | null, handlers: Record<string, (data: any) => void>)`. `path` is passed through `apiUrl` (e.g. `NOTIFICATIONS_STREAM_PATH`). EventSource with credentials, JSON-parses each event's data (falls back to the raw string), reconnects with 1s doubling to 30s, resets on open. Handlers are read through a ref so passing a new object each render does not reconnect; the set of event names (keys) does.
- `lib/auth.tsx`: `useMe()` (react-query, key `ME_QUERY_KEY = ["me"]`, no retry), `safeNext(raw, fallback="/")`, `<RequireAuth>`, `<RequireAdmin>`.
- `lib/theme.tsx`: `ThemeProvider`, `useTheme()` (`theme, resolved, setTheme, toggle`), `themeNoFlashScript` (storage key `knowledge.theme`).
- `lib/providers.tsx`: `Providers` (Theme, QueryClient staleTime 10s with refetchOnWindowFocus false, Toast).
- `lib/cn.ts`: `cn(...)` (clsx). `lib/password.ts`: `passwordPolicyOk, PASSWORD_HINT, MIN_PASSWORD_LENGTH`.

### components/layout

- `AppShell({children, sidebar?, topSlot?, contentClassName?})`: includes `RequireAuth`, mobile drawer, `Sidebar`, `TopBar`, and a `<main className="flex min-h-0 flex-1 flex-col ...">` content area. `sidebar` renders under the primary nav in a scrolling region (page tree slot). `topSlot` is shown left of the search box from the `md` breakpoint (breadcrumbs).
- `Sidebar({mobileOpen?, onMobileClose?, children?})`: space switcher (query key `["spaces"]`, `listSpaces`), Home, Spaces, Search, Templates, Notifications, Admin (system admins), then `children` as the tree slot. Active space comes from the `/spaces/[key]` URL prefix.
- `TopBar({children?, onMenuClick?})`: search slot, Create button (placeholder link to `/spaces`), bell slot, account menu (name, email, Account, theme toggle, Sign out, `Knowledge ${NEXT_PUBLIC_APP_VERSION}`).
- `Breadcrumbs({items: {label, href?}[]})`.

### components/ui (also re-exported from `@/components/ui`)

- `Button({variant: primary|secondary|ghost|danger, size: sm|md|lg, ...button props})`, uses `.btn` classes.
- `Input`, `Textarea({rows=4})`, `Select` (forwardRef, `.control` class). `Field({label?, hint?, error?, className?, children})` wraps a label.
- `Modal({title, onClose, width=640, footer?, children})`: focus trap, Escape, restores focus, `aria-labelledby`. Put `data-autofocus` on an element to focus it on open.
- `ConfirmDialog({title, message?, confirmLabel, cancelLabel, tone: primary|danger, busy?, onConfirm, onCancel})`.
- `Menu` (alias `Dropdown`) `({trigger: (open)=>ReactNode, children: (close)=>ReactNode, align=right, width=200, label?, className?, triggerClassName?})`, `MenuItem({onClick, tone?, disabled?})`, `MenuDivider`. Arrow-key navigation over `role="menuitem"` elements; a `Link` with `role="menuitem"` also works inside.
- `Badge({tone: neutral|blue|green|amber|red|purple|teal|magenta, bold?, title?})`.
- `Avatar({name, seed?, size=24})`, `AvatarGroup({names, size, max})`.
- `ToastProvider` (already in `Providers`), `useToast()` -> `{toast(msg, tone?, ms?), success, error, info}`.
- `EmptyState({title, description?, action?, icon?})`, `Spinner({size, label?, className?})`.
- `Tabs({tabs: {id,label,href?,count?}[], value, onChange?, className?})`: items with `href` render as links, otherwise buttons.
- `Tooltip({content, side=top|bottom, children})`: CSS only, shows on hover and focus-within.

### Styling

`app/globals.css` holds Backlog's tokens (`brand`, `ink` scale flipped under `.dark`, `danger/warning/success/purple/teal/magenta`), `.surface`, `.control`, `.btn*`, `.chip`, `.link`. Tailwind content globs cover `app`, `components`, `lib`. Typography plugin is enabled (`prose`); the prose/custom-node styling belongs to C1 in `editor.css` / `globals.css`.

### Env

`NEXT_PUBLIC_API_BASE_URL` (empty means same origin), `NEXT_PUBLIC_APP_VERSION` (Docker `VERSION` build arg; defaults to `dev` in the UI). The CSP `connect-src` and `img-src` include the API base. The API base is baked in at build time.

## CSP follow-up (next.config.js)

`script-src` now appends `'unsafe-eval'` only when `process.env.NODE_ENV !== "production"`, so `next dev` hydrates while production builds and images keep the strict policy. Verified by loading the config under both NODE_ENV values: production `script-src 'self' 'unsafe-inline'`, development `script-src 'self' 'unsafe-inline' 'unsafe-eval'`. Lint, typecheck and build pass after the change. One earlier build attempt failed with ENOENT on `.next/standalone` files because another agent's `next build` was running in the same `web/` directory at the same time; the rerun was clean. Concurrent builds in `web/` clobber each other's `.next`.
