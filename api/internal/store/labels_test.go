package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/store"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

func labelTestNames(ls []domain.Label) []string {
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = l.Name
	}
	return out
}

func labelTestCounts(ls []store.LabelWithCount) map[string]int {
	out := map[string]int{}
	for _, l := range ls {
		out[l.Name] = l.PageCount
	}
	return out
}

func labelTestEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func labelTestSetStatus(t *testing.T, s *store.Store, pageID uuid.UUID, status string) {
	t.Helper()
	if _, err := s.Pool.Exec(context.Background(), `UPDATE pages SET status = $2 WHERE id = $1`, pageID, status); err != nil {
		t.Fatal(err)
	}
}

func TestSetPageLabelsReplacesSet(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	u := testutil.CreateUser(t, s, "labels@example.test", "", false)
	space := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "LAB", CreatedBy: u.ID})
	page := testutil.CreatePage(t, s, space, u.ID, "Page")
	other := testutil.CreatePage(t, s, space, u.ID, "Other")

	got, err := s.SetPageLabels(ctx, page, []string{"beta", "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if !labelTestEqual(labelTestNames(got), []string{"alpha", "beta"}) {
		t.Fatalf("set returned %v", labelTestNames(got))
	}
	if _, err := s.SetPageLabels(ctx, other, []string{"alpha"}); err != nil {
		t.Fatal(err)
	}

	got, err = s.SetPageLabels(ctx, page, []string{"alpha", "gamma"})
	if err != nil {
		t.Fatal(err)
	}
	if !labelTestEqual(labelTestNames(got), []string{"alpha", "gamma"}) {
		t.Fatalf("replace returned %v", labelTestNames(got))
	}
	listed, err := s.ListPageLabels(ctx, page)
	if err != nil {
		t.Fatal(err)
	}
	if !labelTestEqual(labelTestNames(listed), []string{"alpha", "gamma"}) {
		t.Fatalf("listed %v", labelTestNames(listed))
	}
	otherLabels, _ := s.ListPageLabels(ctx, other)
	if !labelTestEqual(labelTestNames(otherLabels), []string{"alpha"}) {
		t.Fatalf("other page changed: %v", labelTestNames(otherLabels))
	}
	if listed[0].ID != otherLabels[0].ID {
		t.Fatal("same name must reuse the same global label row")
	}

	var total int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM labels`).Scan(&total); err != nil || total != 3 {
		t.Fatalf("labels rows = %d, %v", total, err)
	}

	cleared, err := s.SetPageLabels(ctx, page, []string{})
	if err != nil || len(cleared) != 0 {
		t.Fatalf("clear: %v %v", cleared, err)
	}
	listed, err = s.ListPageLabels(ctx, page)
	if err != nil || listed == nil || len(listed) != 0 {
		t.Fatalf("after clear: %v %v", listed, err)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM labels`).Scan(&total); err != nil || total != 3 {
		t.Fatalf("labels rows after clear = %d, %v", total, err)
	}
}

