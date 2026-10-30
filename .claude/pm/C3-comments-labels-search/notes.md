# C3 comments, labels, search UI notes

Gate (clean): `cd web && npm run lint && npm run typecheck && NEXT_PUBLIC_API_BASE_URL=http://localhost:8080 npm run build`. Build routes added: `/search`, `/spaces/[key]/labels`, `/spaces/[key]/labels/[name]`.

Runtime check: real Go API (built from `api/`, local Postgres container, no object storage) plus a production build of `web/` copied to a scratch dir with a throwaway harness route that rendered `ContentView` + `PageLabels` + `PageComments` (the harness was deleted; nothing of it is in the repo). Verified in the browser pane: inline comments created via the API anchor in the rendered page (including a quote crossing a paragraph and list items), orphan note, mark click selects thread, resolve removes the highlight and moves the thread under "Show N resolved", select text -> floating Comment button -> dialog -> POST -> new mark and thread, reply, edit, delete confirm, label add with free entry and normalisation (`Design Docs` -> `design-docs`), label index and label pages, search page with label filter and snippet marks, quick search with `/` shortcut and arrow keys, 375px layout without horizontal scroll, dark mode. Not exercised: author filter dropdown against several users, page_trashed 409 toast, mentions in comment bodies, light-mode visuals.

## Files

- `web/components/comments/`: `PageComments.tsx`, `InlineComments.tsx`, `CommentThread.tsx`, `CommentItem.tsx`, `CommentComposer.tsx`, `InlineCommentDialog.tsx`, `SelectionCommentButton.tsx`, `useCommentActions.ts`, `useInlineAnchors.ts`, `anchors.ts`, `commentUtils.ts`.
- `web/components/labels/`: `PageLabels.tsx`, `LabelPicker.tsx`, `normalize.ts`, `useDebouncedValue.ts`.
- `web/components/search/`: `QuickSearch.tsx` (named export `QuickSearch` as the TopBar imports it, plus default export), `AuthorFilter.tsx`.
- `web/app/search/page.tsx`, `web/app/spaces/[key]/labels/page.tsx`, `web/app/spaces/[key]/labels/[name]/page.tsx`.
- No changes to `lib/api/*`; the existing exports were sufficient.

## Component props

```tsx
<PageComments page={PageDetail} canComment canEdit contentRef?: RefObject<HTMLElement> activeCommentId?: string onActiveCommentChange?: (id: string | null) => void />
<PageLabels pageId canEdit initial?: Label[] />
<LabelPicker selected?: string[] onSelect(name) onClose? placeholder? disabled? autoFocus? allowCreate?=true closeOnSelect?=false ariaLabel? className? />
<AuthorFilter value={userId | ""} onChange(userId | "") />
```

## Behaviour

- Query key `["comments", pageId]` (`commentsKey`), every mutation invalidates it and toasts failures. A 409 (`page_trashed`) toasts "This page is in the trash...". When `page.status === "trashed"` the composer and all actions are hidden and a notice is shown.
- "Comments (N)" counts non-deleted comments (including replies and resolved) in page-level threads only; inline threads have their own "Inline comments (M)" count of open threads. Deleted comments that still have replies render "This comment was deleted."
- Replies are rendered flat under their root at one indent level, with a "Replying to X" line when the parent is not the root. Reply is offered on depth 0 and 1 only (API cap is depth 2). Resolve/Reopen is on the root only. Edit is author only. Delete is shown to the author or when `canEdit`, as briefed; the API allows non-authors only for space admins, so an editor who is not admin gets a 403 toast.
- Resolved threads (page and inline) sit under a "Show N resolved" toggle; a resolved inline thread stays visible while it is the active one.
- Layout: with inline threads present, `lg` and up shows page comments left and a sticky scrolling inline panel on the right (340px); below `lg` the inline list stacks under the page comments. `PageComments` renders `section#comments`; each comment has `id="comment-<id>"` and a `#comment-<id>` hash scrolls to it (and selects it when inline).
- Active selection: `PageComments` keeps its own `active` id, synced from `activeCommentId` and reported through `onActiveCommentChange`. Clicking a mark scrolls its thread to the centre; clicking the quote of a thread scrolls the first mark into view; clicking elsewhere in a thread only selects it. Works without the page passing either prop.

## Inline anchoring

