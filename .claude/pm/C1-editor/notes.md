# C1 editor notes

Gate (all clean): `cd web && npm run lint && npm run typecheck && npm run build`. Build route added: `ƒ /spaces/[key]/pages/[id]/edit` (198 kB).

Runtime smoke test (production build, API mocked with Playwright route interception, then removed): editor loads sanitised-contract HTML and `getHTML()` returns it unchanged for mention, page link, panel, expand, status, toc, comment mark, image, task list, code block; slash menu (`/info`), `@` mention and `[[` page link suggestions work; heading ids are assigned and deduplicated; autosave PUTs with `base_version`; resume banner with stale-draft note; publish dialog, 409 `version_conflict` dialog, "Publish anyway" sends `force: true` and navigates to `/spaces/KEY/pages/ID/<slug>`; `ContentView` builds TOC, highlights code, adds copy buttons, read-only task checkboxes, table wrappers, comment mark click callback. Not exercised: real image upload, drag and drop, dark mode visuals, mobile layout.

## Files

- `web/components/editor/`: `Editor.tsx`, `ContentView.tsx`, `Toolbar.tsx`, `BubbleMenus.tsx`, `LinkPopover.tsx`, `SuggestionList.tsx`, `suggestion-render.ts`, `slash-items.tsx`, `lowlight.ts`, `defaults.ts`, `types.ts`, `commentAnchor.ts`, `__contract__.ts`, `index.ts`, `extensions/*`.
- `web/lib/content/`: `slugify.ts`, `headings.ts`, `doc.ts`, `initial.ts`, `index.ts`.
- `web/app/editor.css`, `web/app/spaces/[key]/pages/[id]/edit/page.tsx`.
- No changes to `lib/api/*` (existing exports were sufficient).

## Editor API

```tsx
import { Editor, type EditorHandle } from "@/components/editor";
<Editor
  ref={handleRef}
  value={{ json, html }}
  onChange={({ json, html, text }) => ...}
  editable
  variant="full" | "compact"
  placeholder
  uploadImage={(file) => Promise<{ id, url }>}
  searchUsers={(q) => Promise<UserRef[]>}
  searchPages={(q) => Promise<PageLinkItem[]>}
  onRequestComment={({ from, to, text }) => ...}
  autofocus={false | true | "start" | "end"}
  className
  ariaLabel
/>
```

- `value` is read once (initial content). Use the handle to replace it: `EditorHandle { editor, getValue(), setContent({json,html}), clear(), focus("start"|"end"), isEmpty() }`. `getValue()` is exact and synchronous (use it at publish or comment submit rather than the last `onChange`).
- `searchUsers` and `searchPages` default to `GET /users?q=` and `GET /search/quick` (hits mapped to `{id,title,slug,spaceKey,spaceName}`), so embedding needs no wiring. Without `uploadImage` the image toolbar button, slash item, paste and drop are inactive.
- Image upload errors toast through `useToast` (the editor must be under `Providers`, as the whole app is).
- `onChange` fires only for user edits (not initial load, `setContent`, or `clear`).
- Full variant: sticky toolbar (offset by CSS variable `--kn-toolbar-top`, default 56px = TopBar height; the edit page sets 105px because of its own sticky action bar), bubble menu, slash menu, table/status/panel/code/image bubbles, word count.
- Compact variant: bubble menu only (marks and link), `@` mentions and `[[` page links enabled, no toolbar, no slash menu, max height 240px with scroll.

## Embedding the compact editor (comments)

```tsx
const ref = useRef<EditorHandle>(null);
<Editor ref={ref} variant="compact" value={{ json: null, html: "" }} placeholder="Write a comment" onChange={setDraft} />
const v = ref.current?.getValue();
createComment(pageId, { body_json: v.json, body_html: v.html });
ref.current?.clear();
```
Use `isEmpty()` (or `isEmptyDoc` from `@/lib/content`) to disable submit. To render a stored comment read-only use `<ContentView html={body_html} />` or `<Editor editable={false} .../>`.

## ContentView

`<ContentView html className onHeadingsReady?(HeadingInfo[]) onCommentMarkClick?(id, element) activeCommentId? />` renders server-sanitised HTML inside `.kn-content` and in an effect: assigns/dedupes heading ids, fills `div[data-type=toc]`, wraps task item contents so they match the editor DOM (read-only disabled checkbox), syntax-highlights `pre > code.language-x` with lowlight and adds a Copy button, wraps tables in `.kn-table-wrap`, rewrites `img[src^="/api/"]` through `apiUrl` (matters only if `NEXT_PUBLIC_API_BASE_URL` is set), adds `kn-comment-mark` to `span[data-type=comment]` (click calls `onCommentMarkClick`; `activeCommentId` adds `is-active` to the matching mark), and turns same-origin relative link clicks into `router.push`. Expand sections are native `<details>`.

