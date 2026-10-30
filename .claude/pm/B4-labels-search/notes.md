# B4 labels and search: notes

## What was built

- `api/migrations/0040_labels.sql`: `labels(id, name unique CHECK (name = lower(name)), created_at)`, `page_labels(page_id, label_id, pk)` with cascade on both FKs, `idx_page_labels_label(label_id)`. Up and Down. The GIN index on `pages.search_vector` already existed in 0003.
- `internal/search/query.go` (+ `query_test.go`): pure SQL building and parameter handling. `Params.Normalize`, `Build(Params) (Statements{Params, List, Count}, error)`, `QuickParams.Normalize`, `BuildQuick`, `AccessClause(spaceAlias, userPlaceholder, adminPlaceholder)`, `EscapeLike`, `HeadlineOptions`, `ErrEmptyQuery`, `ErrQueryTooLong` (query capped at 200 runes), limits/defaults constants. User input only ever travels as bind parameters.
- `internal/store/labels.go`, `internal/store/search.go` (+ `labels_test.go`, `search_test.go`).
- `internal/http/handlers/labels.go`, `search.go` (+ tests). `LabelHandler` and `SearchHandler` still embed `Deps` and expose `Routes`; all routes are flat `r.With(...)` registrations.

## Routes (all under /api/v1)

| Route | Middleware |
|---|---|
| `GET /labels?q=&limit=` | `RequireAuth` |
| `GET /pages/{pageID}/labels` | `SpaceByPage(PermViewSpace)` |
| `PUT /pages/{pageID}/labels` | `SpaceByPage(PermManageLabels)` |
| `GET /spaces/{key}/labels` | `SpaceByKey(PermViewSpace)` |
| `GET /spaces/{key}/labels/{name}/pages` | `SpaceByKey(PermViewSpace)` |
| `GET /search?q=&space=&label=&author=&page=&limit=` | `RequireAuth` |
| `GET /search/quick?q=&limit=` | `RequireAuth` |

## Exported helpers for the integration pass

```
(*Store).ListPageLabels(ctx, pageID) ([]domain.Label, error)                      // sorted by name, never nil
(*Store).ListLabelsForPages(ctx, pageIDs []uuid.UUID) (map[uuid.UUID][]domain.Label, error)  // pages without labels are absent
(*Store).SetPageLabels(ctx, pageID, names []string) ([]domain.Label, error)       // names must already be normalised; ErrNotFound for an unknown page
(*Store).ListLabels(ctx, userID, isAdmin, prefix, limit) ([]store.LabelWithCount, error)
(*Store).ListSpaceLabels(ctx, spaceID) ([]store.LabelWithCount, error)
(*Store).ListSpaceLabelPages(ctx, spaceID, name) ([]store.LabelPageSummary, error)
(*Store).GetPageLabelGate(ctx, pageID) (domain.PageStatus, createdBy uuid.UUID, error)
(*Store).SearchPages(ctx, search.Params) (store.SearchResult, error)
(*Store).QuickSearchPages(ctx, search.QuickParams) ([]store.QuickSearchHit, error)
```
`domain.Label` has no `page_count`, so `GET /pages/{id}/labels` and `PUT` return `{id, name, created_at}` (the web type marks `page_count` optional). Label list endpoints that count return `store.LabelWithCount` (`domain.Label` embedded plus `page_count`). `GET /pages/{id}` can drop `ListPageLabels` straight into its `labels` field.

## Behaviour worth knowing

- Access rule (shared by labels and search via `search.AccessClause`): space not soft-deleted AND (caller is system admin OR space is `internal` OR caller is a member). Archived spaces are included. Soft-deleted spaces are excluded even for system admins in `/search`, `/search/quick` and `/labels`.
- Label name normalisation (`labelNormalize` in handlers/labels.go): lower-case, whitespace runs become a single `-`, then only `[a-z0-9-_.]`, 1-50 chars, otherwise 400. Duplicates collapse after normalisation; more than 20 distinct labels is a 400; a missing or null `labels` field is a 400; `[]` clears the set. A failed PUT changes nothing. Labels are created on demand in a transaction (`INSERT ... ON CONFLICT (name)`), names sorted before insert to avoid lock-order deadlocks. Orphaned labels are not deleted (an admin labels page can manage them later).
- `/spaces/{key}/labels/{name}/pages`: the name is URL-unescaped and normalised; an invalid or unknown name returns `[]` with 200, not an error. Ordered by title case-insensitively.
- Quick search: `title ILIKE '%q%'` with LIKE wildcards in `q` escaped; ordering is prefix matches first, then `updated_at DESC`. Caller's own `draft` pages in accessible spaces are included (their `status` is `draft`); other users' drafts and trashed pages never are. An empty `q` is allowed on `/search/quick` and returns the most recently updated pages (useful for the page-link picker); `/search` with an empty `q` is 400.
- Search ranking: `ts_rank_cd(search_vector, websearch_to_tsquery('english', q))`, then `updated_at DESC`, then id. Title is weight A so title matches outrank body matches. The page of results is chosen in a CTE first and `ts_headline` is computed only for those rows. A query that reduces to nothing (stop words, punctuation) returns zero hits rather than an error.
- `has_children` in every PageSummary here means "has at least one published child".
- Author filter is `pages.updated_by` (matches the `author` shown in each hit). An unparsable `author` is a 400. `space` is upper-cased, `label` lower-cased.
- `page` is clamped to 1-10000, `limit` to 1-50 (default 20) for search, 1-20 (default 8) for quick, 1-50 (default 20) for `/labels`.

