package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/store"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

func spaceTestPage(t *testing.T, s *store.Store, spaceID, createdBy uuid.UUID, title, status string, parent *uuid.UUID, position int) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	_, err := s.Pool.Exec(context.Background(),
		`INSERT INTO pages (id, space_id, parent_id, title, slug, position, status, created_by, updated_by, trashed_at, trashed_by)
		 VALUES ($1, $2, $3, $4, $4, $5, $6::text, $7::uuid, $7::uuid, CASE WHEN $6::text = 'trashed' THEN now() END, CASE WHEN $6::text = 'trashed' THEN $7::uuid END)`,
		id, spaceID, parent, title, position, status, createdBy)
	if err != nil {
		t.Fatalf("insert page %s: %v", title, err)
	}
	return id
}

func TestSpaceSlugFromTitle(t *testing.T) {
	cases := map[string]string{
		"Hello World":            "hello-world",
		"  Trim  Me  ":           "trim-me",
		"A -- B__C":              "a-b-c",
		"Engineering (2024)!":    "engineering-2024",
		"":                       "untitled",
		"!!!":                    "untitled",
		"Ünïcode Títle":          "ünïcode-títle",
		"--leading and trailing": "leading-and-trailing",
		"Edit":                   "edit-page",
		" History! ":             "history-page",
		"ATTACHMENTS":            "attachments-page",
		"Edit notes":             "edit-notes",
	}
	for in, want := range cases {
		if got := store.SlugFromTitle(in); got != want {
			t.Errorf("SlugFromTitle(%q) = %q, want %q", in, got, want)
		}
	}
	long := store.SlugFromTitle(strings.Repeat("word ", 40))
	if len([]rune(long)) > 80 || strings.HasSuffix(long, "-") || strings.HasPrefix(long, "-") {
		t.Errorf("long slug: %q (%d)", long, len(long))
	}
	exact := store.SlugFromTitle(strings.Repeat("a", 120))
	if len(exact) != 80 {
		t.Errorf("exact slug length %d", len(exact))
	}
}