`useHeadings(html)` returns `HeadingInfo[]` = `{ id, level, text }`, computed from the HTML string with the same id algorithm the component applies to the DOM, so it is safe to call in the page component for a sticky TOC and the ids match the anchors. `extractHeadings(html)` is the same function outside React.

## CommentMark commands

Registered as mark `comment` (HTML `<span data-type="comment" data-id="<commentID>">`, `inclusive: false`, `excludes: ""` so overlapping comments are possible):

- `editor.commands.setCommentMark(id)` applies the mark to the current selection.
- `editor.commands.unsetCommentMark(id)` removes only marks with that id everywhere in the doc and returns whether anything changed.

Because the page view renders `content_html` (no editor), anchoring a new comment on a published page requires editing content. Helpers (headless TipTap editor, browser only) in `@/components/editor`:

- `anchorCommentInContent({json, html}, quotedText, commentId) -> {json, html, text} | null` (first occurrence inside a single text block; null if not found, e.g. text spanning blocks).
- `removeCommentFromContent({json, html}, commentId) -> {...} | null`.
The caller must persist the result (publish or a metadata endpoint). Sanitiser admits `span[data-type=comment]` with UUID `data-id`.

## `.kn-content` convention

Any container that shows page or comment content gets class `kn-content`. The editor surface has it too, so view and edit look identical. All custom-node styles are scoped under `.kn-content`; editor-only chrome uses `.kn-editor*`, `.kn-toolbar`, `.kn-bubble`, `.kn-suggest*`, `.kn-link-popover`. Dark mode comes from `.dark` CSS variables (`--kn-*` tokens defined in `editor.css`). Code blocks are dark in both themes. editor.css uses plain selectors (no `@layer`), all at class specificity or higher so preflight does not override them.

## Contract notes and uncertainties (for the integration agent)

