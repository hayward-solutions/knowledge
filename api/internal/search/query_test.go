package search_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/search"
)

func TestNormalizeParams(t *testing.T) {
	cases := []struct {
		name  string
		in    search.Params
		page  int
		limit int
	}{
		{"defaults", search.Params{Q: "go"}, 1, 20},
		{"negative page", search.Params{Q: "go", Page: -3, Limit: -1}, 1, 1},
		{"limit capped", search.Params{Q: "go", Page: 2, Limit: 500}, 2, 50},
		{"page capped", search.Params{Q: "go", Page: 99999999}, search.MaxPage, 20},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := c.in.Normalize()
			if err != nil {
				t.Fatal(err)
			}
			if got.Page != c.page || got.Limit != c.limit {
				t.Fatalf("page=%d limit=%d, want %d %d", got.Page, got.Limit, c.page, c.limit)
			}
		})
	}
}

func TestNormalizeTrimsAndCases(t *testing.T) {
	got, err := search.Params{Q: "  hello world  ", SpaceKey: " eng ", Label: " Runbook "}.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if got.Q != "hello world" || got.SpaceKey != "ENG" || got.Label != "runbook" {
		t.Fatalf("got %+v", got)
	}
}

func TestEmptyAndLongQuery(t *testing.T) {
	for _, q := range []string{"", "   ", "\t\n"} {
		if _, err := search.Build(search.Params{Q: q}); !errors.Is(err, search.ErrEmptyQuery) {
			t.Errorf("q=%q: %v", q, err)
		}
	}
	if _, err := search.Build(search.Params{Q: strings.Repeat("a", search.MaxQueryLen+1)}); !errors.Is(err, search.ErrQueryTooLong) {
		t.Fatalf("long query: %v", err)
	}
	if _, err := search.Build(search.Params{Q: strings.Repeat("a", search.MaxQueryLen)}); err != nil {
		t.Fatalf("max length query: %v", err)
	}
}

