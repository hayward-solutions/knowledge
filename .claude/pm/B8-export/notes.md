# B8 export and import: notes

## Files

- `api/internal/content/markdown.go`: `ToMarkdown`, `ToMarkdownFromHTML`, `ToMarkdownTitle`, `ToStandaloneHTML`.
- `api/internal/content/import.go`: `FromMarkdown`, `TitleFromFilename`, `TitleFromFolder`.
- `api/internal/store/export.go`: `ListSpacePagesForExport`, `ImportPages`.
- `api/internal/http/handlers/export.go` (stub replaced, `ExportHandler{Deps}` kept) and `export_test.go`.
- Tests and fixtures: `content/markdown_test.go`, `content/import_test.go`, `content/testdata/{blocks.json,custom.json,contract.html}` with golden `blocks.md`, `custom.md`, `contract.md` (`go test ./internal/content -update` rewrites goldens).
- `api/internal/deps/deps.go` deleted (goldmark is imported for real). go.mod and go.sum untouched.

## Routes (flat, `/api/v1`)

| Route | Middleware | Result |
|---|---|---|
| `GET /pages/{pageID}/export?format=md\|html` | `SpaceByPage(PermViewSpace)` | file download, default format md, other values 400 |
| `GET /spaces/{key}/export` | `SpaceByKey(PermViewSpace)` | `application/zip`, `<KEY>-export.zip` |
| `POST /spaces/{key}/import` | `SpaceByKey(PermEditPages)` | 201 `{"pages": PageSummary[]}` |

Page export reuses the pages handler's `pageLoad`, so hidden drafts and viewer-visible trashed pages are 404 exactly as in `GET /pages/{id}`. Markdown is `# Title` (escaped) + body, filename `<slug>.md`, `text/markdown; charset=utf-8`. HTML is a standalone document (inline CSS for panels, status, task lists, tables, code, dark mode, print), sanitised again on output, served with `nosniff` and a sandboxing CSP. Non-ASCII slugs add `filename*=UTF-8''`.

## Markdown contract decisions

- JSON is preferred; if JSON is null, invalid, or renders empty while HTML is non-empty, the HTML path is used (same renderer: the DOM is converted into the same node tree).
- Marks: bold `**`, italic `*`, strike `~~`, highlight `==`, link `[t](href)` (href wrapped in `<...>` when it has spaces/parens), code with backtick-run-aware delimiters, underline and comment dropped (text kept). Adjacent runs with the same marks merge and leading/trailing whitespace is moved outside delimiters.
- Escaping: `\ ` * _ [ ] < ~` plus line-start `#`, `>`, `-`, `+`, `1.`, `---`. `|` is escaped inside table cells.
- Hard break is backslash-newline (`<br>` in table cells, a space in headings). Tables flatten cells to one line; first row is the header; colspan pads empty cells.
- Panel: blockquote whose first paragraph is prefixed `**Info:**`, `**Note:**`, `**Success:**`, `**Warning:**`, `**Error:**`. Expand: `<details><summary>Title</summary>\n\n...\n\n</details>` with the title HTML-escaped. Status `**[TEXT]**`, mention `@Label`, page link `[Title](href)`, image `![alt](src)` (width dropped), toc omitted.

## Import

- Goldmark with `extension.GFM`, default (safe) HTML options: raw HTML blocks/inline HTML are omitted, `javascript:` URLs dropped. The rendered HTML is re-parsed with x/net/html and rewritten: task lists become `<ul data-type="taskList"><li data-type="taskItem" data-checked=...><p>`, `<del>` becomes `<s>`, table cells get `<p>`, `==x==` becomes `<mark>`; then `SanitizeHTML` runs last.
- One safe exception to "no raw HTML": a custom HTMLBlock renderer re-synthesises only exact `<details><summary>T</summary>` / `</details>` blocks (as written by the exporter) into `<details data-type="expand"><summary>T</summary><div>` ... `</div></details>`, with the title escaped. Everything else in an HTML block is dropped (a whole HTML block is dropped, including text on the same lines).
- Title: first top-level H1 provided no other heading precedes it; it is removed from the body. `FromMarkdown` returns "" otherwise and the handler falls back to `TitleFromFilename` (extension stripped, `-`/`_` to spaces, first letter capitalised). Folder pages use `TitleFromFolder` (so `guides/` becomes "Guides"). Titles are clamped to 500 runes.
- Lossy on re-import: panels come back as blockquotes with a bold label, status/mentions/page links as plain text/links, ordered list start numbers (sanitiser does not admit `start`), image widths.

## Import handler behaviour

- Streams `r.MultipartReader()`; raw filename read from the part's `Content-Disposition` (Go's `Part.FileName()` strips directories). `\` is normalised to `/`; absolute paths, drive letters and `..` segments are rejected with 400. Test `TestImportFilenameFallbackTitle`/`TestImportNestedFolders` upload `guides/setup.md` and assert a "Guides" folder with "Setup" child; the web client must send `webkitRelativePath` as the multipart filename.
- Limits: at most 200 `files` parts (400), each at most `Config.AttachmentMaxBytes` (default 25 MiB when 0/unset; 413), whole body at most 8x that plus 1 MiB (413). Only `.md`/`.markdown` accepted; any other file is a 400 for the whole request (nothing is created). `parent_id` may come before or after the files; it must be a non-trashed visible page in the same space (404 otherwise, 400 if malformed). Default parent is the space home page.
- Nesting: folder pages are created once per path, published, empty body (unless that folder has `index.md`/`index.markdown`, which becomes the folder page content and, if it has an H1, the title). A root-level `index.md` is an ordinary page. Pages are created in upload order, positions appended.
- All pages are inserted in one transaction (`Store.ImportPages`, under the space advisory lock): status published, version 1, `content_json` NULL, `content_html` sanitised, `content_text` extracted, `page_versions` row with message `Imported from <filename as sent>` (folders: `Imported from folder <path>`). `PagePublished` (version 1, no mentions) is published for every created page, including folders, after commit. Response order is creation order (parents before children); `has_children` computed within the batch.

## Zip layout

Pages are the caller-independent set of published pages (drafts and trashed excluded; published pages whose parent is not exported become roots). Page with children: folder `<slug>/` containing `index.md` plus children; leaf: `<slug>.md`. Names deduplicated case-insensitively per directory with `-2`, `-3`; `readme` is reserved at the root and `index` inside folders. The space home page is normally the single root, so the tree starts at `<home-slug>/index.md`. `README.md` at the top is a nested link list. Whole zip is built from one query and written straight to the response with `archive/zip` (content for all pages is loaded in memory first).

## Gate

`cd api && gofmt -l . && go build ./... && go vet ./... && go test -count=1 ./...` passes (Docker running, no skips).