func TestSetPageLabelsUnknownPage(t *testing.T) {
	s := testutil.NewDB(t)
	if _, err := s.SetPageLabels(context.Background(), uuid.Must(uuid.NewV7()), []string{"x"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestLabelsCheckConstraint(t *testing.T) {
	s := testutil.NewDB(t)
	_, err := s.Pool.Exec(context.Background(), `INSERT INTO labels (id, name) VALUES ($1, 'Upper')`, uuid.Must(uuid.NewV7()))
	if err == nil {
		t.Fatal("uppercase label name must be rejected")
	}
}

func TestPageLabelsCascadeOnPageDelete(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	u := testutil.CreateUser(t, s, "cascade@example.test", "", false)
	space := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "CAS", CreatedBy: u.ID})
	page := testutil.CreatePage(t, s, space, u.ID, "Page")
	if _, err := s.SetPageLabels(ctx, page, []string{"x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `DELETE FROM pages WHERE id = $1`, page); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM page_labels`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("page_labels rows = %d, %v", n, err)
	}
}

func TestListLabelsForPages(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	u := testutil.CreateUser(t, s, "multi@example.test", "", false)
	space := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "MUL", CreatedBy: u.ID})
	a := testutil.CreatePage(t, s, space, u.ID, "A")
	b := testutil.CreatePage(t, s, space, u.ID, "B")
	c := testutil.CreatePage(t, s, space, u.ID, "C")
	if _, err := s.SetPageLabels(ctx, a, []string{"one", "two"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetPageLabels(ctx, b, []string{"two"}); err != nil {
		t.Fatal(err)
	}

	m, err := s.ListLabelsForPages(ctx, []uuid.UUID{a, b, c})
	if err != nil {
		t.Fatal(err)
	}
	if !labelTestEqual(labelTestNames(m[a]), []string{"one", "two"}) || !labelTestEqual(labelTestNames(m[b]), []string{"two"}) {
		t.Fatalf("map %v", m)
	}
	if _, ok := m[c]; ok {
		t.Fatal("page without labels must be absent from the map")
	}
	empty, err := s.ListLabelsForPages(ctx, nil)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty: %v %v", empty, err)
	}
}

func TestListLabelsAccessAndCounts(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	owner := testutil.CreateUser(t, s, "owner@example.test", "", false)
	member := testutil.CreateUser(t, s, "member@example.test", "", false)
	outsider := testutil.CreateUser(t, s, "outsider@example.test", "", false)
	admin := testutil.CreateUser(t, s, "admin@example.test", "", true)

	pub := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "PUB", CreatedBy: owner.ID})
	priv := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "PRV", Visibility: domain.VisibilityPrivate, CreatedBy: owner.ID})
	gone := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "DEL", Deleted: true, CreatedBy: owner.ID})
	testutil.AddMember(t, s, priv, member.ID, domain.RoleViewer)

	label := func(space uuid.UUID, title string, names ...string) uuid.UUID {
		id := testutil.CreatePage(t, s, space, owner.ID, title)
		if _, err := s.SetPageLabels(ctx, id, names); err != nil {
			t.Fatal(err)
		}
		return id
	}
	label(pub, "p1", "shared", "public-only")
	label(pub, "p2", "shared")
	label(priv, "q1", "shared", "secret")
	label(priv, "q2", "secret")
	label(gone, "g1", "ghost")
	draft := label(pub, "d1", "draft-only")
	labelTestSetStatus(t, s, draft, "draft")
	trashed := label(pub, "t1", "trash-only", "shared")
	labelTestSetStatus(t, s, trashed, "trashed")

	got, err := s.ListLabels(ctx, outsider.ID, false, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	counts := labelTestCounts(got)
	if len(counts) != 2 || counts["shared"] != 2 || counts["public-only"] != 1 {
		t.Fatalf("outsider counts %v", counts)
	}
	if got[0].Name != "shared" {
		t.Fatalf("order %v", got)
	}

	got, _ = s.ListLabels(ctx, member.ID, false, "", 20)
	counts = labelTestCounts(got)
	if counts["shared"] != 3 || counts["secret"] != 2 || counts["public-only"] != 1 || len(counts) != 3 {
		t.Fatalf("member counts %v", counts)
	}
	if got[0].Name != "shared" || got[1].Name != "secret" || got[2].Name != "public-only" {
		t.Fatalf("member order %v", got)
	}

	got, _ = s.ListLabels(ctx, admin.ID, true, "", 20)
	counts = labelTestCounts(got)
	if counts["shared"] != 3 || counts["secret"] != 2 || counts["ghost"] != 0 {
		t.Fatalf("admin counts %v", counts)
	}
	if _, ok := counts["ghost"]; ok {
		t.Fatal("deleted space labels must not be listed even for admins")
	}

	got, _ = s.ListLabels(ctx, member.ID, false, "SH", 20)
	if len(got) != 1 || got[0].Name != "shared" {
		t.Fatalf("prefix: %v", got)
	}
	got, _ = s.ListLabels(ctx, member.ID, false, "hared", 20)
	if len(got) != 0 {
		t.Fatalf("prefix must anchor at the start: %v", got)
	}
	got, _ = s.ListLabels(ctx, member.ID, false, "%", 20)
	if len(got) != 0 {
		t.Fatalf("wildcards must be escaped: %v", got)
	}
	got, _ = s.ListLabels(ctx, member.ID, false, "", 2)
	if len(got) != 2 {
		t.Fatalf("limit: %v", got)
	}
}