## Deviations and decisions

1. `GET /labels` omits labels whose accessible `page_count` is 0. Otherwise the global autocomplete would reveal label names that exist only in spaces the caller cannot see (and orphaned labels). Consequence: a brand-new label that is only on a draft page is not suggested until a page carrying it is published.
2. `PUT /pages/{id}/labels` applies the same draft-visibility rule as GET (404 unless creator or space/system admin). The brief only mentioned GET; leaving PUT open would have let any editor read another user's draft labels from the response.
3. Trashed pages are still readable via `GET /pages/{id}/labels` (only drafts are gated), matching the brief.
4. `store` type names are prefixed (`LabelPageSummary`, `SearchPageSummary`) so they do not collide with the pages/spaces agents' types.
5. Query length is capped at 200 characters (400) to bound tsquery cost; not in the brief.

## Snippets are HTML-safe

`ts_headline` is called with per-process random sentinel markers (`search.HeadlineOptions`, 128 bits of crypto/rand hex, generated in `init`) for StartSel, StopSel and FragmentDelimiter, so page text cannot contain them. `store.SearchPages` passes every headline through `search.SafeSnippet`: `html.EscapeString` on the whole string, then the sentinels are swapped for `<mark>`, `</mark>` and ` … `. The `snippet` field therefore contains no markup other than `<mark>` and `</mark>`, and the web can render it as HTML directly (for example `dangerouslySetInnerHTML`). Escaped entities (`&lt;`, `&amp;`) appear literally in the string and render as the original characters. Proven by `TestSafeSnippet` (unit) and `TestSearchPagesSnippetIsHTMLSafe` (a page whose text contains `<img onerror=alert(1)>` and `<script>`).

## Tests

- `internal/search/query_test.go`: normalisation and clamping, empty/over-long queries, parameterisation (hostile input never reaches SQL text), argument ordering for list vs count, absence of unused filters, ranking/headline fragments, access clause, LIKE escaping, quick-search patterns.
- `store/labels_test.go`, `store/search_test.go`: replace semantics, global label reuse, cascade, CHECK constraint, bulk map, count scoping (private/draft/trashed/deleted), prefix and escaping, space labels and page ordering, access matrix (outsider, member, system admin, deleted, archived), every filter, title-over-body ranking, `<mark>` snippets, websearch syntax (negation, phrase, or, junk input), pagination totals with no repeats, quick-search ordering/drafts/wildcards.
- `handlers/labels_test.go`, `handlers/search_test.go`: end to end over the real router: auth, per-role permissions, archived read-only, draft visibility, normalisation and every 400, replace and clear semantics, response shapes against `web/lib/api/types.ts`, access rules, filters, pagination clamping, quick search.

## Gate output

```
$ cd api && gofmt -l . && go build ./... && go vet ./... && go test -count=1 ./...
(gofmt: no output)
ok  	github.com/hayward-solutions/knowledge/api/internal/auth	0.359s
ok  	github.com/hayward-solutions/knowledge/api/internal/bootstrap	1.601s
ok  	github.com/hayward-solutions/knowledge/api/internal/config	0.101s
ok  	github.com/hayward-solutions/knowledge/api/internal/content	0.112s
ok  	github.com/hayward-solutions/knowledge/api/internal/domain	0.103s
ok  	github.com/hayward-solutions/knowledge/api/internal/events	0.102s
ok  	github.com/hayward-solutions/knowledge/api/internal/http	0.127s
ok  	github.com/hayward-solutions/knowledge/api/internal/http/handlers	19.075s
ok  	github.com/hayward-solutions/knowledge/api/internal/http/middleware	1.920s
ok  	github.com/hayward-solutions/knowledge/api/internal/search	0.063s
ok  	github.com/hayward-solutions/knowledge/api/internal/storage	0.131s
ok  	github.com/hayward-solutions/knowledge/api/internal/store	5.688s
```

Re-run after the snippet remediation (gofmt clean, build and vet clean):

```
ok  	github.com/hayward-solutions/knowledge/api/internal/http/handlers	19.075s
ok  	github.com/hayward-solutions/knowledge/api/internal/search	0.163s
ok  	github.com/hayward-solutions/knowledge/api/internal/store	6.086s
(all other packages ok)
```

## Trashed pages (remediation)

- `PUT /pages/{pageID}/labels` on a trashed page returns 409 `{"error":"page is trashed","code":"page_trashed"}` and changes nothing. Order of checks: permission middleware, body validation (400), draft visibility (404), trashed (409).
- `GET /pages/{pageID}/labels` on a trashed page requires effective role editor or admin (system admin counts); viewers, including implicit viewers of internal spaces, get 404.
- Implemented in `labelGate(w, r, write bool)` in handlers/labels.go; covered by `TestPageLabelsTrashedPage`.
- Middleware now answers 404 (not 403) for non-members of private spaces; the label tests were updated to match.

Gate after this change: `gofmt -l .` clean, `go build ./...`, `go vet ./...`, and `go test -count=1 ./...` all ok (handlers 18.9s, store 6.2s, search 0.06s, middleware 2.0s).
