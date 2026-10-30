package search

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"html"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	DefaultLimit      = 20
	MaxLimit          = 50
	MaxPage           = 10000
	QuickDefaultLimit = 8
	QuickMaxLimit     = 20
	MaxQueryLen       = 200
)

var (
	markStart       string
	markStop        string
	markDelimiter   string
	HeadlineOptions string
)

func init() {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	token := hex.EncodeToString(buf)
	markStart = "KNMS" + token + "E"
	markStop = "KNME" + token + "E"
	markDelimiter = "KNMD" + token + "E"
	HeadlineOptions = "MaxWords=30, MinWords=12, StartSel=" + markStart + ", StopSel=" + markStop + ", MaxFragments=2, FragmentDelimiter=" + markDelimiter
}

func SafeSnippet(raw string) string {
	out := html.EscapeString(raw)
	out = strings.ReplaceAll(out, markStart, "<mark>")
	out = strings.ReplaceAll(out, markStop, "</mark>")
	return strings.ReplaceAll(out, markDelimiter, " … ")
}

var (
	ErrEmptyQuery   = errors.New("search query is required")
	ErrQueryTooLong = errors.New("search query is too long")
)

type Query struct {
	SQL  string
	Args []any
}

type Params struct {
	Q        string
	SpaceKey string
	Label    string
	AuthorID uuid.UUID
	Page     int
	Limit    int
	UserID   uuid.UUID
	IsAdmin  bool
}

type Statements struct {
	Params Params
	List   Query
	Count  Query
}

type QuickParams struct {
	Q       string
	Limit   int
	UserID  uuid.UUID
	IsAdmin bool
}

type argList struct {
	args []any
}

func (a *argList) add(v any) string {
	a.args = append(a.args, v)
	return "$" + strconv.Itoa(len(a.args))
}

func (a *argList) snapshot() []any {
	out := make([]any, len(a.args))
	copy(out, a.args)
	return out
}

func AccessClause(spaceAlias, userPlaceholder, adminPlaceholder string) string {
	return spaceAlias + ".deleted_at IS NULL AND (" + adminPlaceholder + "::boolean" +
		" OR " + spaceAlias + ".visibility = 'internal'" +
		" OR EXISTS (SELECT 1 FROM space_members m WHERE m.space_id = " + spaceAlias + ".id AND m.user_id = " + userPlaceholder + "::uuid))"
}

func EscapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func clamp(v, def, min, max int) int {
	if v == 0 {
		return def
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func cleanQuery(q string) (string, error) {
	q = strings.TrimSpace(q)
	if utf8.RuneCountInString(q) > MaxQueryLen {
		return "", ErrQueryTooLong
	}
	return q, nil
}

func (p Params) Normalize() (Params, error) {
	q, err := cleanQuery(p.Q)
	if err != nil {
		return p, err
	}
	if q == "" {
		return p, ErrEmptyQuery
	}
	p.Q = q
	p.SpaceKey = strings.ToUpper(strings.TrimSpace(p.SpaceKey))
	p.Label = strings.ToLower(strings.TrimSpace(p.Label))
	p.Page = clamp(p.Page, 1, 1, MaxPage)
	p.Limit = clamp(p.Limit, DefaultLimit, 1, MaxLimit)
	return p, nil
}

func Build(p Params) (Statements, error) {
	p, err := p.Normalize()
	if err != nil {
		return Statements{}, err
	}
	var a argList
	tsq := a.add(p.Q)
	user := a.add(p.UserID)
	admin := a.add(p.IsAdmin)

	where := []string{
		"p.status = 'published'",
		"p.search_vector @@ q.tsq",
		AccessClause("s", user, admin),
	}
	if p.SpaceKey != "" {
		where = append(where, "s.key = "+a.add(p.SpaceKey))
	}
	if p.Label != "" {
		where = append(where,
			"EXISTS (SELECT 1 FROM page_labels pl JOIN labels l ON l.id = pl.label_id WHERE pl.page_id = p.id AND l.name = "+a.add(p.Label)+")")
	}
	if p.AuthorID != uuid.Nil {
		where = append(where, "p.updated_by = "+a.add(p.AuthorID))
	}
	filter := strings.Join(where, " AND ")
	withQ := "WITH q AS (SELECT websearch_to_tsquery('english', " + tsq + "::text) AS tsq)"

	count := Query{
		SQL:  withQ + " SELECT count(*) FROM pages p JOIN spaces s ON s.id = p.space_id CROSS JOIN q WHERE " + filter,
		Args: a.snapshot(),
	}

	limit := a.add(p.Limit)
	offset := a.add((p.Page - 1) * p.Limit)
	list := Query{
		SQL: withQ + `, ranked AS (
	SELECT p.id, p.title, p.slug, p.parent_id, p.position, p.status, p.content_text, p.updated_at, p.updated_by,
	       s.key AS space_key, ts_rank_cd(p.search_vector, q.tsq) AS score
	FROM pages p
	JOIN spaces s ON s.id = p.space_id
	CROSS JOIN q
	WHERE ` + filter + `
	ORDER BY score DESC, p.updated_at DESC, p.id
	LIMIT ` + limit + ` OFFSET ` + offset + `
)
SELECT r.id, r.title, r.slug, r.parent_id, r.position, r.status,
       EXISTS (SELECT 1 FROM pages c WHERE c.parent_id = r.id AND c.status = 'published'),
       r.space_key,
       ts_headline('english', r.content_text, q.tsq, '` + HeadlineOptions + `'),
       r.updated_at, u.id, u.display_name, u.email
FROM ranked r
CROSS JOIN q
LEFT JOIN users u ON u.id = r.updated_by
ORDER BY r.score DESC, r.updated_at DESC, r.id`,
		Args: a.snapshot(),
	}
	return Statements{Params: p, List: list, Count: count}, nil
}

func (p QuickParams) Normalize() (QuickParams, error) {
	q, err := cleanQuery(p.Q)
	if err != nil {
		return p, err
	}
	p.Q = q
	p.Limit = clamp(p.Limit, QuickDefaultLimit, 1, QuickMaxLimit)
	return p, nil
}

func BuildQuick(p QuickParams) (Query, error) {
	p, err := p.Normalize()
	if err != nil {
		return Query{}, err
	}
	var a argList
	user := a.add(p.UserID)
	admin := a.add(p.IsAdmin)
	where := []string{
		"(p.status = 'published' OR (p.status = 'draft' AND p.created_by = " + user + "::uuid))",
		AccessClause("s", user, admin),
	}
	order := "p.updated_at DESC, p.id"
	if p.Q != "" {
		escaped := EscapeLike(p.Q)
		contains := a.add("%" + escaped + "%")
		prefix := a.add(escaped + "%")
		where = append(where, "p.title ILIKE "+contains)
		order = "(p.title ILIKE " + prefix + ") DESC, " + order
	}
	limit := a.add(p.Limit)
	return Query{
		SQL: `SELECT p.id, p.title, p.slug, p.parent_id, p.position, p.status,
       EXISTS (SELECT 1 FROM pages c WHERE c.parent_id = p.id AND c.status = 'published'),
       s.key, s.name
FROM pages p
JOIN spaces s ON s.id = p.space_id
WHERE ` + strings.Join(where, " AND ") + `
ORDER BY ` + order + `
LIMIT ` + limit,
		Args: a.snapshot(),
	}, nil
}