func TestBuildParameterises(t *testing.T) {
	user := uuid.Must(uuid.NewV7())
	author := uuid.Must(uuid.NewV7())
	hostile := "x'); DROP TABLE pages;--"
	st, err := search.Build(search.Params{
		Q: hostile, SpaceKey: "eng", Label: "ops", AuthorID: author,
		Page: 3, Limit: 10, UserID: user, IsAdmin: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{st.List.SQL, st.Count.SQL} {
		if strings.Contains(sql, "DROP TABLE") || strings.Contains(sql, "ENG") || strings.Contains(sql, "ops") {
			t.Fatalf("user input leaked into SQL: %s", sql)
		}
		for _, want := range []string{
			"websearch_to_tsquery('english', $1::text)",
			"p.status = 'published'",
			"s.deleted_at IS NULL",
			"s.key = $4",
			"l.name = $5",
			"p.updated_by = $6",
		} {
			if !strings.Contains(sql, want) {
				t.Errorf("missing %q in %s", want, sql)
			}
		}
	}
	wantCount := []any{hostile, user, true, "ENG", "ops", author}
	if len(st.Count.Args) != len(wantCount) {
		t.Fatalf("count args %v", st.Count.Args)
	}
	for i, w := range wantCount {
		if st.Count.Args[i] != w {
			t.Errorf("count arg %d = %v, want %v", i, st.Count.Args[i], w)
		}
	}
	if len(st.List.Args) != len(wantCount)+2 {
		t.Fatalf("list args %v", st.List.Args)
	}
	if st.List.Args[6] != 10 || st.List.Args[7] != 20 {
		t.Fatalf("limit/offset = %v %v", st.List.Args[6], st.List.Args[7])
	}
	if !strings.Contains(st.List.SQL, "LIMIT $7 OFFSET $8") {
		t.Fatalf("limit placeholders: %s", st.List.SQL)
	}
	if strings.Contains(st.Count.SQL, "LIMIT") {
		t.Fatalf("count query must not paginate: %s", st.Count.SQL)
	}
}

func TestBuildOmitsUnusedFilters(t *testing.T) {
	st, err := search.Build(search.Params{Q: "go", UserID: uuid.Must(uuid.NewV7())})
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"s.key =", "l.name", "p.updated_by ="} {
		if strings.Contains(st.List.SQL, bad) {
			t.Errorf("unexpected %q", bad)
		}
	}
	if len(st.Count.Args) != 3 || len(st.List.Args) != 5 {
		t.Fatalf("args %d %d", len(st.Count.Args), len(st.List.Args))
	}
}

func TestBuildRankingAndHeadline(t *testing.T) {
	st, err := search.Build(search.Params{Q: "go"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"ts_rank_cd(p.search_vector, q.tsq)",
		"ts_headline('english', r.content_text, q.tsq, '" + search.HeadlineOptions + "')",
		"ORDER BY r.score DESC",
	} {
		if !strings.Contains(st.List.SQL, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(st.Count.SQL, "ts_headline") {
		t.Fatal("count query must not compute headlines")
	}
}

func TestAccessClause(t *testing.T) {
	got := search.AccessClause("s", "$1", "$2")
	for _, want := range []string{"s.deleted_at IS NULL", "$2::boolean", "s.visibility = 'internal'", "m.user_id = $1::uuid"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestEscapeLike(t *testing.T) {
	if got := search.EscapeLike(`50%_off\`); got != `50\%\_off\\` {
		t.Fatalf("got %q", got)
	}
}

func TestBuildQuick(t *testing.T) {
	user := uuid.Must(uuid.NewV7())
	q, err := search.BuildQuick(search.QuickParams{Q: " 100%_x ", UserID: user})
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Args) != 5 {
		t.Fatalf("args %v", q.Args)
	}
	if q.Args[2] != `%100\%\_x%` || q.Args[3] != `100\%\_x%` {
		t.Fatalf("patterns %v %v", q.Args[2], q.Args[3])
	}
	if q.Args[4] != search.QuickDefaultLimit {
		t.Fatalf("limit %v", q.Args[4])
	}
	for _, want := range []string{"p.title ILIKE $3", "(p.title ILIKE $4) DESC", "p.created_by = $1::uuid", "LIMIT $5"} {
		if !strings.Contains(q.SQL, want) {
			t.Errorf("missing %q in %s", want, q.SQL)
		}
	}
}

func TestBuildQuickEmptyAndLimits(t *testing.T) {
	q, err := search.BuildQuick(search.QuickParams{Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(q.SQL, "ILIKE") {
		t.Fatalf("empty query must not filter titles: %s", q.SQL)
	}
	if q.Args[len(q.Args)-1] != search.QuickMaxLimit {
		t.Fatalf("limit %v", q.Args[len(q.Args)-1])
	}
	if _, err := search.BuildQuick(search.QuickParams{Q: strings.Repeat("a", search.MaxQueryLen+1)}); !errors.Is(err, search.ErrQueryTooLong) {
		t.Fatalf("long: %v", err)
	}
}

func TestSafeSnippet(t *testing.T) {
	opts := search.HeadlineOptions
	field := func(name string) string {
		i := strings.Index(opts, name+"=")
		rest := opts[i+len(name)+1:]
		if j := strings.Index(rest, ","); j >= 0 {
			rest = rest[:j]
		}
		return rest
	}
	start, stop, delim := field("StartSel"), field("StopSel"), field("FragmentDelimiter")
	if start == stop || start == delim || strings.ContainsAny(start+stop+delim, "<>&\"' ,") {
		t.Fatalf("bad sentinels %q %q %q", start, stop, delim)
	}
	raw := "<img onerror=alert(1)> & " + start + "hit" + stop + delim + "<script>x</script>"
	got := search.SafeSnippet(raw)
	want := "&lt;img onerror=alert(1)&gt; &amp; <mark>hit</mark> … &lt;script&gt;x&lt;/script&gt;"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if strings.Contains(got, "<img") || strings.Contains(got, "<script") {
		t.Fatal("raw tag survived")
	}
}