- Threads with `inline_anchor` and not resolved/deleted are anchored in the DOM under `contentRef.current` by `useInlineAnchors` (hook used inside `InlineComments`, which is always mounted so cleanup also runs when the last inline thread disappears).
- If `span[data-type=comment][data-id=<id>]` already exists (comment made in the editor) nothing is done. Otherwise `anchorQuote` builds a whitespace-normalised text corpus by walking text nodes (skipping `pre`, the TOC, buttons), inserting a virtual space at block and `<br>` boundaries so quotes that cross paragraphs and list items match, finds the first occurrence of the normalised quote, and wraps each covered text-node segment in `span.kn-comment-mark[data-type=comment][data-id][data-client-anchor]` (splitting the first and last text nodes). Reusing the existing editor.css classes gives identical highlight and `.is-active` styling.
- Client anchors are removed again (spans unwrapped, text normalised) when the thread is resolved, deleted or no longer in the list. A `MutationObserver` (reconnected around our own edits, 60 ms debounce) re-applies anchors when `ContentView` replaces its HTML (page refetch) or decorates. If `contentRef.current` is not mounted yet it retries every 250 ms for 10 s.
- `is-active` is toggled on all marks for the active id (same as `ContentView.activeCommentId`, so both agree). A delegated click listener on the container also reports mark clicks, so it works even when the page does not wire `onCommentMarkClick`.
- Orphans: after each pass, an open inline thread with no mark in the DOM shows "The commented text is no longer present on this page." Resolved threads are not checked.
- Creating from the view: `SelectionCommentButton` (portal, `position: fixed`) shows a Comment button near a non-collapsed selection fully inside `contentRef` (not in code blocks, the TOC, buttons or inputs); hidden while the mouse is down; below the selection on screens under 640px or near the top. Clicking opens `InlineCommentDialog` (shared `Modal` with the quote and a compact editor, Cmd/Ctrl+Enter submits). The quote is whitespace-normalised and capped at 2000 characters before `POST /pages/{id}/comments` with `inline_anchor {quoted_text}`; the new thread becomes active. Only the first occurrence of repeated text is anchored (as briefed).
- Resolved threads whose marks came from the editor stay highlighted in the content (the content HTML is authoritative and editor.css has no resolved style).

## Labels

- `PageLabels`: query `["page-labels", pageId]` seeded with `initial`; chips link to `/spaces/<key>/labels/<name>` using the `key` route param (non-links when not under `/spaces/[key]`). Editors get remove buttons and an "Add label" control opening `LabelPicker`. Each change PUTs the full set and stores the response; max 20 labels checked client-side. After a save it invalidates `["labels"]`, `["space-labels"]`, `["label-pages"]`. It does not touch the page view's own page query, so the page agent should not rely on `PageDetail.labels` being refreshed by it (the chips own their state).
- `LabelPicker`: combobox with debounced `GET /labels?q=`, arrow/enter/escape/comma, "Create <name>" for free entry, normalisation `trim, lower, whitespace -> "-"` and validation `^[a-z0-9._-]{1,50}$` mirroring the API. The label filter on the search page reuses it with `closeOnSelect`.
- Note from B4: `GET /labels` only returns labels with an accessible page count above 0, so a label that exists only on a draft page is not suggested.

## Search

- `QuickSearch` (TopBar): debounced `GET /search/quick`, empty query shows recently updated pages, space chips, Draft badge, arrow up/down, Enter on a selected hit navigates, Enter with nothing selected goes to `/search?q=`, last row is "Search for ...", Escape closes then blurs. `/` (when not typing in a field) and Cmd/Ctrl+K focus and select the input. Dropdown is full width under the bar below `sm`.
- `/search`: inside `AppShell`; URL params `q, space, label, author (user id), page`, all via `router.push`. Space select from `GET /spaces` (query key `["spaces"]`, shared with the sidebar), label picker/chip, author autocomplete (`GET /users?q=`, chip name from `GET /users/{id}`), clear filters, page size 20, windowed pagination from `SearchResult`, empty and error states. Snippets go through `dangerouslySetInnerHTML` after a defensive `safeSnippet` that escapes any `<` not starting `<mark>` or `</mark>`; mark styling via Tailwind arbitrary variants.
- Label pages (`/spaces/[key]/labels`, `/spaces/[key]/labels/[name]`) are plain client pages without `AppShell`; they rely on `app/spaces/[key]/layout.tsx` for the shell. They render their own small breadcrumb line.

## Deviations and caveats

- `useDebouncedValue` lives in `components/labels` and is imported by the search components.
- `PageComments` shows a hint ("select text in the page to leave an inline comment") only when `contentRef` is passed. Without `contentRef` inline anchoring, the selection button and orphan notes are all off and inline threads still list with their quotes.
- The page view must keep the element behind `contentRef` mounted while `PageComments` is mounted; client anchors are DOM mutations inside the `ContentView` container, which React does not diff (it only resets them when `content_html` changes, which the observer handles).
- The harness exercised a real API with the admin user as the only author; edit-own and delete-others rules were not tested across users.