- `__contract__.ts` exports `CONTRACT_SAMPLES` (one HTML snippet per node, taken from real `getHTML()` output) to diff against the Go sanitiser.
- Text alignment: `KnTextAlign` (extensions/text-align.ts) emits `data-align="center|right|justify"` on p and h1-h6 (nothing for left) and parses `data-align`, falling back to `style.textAlign`. Styled by `.kn-content [data-align=...]`. Requires the sanitiser to admit `data-align` with left|center|right|justify on p and h1-h6.
- Page links in `content_json` are node type `pageLink` (attrs `pageId`, `title`, `href`). `content/text.go` does not treat `pageLink` as an inline leaf, so link titles are missing from `content_text` (search). Add `pageLink` to `inlineLeaves` and read `title` in `leafText`.
- Mention JSON attrs: `id`, `label`, `mentionSuggestionChar` (always "@"). `MentionedUserIDs` reads `id` only, which matches.
- Status JSON attrs: `text`, `color` (matches `leafText`'s `text` key).
- Task item HTML is `<li data-type="taskItem" data-checked="true|false"><p>..</p></li>` (no label/input), which round-trips through the sanitiser; the editor node view draws the checkbox.
- Tables: `getHTML()` emits `<table style><colgroup><col style>`. The sanitiser strips style/colgroup, so column resize widths persist only via `colwidth` on cells (admitted).
- Expand parses `details[data-type=expand] > summary` and `> div`; the `open` attribute is never emitted. Collapsed state in the editor is DOM-only and not saved.
- Heading `id` attribute is generated by a ProseMirror `appendTransaction` plugin: slug of text, starts with a letter, at most 80 chars, deduplicated as `-1`, `-2`. Content that has never been edited has no ids in HTML; `ContentView` and `extractHeadings` generate the same ids on the fly.
- Image `src` stored in the node is whatever `uploadImage` returns. The edit page returns `attachmentContentPath(id)` (`/api/v1/attachments/<id>/content`) to keep the contract; the editor node view and `ContentView` resolve it with `apiUrl` for display. Deviation from the brief, which said `apiUrl(...)`: identical when `NEXT_PUBLIC_API_BASE_URL` is empty, and avoids absolute origins in stored HTML otherwise.
- Inline image width is stored as `width="25%|50%|75%|100%"` (admitted by the sanitiser pattern).
- Links: `target="_blank" rel="noopener noreferrer"`; only http(s), mailto, relative and `#` hrefs are accepted by the link popover.

## Edit page behaviour and decisions

- Route is `/spaces/[key]/pages/[id]/edit`. It no longer wraps itself in `AppShell`; it relies on `app/spaces/[key]/layout.tsx` (C2) rendering `AppShell` (auth gate, TopBar, sidebar). The sticky action bar sits at `top-14` under the sticky TopBar and `--kn-toolbar-top` stays 105px (56 TopBar + 49 action bar), so both assume the layout's shell keeps a sticky 56px TopBar.
- Queries use keys `["editor","page",id]` and `["editor","draft",id]` with `staleTime: Infinity`, `gcTime: 0`, `refetchOnMount: "always"`, so every visit loads fresh data and background refetches never clobber local state. After publishing it invalidates every other query (`qc.invalidateQueries` excluding `"editor"` keys) so the view page and sidebar tree refetch.
- `base_version`: a fresh edit uses `page.version`; a resumed draft keeps `draft.base_version`, so a draft created before someone else published triggers a 409 on publish (the banner also warns). Discarding the draft resets to `page.version`.
- Autosave: 1500 ms debounce after any title or content change; in-flight saves are serialised and re-queued; status shows Saving..., Saved HH:mm, Unsaved changes, Save failed with Retry. Closing flushes the draft first and asks to confirm if the flush fails; component unmount also flushes. `beforeunload` guard while unsaved (browser reload/close only; in-app navigation via other links relies on the unmount flush).
- Draft title falls back to the page title or "Untitled" when the title field is empty; Publish is disabled until the title is non-empty.
- Presence: heartbeat on mount and every 15 s; the banner lists editors other than the current user (filtered by `useMe` id because the contract does not say whether the caller is included).
- Permission handling: 403 from page or draft, `my_role === "viewer"`, 404 and trashed pages each render an `EmptyState`.
- Dev server note: A2's CSP has no `unsafe-eval`, so `next dev` pages fail to hydrate (React dev uses eval). Verify with a production build (`npm run build && node .next/standalone/server.js` or `next start`).

## No new dependencies

Uses only pinned packages. Code highlighting imports `highlight.js/lib/languages/*` (already pinned) into a `createLowlight` subset: bash, css, go, java, js, json, markdown, python, rust, sql, ts, xml/html, yaml.

## Follow-up changes

- Alignment now round-trips via `data-align` (see contract notes).
- `AppShell` and the breadcrumb top slot were removed from the edit page; the page renders only its own content.

## Final review fixes (review-final-web P2-2 and P2-3)

- Trashed pages: `page.status === "trashed"` renders an `EmptyState` ("This page is in the trash. Restore it to edit.") with a "Back to page" link. During a session, a draft save or publish that returns 409 `page_trashed` calls `markTrashed`: autosave timers stop and no further saves are attempted, the editor and title become read-only (`editable={false}`, `readOnly`), Publish is disabled, the Save failed/Retry indicator is hidden, and a red banner explains the state and links back to the page. Close no longer tries to flush or prompts, and the unmount flush is skipped. The `beforeunload` guard still applies because the latest edits were not saved.
- Inline comments: the bubble menu Comment action (`onRequestComment`) opens a dialog with the quoted text and a compact `Editor` for the body. On submit it calls `createComment(pageId, { body_json, body_html, inline_anchor: { quoted_text, prefix, suffix } })`, then runs `setTextSelection({from,to}).setCommentMark(id)` on the main editor so the mark lands in the draft and autosaves/publishes with the page, and toasts "Comment added". If the text under the stored range changed, it toasts that the comment was saved but could not be highlighted. Comment queries (key containing "comment") are invalidated.
- `getSelectionContext(editor, {from,to})` (exported from `@/components/editor`) returns `{quoted_text, prefix, suffix}`; prefix and suffix are up to 32 characters of surrounding plain text (block boundaries become spaces).
- `InlineAnchor` in `lib/api/types.ts` only declares `quoted_text`; the page builds the anchor as a local `InlineAnchor & { prefix, suffix }` value so it type-checks without editing that file. Adding `prefix?: string; suffix?: string` there is a one-line follow-up for its owner.
- Verified with the mocked-API Playwright setup: selection, Comment bubble button, dialog, POST body (`quoted_text "beta"`, prefix "Alpha ", suffix " gamma delta"), mark in `getHTML()`, then a 409 `page_trashed` on autosave produced the read-only banner with Publish disabled and no Retry.