func TestListLabelsTieBreaksByName(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	u := testutil.CreateUser(t, s, "tie@example.test", "", false)
	space := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "TIE", CreatedBy: u.ID})
	page := testutil.CreatePage(t, s, space, u.ID, "P")
	if _, err := s.SetPageLabels(ctx, page, []string{"zeta", "alpha", "mid"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListLabels(ctx, u.ID, false, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Name != "alpha" || got[1].Name != "mid" || got[2].Name != "zeta" {
		t.Fatalf("order %v", got)
	}
}

func TestSpaceLabelsAndPages(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	u := testutil.CreateUser(t, s, "spacelabels@example.test", "", false)
	one := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "ONE", CreatedBy: u.ID})
	two := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "TWO", CreatedBy: u.ID})

	pb := testutil.CreatePage(t, s, one, u.ID, "banana")
	pa := testutil.CreatePage(t, s, one, u.ID, "Apple")
	pc := testutil.CreatePage(t, s, one, u.ID, "cherry")
	pd := testutil.CreatePage(t, s, one, u.ID, "Draft")
	px := testutil.CreatePage(t, s, two, u.ID, "elsewhere")
	for _, id := range []uuid.UUID{pb, pa, pd, px} {
		if _, err := s.SetPageLabels(ctx, id, []string{"fruit"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.SetPageLabels(ctx, pc, []string{"red"}); err != nil {
		t.Fatal(err)
	}
	labelTestSetStatus(t, s, pd, "draft")
	child := testutil.CreatePage(t, s, one, u.ID, "child")
	if _, err := s.Pool.Exec(ctx, `UPDATE pages SET parent_id = $2 WHERE id = $1`, child, pa); err != nil {
		t.Fatal(err)
	}

	labels, err := s.ListSpaceLabels(ctx, one)
	if err != nil {
		t.Fatal(err)
	}
	counts := labelTestCounts(labels)
	if len(counts) != 2 || counts["fruit"] != 2 || counts["red"] != 1 {
		t.Fatalf("counts %v", counts)
	}
	if labels[0].Name != "fruit" {
		t.Fatalf("order %v", labels)
	}

	pages, err := s.ListSpaceLabelPages(ctx, one, "fruit")
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 || pages[0].Title != "Apple" || pages[1].Title != "banana" {
		t.Fatalf("pages %+v", pages)
	}
	if !pages[0].HasChildren || pages[1].HasChildren {
		t.Fatalf("has_children %+v", pages)
	}
	if pages[0].Status != domain.PageStatusPublished {
		t.Fatalf("status %v", pages[0].Status)
	}
	none, err := s.ListSpaceLabelPages(ctx, one, "nope")
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("none: %v %v", none, err)
	}
}

func TestGetPageLabelGate(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	u := testutil.CreateUser(t, s, "gate@example.test", "", false)
	space := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "GAT", CreatedBy: u.ID})
	page := testutil.CreatePage(t, s, space, u.ID, "P")
	labelTestSetStatus(t, s, page, "draft")
	status, by, err := s.GetPageLabelGate(ctx, page)
	if err != nil || status != domain.PageStatusDraft || by != u.ID {
		t.Fatalf("got %v %v %v", status, by, err)
	}
	if _, _, err := s.GetPageLabelGate(ctx, uuid.Must(uuid.NewV7())); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}