func TestCreateSpaceCreatesHomePage(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	u := testutil.CreateUser(t, s, "creator@example.test", "", false)

	icon := "book"
	sp, err := s.CreateSpace(ctx, store.CreateSpaceParams{
		Key: "ENG", Name: "Engineering Docs", Description: "d", Icon: &icon,
		Visibility: domain.VisibilityPrivate, CreatedBy: u.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sp.Key != "ENG" || sp.Visibility != domain.VisibilityPrivate || sp.Icon == nil || *sp.Icon != "book" {
		t.Fatalf("space: %+v", sp)
	}
	if sp.Role == nil || *sp.Role != domain.RoleAdmin || sp.MemberCount != 1 || sp.PageCount != 1 {
		t.Fatalf("role/counts: %+v", sp)
	}
	if sp.HomePageID == nil {
		t.Fatal("home page not linked")
	}

	var title, slug, status, html string
	var version, position int
	var createdBy, updatedBy uuid.UUID
	var publishedNull bool
	err = s.Pool.QueryRow(ctx,
		`SELECT title, slug, status, version, position, content_html, created_by, updated_by, published_at IS NULL
		 FROM pages WHERE id = $1`, *sp.HomePageID).
		Scan(&title, &slug, &status, &version, &position, &html, &createdBy, &updatedBy, &publishedNull)
	if err != nil {
		t.Fatal(err)
	}
	if title != "Engineering Docs" || slug != "engineering-docs" || status != "published" || version != 1 ||
		position != 0 || html != "<p></p>" || createdBy != u.ID || updatedBy != u.ID || publishedNull {
		t.Fatalf("home page: %s %s %s v%d pos%d %s", title, slug, status, version, position, html)
	}
	var docType string
	if err := s.Pool.QueryRow(ctx, `SELECT content_json->>'type' FROM pages WHERE id = $1`, *sp.HomePageID).Scan(&docType); err != nil || docType != "doc" {
		t.Fatalf("content_json type %q %v", docType, err)
	}

	var vTitle, vMsg string
	var vNum int
	var vAuthor uuid.UUID
	err = s.Pool.QueryRow(ctx,
		`SELECT title, message, version, author_id FROM page_versions WHERE page_id = $1`, *sp.HomePageID).
		Scan(&vTitle, &vMsg, &vNum, &vAuthor)
	if err != nil || vTitle != "Engineering Docs" || vMsg != "Created" || vNum != 1 || vAuthor != u.ID {
		t.Fatalf("version row: %q %q %d %v", vTitle, vMsg, vNum, err)
	}

	_, err = s.CreateSpace(ctx, store.CreateSpaceParams{Key: "ENG", Name: "Other", Visibility: domain.VisibilityInternal, CreatedBy: u.ID})
	if !errors.Is(err, store.ErrConflict) {
		t.Fatalf("duplicate key: %v", err)
	}
	var n int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM pages`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("failed create leaked a page: %d %v", n, err)
	}
}

func TestListSpacesAccessible(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	owner := testutil.CreateUser(t, s, "owner@example.test", "", false)
	member := testutil.CreateUser(t, s, "member@example.test", "", false)
	outsider := testutil.CreateUser(t, s, "outsider@example.test", "", false)
	root := testutil.CreateUser(t, s, "root@example.test", "", true)

	pub, _ := s.CreateSpace(ctx, store.CreateSpaceParams{Key: "PUB", Name: "Bravo", Visibility: domain.VisibilityInternal, CreatedBy: owner.ID})
	priv, _ := s.CreateSpace(ctx, store.CreateSpaceParams{Key: "PRIV", Name: "alpha", Visibility: domain.VisibilityPrivate, CreatedBy: owner.ID})
	gone, _ := s.CreateSpace(ctx, store.CreateSpaceParams{Key: "GONE", Name: "Gone", Visibility: domain.VisibilityInternal, CreatedBy: owner.ID})
	arch, _ := s.CreateSpace(ctx, store.CreateSpaceParams{Key: "ARCH", Name: "Charlie", Visibility: domain.VisibilityInternal, CreatedBy: owner.ID})
	testutil.AddMember(t, s, priv.ID, member.ID, domain.RoleEditor)
	testutil.CreatePage(t, s, pub.ID, owner.ID, "extra")
	spaceTestPage(t, s, pub.ID, owner.ID, "draft", "draft", nil, 0)
	spaceTestPage(t, s, pub.ID, owner.ID, "trashed", "trashed", nil, 0)
	if err := s.SoftDeleteSpace(ctx, gone.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSpaceArchived(ctx, arch.ID, true); err != nil {
		t.Fatal(err)
	}

	keys := func(list []store.SpaceView) string {
		var ks []string
		for _, v := range list {
			ks = append(ks, v.Key)
		}
		return strings.Join(ks, ",")
	}

	got, err := s.ListSpacesAccessible(ctx, member.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if keys(got) != "PRIV,PUB,ARCH" {
		t.Fatalf("member keys %s", keys(got))
	}
	if *got[0].Role != domain.RoleEditor || *got[1].Role != domain.RoleViewer || got[0].MemberCount != 2 {
		t.Fatalf("member roles %+v", got)
	}
	if got[1].PageCount != 2 || got[1].MemberCount != 1 {
		t.Fatalf("PUB counts: page=%d member=%d", got[1].PageCount, got[1].MemberCount)
	}
	if got[2].ArchivedAt == nil {
		t.Fatal("archived space must still be listed with archived_at")
	}

	got, _ = s.ListSpacesAccessible(ctx, outsider.ID, false)
	if keys(got) != "PUB,ARCH" {
		t.Fatalf("outsider keys %s", keys(got))
	}

	got, _ = s.ListSpacesAccessible(ctx, root.ID, true)
	if keys(got) != "PRIV,PUB,ARCH" {
		t.Fatalf("root keys %s", keys(got))
	}
	for _, v := range got {
		if *v.Role != domain.RoleAdmin {
			t.Fatalf("system admin role for %s: %s", v.Key, *v.Role)
		}
	}

	all, err := s.ListAdminSpaces(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if keys(all) != "ARCH,GONE,PRIV,PUB" {
		t.Fatalf("admin keys %s", keys(all))
	}
	if all[1].DeletedAt == nil || all[3].PageCount != 2 {
		t.Fatalf("admin view: %+v", all)
	}
}

func TestUpdateSpace(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	u := testutil.CreateUser(t, s, "u@example.test", "", false)
	sp, _ := s.CreateSpace(ctx, store.CreateSpaceParams{Key: "UPD", Name: "Old", Icon: spacesStrPtr("x"), Visibility: domain.VisibilityInternal, CreatedBy: u.ID})
	other, _ := s.CreateSpace(ctx, store.CreateSpaceParams{Key: "OTH", Name: "Other", Visibility: domain.VisibilityInternal, CreatedBy: u.ID})

	name := "New"
	vis := domain.VisibilityPrivate
	if err := s.UpdateSpace(ctx, sp.ID, store.SpaceUpdate{Name: &name, Visibility: &vis, SetIcon: true}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetSpaceView(ctx, sp.ID)
	if got.Name != "New" || got.Visibility != domain.VisibilityPrivate || got.Icon != nil || got.HomePageID == nil {
		t.Fatalf("after update: %+v", got)
	}

	child := testutil.CreatePage(t, s, sp.ID, u.ID, "child")
	if err := s.UpdateSpace(ctx, sp.ID, store.SpaceUpdate{SetHomePage: true, HomePageID: &child}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetSpaceView(ctx, sp.ID)
	if got.HomePageID == nil || *got.HomePageID != child {
		t.Fatalf("home page not switched: %+v", got.HomePageID)
	}

	foreign := *other.HomePageID
	if err := s.UpdateSpace(ctx, sp.ID, store.SpaceUpdate{SetHomePage: true, HomePageID: &foreign}); !errors.Is(err, store.ErrInvalidHomePage) {
		t.Fatalf("foreign home page: %v", err)
	}
	trashed := spaceTestPage(t, s, sp.ID, u.ID, "bin", "trashed", nil, 0)
	if err := s.UpdateSpace(ctx, sp.ID, store.SpaceUpdate{SetHomePage: true, HomePageID: &trashed}); !errors.Is(err, store.ErrInvalidHomePage) {
		t.Fatalf("trashed home page: %v", err)
	}

	if err := s.UpdateSpace(ctx, sp.ID, store.SpaceUpdate{SetHomePage: true}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetSpaceView(ctx, sp.ID)
	if got.HomePageID != nil {
		t.Fatalf("home page should be cleared")
	}

	if err := s.SoftDeleteSpace(ctx, sp.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.SoftDeleteSpace(ctx, sp.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
	if err := s.UpdateSpace(ctx, sp.ID, store.SpaceUpdate{Name: &name}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("update deleted: %v", err)
	}
	if err := s.UpdateSpace(ctx, uuid.New(), store.SpaceUpdate{Name: &name}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}
}

func TestSetSpaceArchived(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	u := testutil.CreateUser(t, s, "u@example.test", "", false)
	sp, _ := s.CreateSpace(ctx, store.CreateSpaceParams{Key: "ARC", Name: "Arc", Visibility: domain.VisibilityInternal, CreatedBy: u.ID})

	if err := s.SetSpaceArchived(ctx, sp.ID, true); err != nil {
		t.Fatal(err)
	}
	first, _ := s.GetSpaceView(ctx, sp.ID)
	if first.ArchivedAt == nil {
		t.Fatal("not archived")
	}
	if err := s.SetSpaceArchived(ctx, sp.ID, true); err != nil {
		t.Fatal(err)
	}
	again, _ := s.GetSpaceView(ctx, sp.ID)
	if !again.ArchivedAt.Equal(*first.ArchivedAt) {
		t.Fatal("re-archive must keep the original timestamp")
	}
	if err := s.SetSpaceArchived(ctx, sp.ID, false); err != nil {
		t.Fatal(err)
	}
	back, _ := s.GetSpaceView(ctx, sp.ID)
	if back.ArchivedAt != nil {
		t.Fatal("not unarchived")
	}
	if err := s.SetSpaceArchived(ctx, uuid.New(), true); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestSpaceMembers(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	zed := testutil.CreateUser(t, s, "zed@example.test", "", false)
	amy := testutil.CreateUser(t, s, "amy@example.test", "", false)
	bob := testutil.CreateUser(t, s, "bob@example.test", "", false)
	sp, _ := s.CreateSpace(ctx, store.CreateSpaceParams{Key: "MEM", Name: "Mem", Visibility: domain.VisibilityInternal, CreatedBy: zed.ID})

	m, inserted, err := s.UpsertSpaceMember(ctx, sp.ID, amy.ID, domain.RoleEditor)
	if err != nil || !inserted || m.Role != domain.RoleEditor || m.User.DisplayName != "amy@example.test" || m.SpaceID != sp.ID {
		t.Fatalf("insert: %+v %v %v", m, inserted, err)
	}
	m, inserted, err = s.UpsertSpaceMember(ctx, sp.ID, amy.ID, domain.RoleViewer)
	if err != nil || inserted || m.Role != domain.RoleViewer {
		t.Fatalf("update: %+v %v %v", m, inserted, err)
	}
	if _, _, err := s.UpsertSpaceMember(ctx, sp.ID, bob.ID, domain.RoleAdmin); err != nil {
		t.Fatal(err)
	}

	list, err := s.ListSpaceMembers(ctx, sp.ID)
	if err != nil || len(list) != 3 {
		t.Fatalf("list: %v %+v", err, list)
	}
	if list[0].User.ID != amy.ID || list[1].User.ID != bob.ID || list[2].User.ID != zed.ID {
		t.Fatalf("not sorted by display name: %+v", list)
	}

	if _, _, err := s.UpsertSpaceMember(ctx, sp.ID, uuid.New(), domain.RoleViewer); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown user: %v", err)
	}

	if _, _, err := s.UpsertSpaceMember(ctx, sp.ID, zed.ID, domain.RoleEditor); err != nil {
		t.Fatalf("demote with a second admin present: %v", err)
	}
	if _, _, err := s.UpsertSpaceMember(ctx, sp.ID, bob.ID, domain.RoleViewer); !errors.Is(err, store.ErrLastAdmin) {
		t.Fatalf("demote last admin: %v", err)
	}
	if err := s.RemoveSpaceMember(ctx, sp.ID, bob.ID); !errors.Is(err, store.ErrLastAdmin) {
		t.Fatalf("remove last admin: %v", err)
	}
	if _, _, err := s.UpsertSpaceMember(ctx, sp.ID, bob.ID, domain.RoleAdmin); err != nil {
		t.Fatalf("re-asserting admin must be allowed: %v", err)
	}
	if err := s.RemoveSpaceMember(ctx, sp.ID, amy.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveSpaceMember(ctx, sp.ID, amy.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("remove twice: %v", err)
	}
	if _, err := s.GetSpaceMemberView(ctx, sp.ID, amy.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("get removed: %v", err)
	}
}

func TestSpaceTreeAndTrash(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	admin := testutil.CreateUser(t, s, "admin@example.test", "", false)
	ed := testutil.CreateUser(t, s, "ed@example.test", "", false)
	other := testutil.CreateUser(t, s, "other@example.test", "", false)
	sp, _ := s.CreateSpace(ctx, store.CreateSpaceParams{Key: "TRE", Name: "Tree", Visibility: domain.VisibilityInternal, CreatedBy: admin.ID})
	home := *sp.HomePageID

	a := spaceTestPage(t, s, sp.ID, ed.ID, "A", "published", &home, 1)
	b := spaceTestPage(t, s, sp.ID, ed.ID, "B", "published", &home, 0)
	aChild := spaceTestPage(t, s, sp.ID, ed.ID, "A child", "published", &a, 0)
	edDraft := spaceTestPage(t, s, sp.ID, ed.ID, "Ed draft", "draft", &b, 0)
	spaceTestPage(t, s, sp.ID, other.ID, "Other draft", "draft", &b, 1)
	trashedChild := spaceTestPage(t, s, sp.ID, ed.ID, "Trashed", "trashed", &b, 2)
	_ = aChild

	byTitle := func(list []store.SpacePageSummary) map[string]store.SpacePageSummary {
		m := map[string]store.SpacePageSummary{}
		for _, p := range list {
			m[p.Title] = p
		}
		return m
	}

	tree, err := s.ListSpaceTree(ctx, sp.ID, ed.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	m := byTitle(tree)
	if len(tree) != 5 {
		t.Fatalf("editor tree: %d %+v", len(tree), tree)
	}
	if _, ok := m["Trashed"]; ok {
		t.Fatal("trashed page in tree")
	}
	if _, ok := m["Other draft"]; ok {
		t.Fatal("another user's draft visible")
	}
	if m["Ed draft"].ID != edDraft || m["Ed draft"].Status != domain.PageStatusDraft {
		t.Fatalf("own draft: %+v", m["Ed draft"])
	}
	if !m["B"].HasChildren || !m["A"].HasChildren || m["A child"].HasChildren || !m[sp.Name].HasChildren {
		t.Fatalf("has_children: %+v", tree)
	}
	if tree[0].ParentID != nil || tree[0].ID != home {
		t.Fatalf("root first: %+v", tree[0])
	}
	if tree[1].Title != "B" || tree[2].Title != "A" {
		t.Fatalf("position order under home: %s, %s", tree[1].Title, tree[2].Title)
	}

	tree, _ = s.ListSpaceTree(ctx, sp.ID, other.ID, false)
	m = byTitle(tree)
	if len(tree) != 5 || m["Ed draft"].ID != uuid.Nil || m["Other draft"].ID == uuid.Nil {
		t.Fatalf("other tree: %+v", tree)
	}

	tree, _ = s.ListSpaceTree(ctx, sp.ID, admin.ID, true)
	if len(tree) != 6 {
		t.Fatalf("admin tree sees all drafts: %d", len(tree))
	}

	onlyPublished := spaceTestPage(t, s, sp.ID, ed.ID, "Solo parent", "published", nil, 5)
	spaceTestPage(t, s, sp.ID, ed.ID, "Hidden draft kid", "draft", &onlyPublished, 0)
	tree, _ = s.ListSpaceTree(ctx, sp.ID, other.ID, false)
	if byTitle(tree)["Solo parent"].HasChildren {
		t.Fatal("has_children must be computed against the visible set")
	}

	trash, err := s.ListSpaceTrash(ctx, sp.ID, ed.ID, false)
	if err != nil || len(trash) != 1 {
		t.Fatalf("trash: %v %+v", err, trash)
	}
	if trash[0].ID != trashedChild || trash[0].TrashedBy == nil || *trash[0].TrashedBy != ed.ID ||
		trash[0].TrashedByUser == nil || trash[0].TrashedByUser.DisplayName != "ed@example.test" || trash[0].Status != domain.PageStatusTrashed {
		t.Fatalf("trash row: %+v", trash[0])
	}

	older := spaceTestPage(t, s, sp.ID, ed.ID, "Older", "trashed", nil, 0)
	if _, err := s.Pool.Exec(ctx, `UPDATE pages SET trashed_at = now() - interval '2 days', trashed_by = NULL WHERE id = $1`, older); err != nil {
		t.Fatal(err)
	}
	trash, _ = s.ListSpaceTrash(ctx, sp.ID, ed.ID, false)
	if len(trash) != 2 || trash[0].ID != trashedChild || trash[1].ID != older || trash[1].TrashedByUser != nil {
		t.Fatalf("trash order: %+v", trash)
	}

	trash, _ = s.ListSpaceTrash(ctx, sp.ID, other.ID, false)
	if len(trash) != 0 {
		t.Fatalf("never-published trashed pages must not leak to other users: %+v", trash)
	}
	trash, _ = s.ListSpaceTrash(ctx, sp.ID, admin.ID, true)
	if len(trash) != 2 {
		t.Fatalf("admin sees trashed drafts: %+v", trash)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE pages SET version = 3 WHERE id = $1`, older); err != nil {
		t.Fatal(err)
	}
	trash, _ = s.ListSpaceTrash(ctx, sp.ID, other.ID, false)
	if len(trash) != 1 || trash[0].ID != older {
		t.Fatalf("previously published trashed page is visible to everyone: %+v", trash)
	}
}

func spacesStrPtr(s string) *string { return &s }
