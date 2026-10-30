package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/store"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

func templateTestNames(list []store.TemplateView) []string {
	out := make([]string, len(list))
	for i, v := range list {
		out[i] = v.Name
	}
	return out
}

func templateTestEqual(a, b []string) bool {
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

func TestTemplateCRUDAndContent(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	u := testutil.CreateUser(t, s, "admin@example.test", "", true)
	spaceID := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "ENG", CreatedBy: u.ID})

	doc := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph"}]}`)
	sp, err := s.CreateTemplate(ctx, store.NewTemplate{
		SpaceID: &spaceID, Name: "Space tpl", Description: "d", ContentJSON: doc, ContentHTML: "<p></p>", CreatedBy: u.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sp.SpaceKey == nil || *sp.SpaceKey != "ENG" || sp.SpaceID == nil || *sp.SpaceID != spaceID {
		t.Fatalf("space tpl %+v", sp)
	}
	gl, err := s.CreateTemplate(ctx, store.NewTemplate{Name: "Global tpl", CreatedBy: u.ID})
	if err != nil {
		t.Fatal(err)
	}
	if gl.SpaceID != nil || gl.SpaceKey != nil || string(gl.ContentJSON) != "" {
		t.Fatalf("global tpl %+v json=%q", gl, gl.ContentJSON)
	}
	out, err := json.Marshal(gl)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if v, ok := m["space_key"]; !ok || v != nil {
		t.Fatalf("space_key %v present=%v", v, ok)
	}
	if v, ok := m["content_json"]; !ok || v != nil {
		t.Fatalf("content_json %v present=%v", v, ok)
	}

	c, err := s.GetTemplateContent(ctx, sp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c.SpaceID != spaceID || c.Title != "Space tpl" || c.ContentHTML != "<p></p>" {
		t.Fatalf("content %+v", c)
	}
	var back map[string]any
	if err := json.Unmarshal(c.ContentJSON, &back); err != nil || back["type"] != "doc" {
		t.Fatalf("content json %s %v", c.ContentJSON, err)
	}
	cg, err := s.GetTemplateContent(ctx, gl.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cg.SpaceID != uuid.Nil || cg.ContentJSON != nil {
		t.Fatalf("global content %+v", cg)
	}
	if _, err := s.GetTemplateContent(ctx, uuid.Must(uuid.NewV7())); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("want not found, got %v", err)
	}

	name := "Renamed"
	up, err := s.UpdateTemplate(ctx, sp.ID, store.TemplatePatch{Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	if up.Name != "Renamed" || up.Description != "d" || up.ContentHTML != "<p></p>" || string(up.ContentJSON) == "" {
		t.Fatalf("patch should keep untouched fields: %+v", up)
	}
	if !up.UpdatedAt.After(sp.UpdatedAt) {
		t.Fatalf("updated_at not bumped")
	}
	cleared, err := s.UpdateTemplate(ctx, sp.ID, store.TemplatePatch{SetContentJSON: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(cleared.ContentJSON) != "" {
		t.Fatalf("content_json should be null: %s", cleared.ContentJSON)
	}
	html := "<p>x</p>"
	set, err := s.UpdateTemplate(ctx, sp.ID, store.TemplatePatch{SetContentJSON: true, ContentJSON: doc, ContentHTML: &html})
	if err != nil {
		t.Fatal(err)
	}
	if string(set.ContentJSON) == "" || set.ContentHTML != html {
		t.Fatalf("set %+v", set)
	}
	if _, err := s.UpdateTemplate(ctx, uuid.Must(uuid.NewV7()), store.TemplatePatch{Name: &name}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("want not found, got %v", err)
	}

	if err := s.DeleteTemplate(ctx, gl.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteTemplate(ctx, gl.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
	if _, err := s.GetTemplate(ctx, gl.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
}

func TestTemplateListVisibility(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	admin := testutil.CreateUser(t, s, "admin@example.test", "", true)
	editor := testutil.CreateUser(t, s, "editor@example.test", "", false)
	viewer := testutil.CreateUser(t, s, "viewer@example.test", "", false)
	stranger := testutil.CreateUser(t, s, "stranger@example.test", "", false)
	a := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "AAA", CreatedBy: admin.ID})
	b := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "BBB", Visibility: domain.VisibilityPrivate, CreatedBy: admin.ID})
	gone := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "DEL", Deleted: true, CreatedBy: admin.ID})
	testutil.AddMember(t, s, a, editor.ID, domain.RoleEditor)
	testutil.AddMember(t, s, a, viewer.ID, domain.RoleViewer)
	testutil.AddMember(t, s, b, viewer.ID, domain.RoleAdmin)
	testutil.AddMember(t, s, gone, editor.ID, domain.RoleEditor)

	mk := func(spaceID *uuid.UUID, name string) {
		t.Helper()
		if _, err := s.CreateTemplate(ctx, store.NewTemplate{SpaceID: spaceID, Name: name, CreatedBy: admin.ID}); err != nil {
			t.Fatal(err)
		}
	}
	mk(nil, "zeta global")
	mk(nil, "Alpha global")
	mk(&a, "b in A")
	mk(&a, "a in A")
	mk(&b, "in B")
	mk(&gone, "in deleted")

	cases := []struct {
		name  string
		user  uuid.UUID
		admin bool
		want  []string
	}{
		{"system admin", admin.ID, true, []string{"Alpha global", "zeta global", "a in A", "b in A", "in B"}},
		{"editor", editor.ID, false, []string{"Alpha global", "zeta global", "a in A", "b in A"}},
		{"viewer in A, admin in B", viewer.ID, false, []string{"Alpha global", "zeta global", "in B"}},
		{"stranger", stranger.ID, false, []string{"Alpha global", "zeta global"}},
	}
	for _, c := range cases {
		got, err := s.ListVisibleTemplates(ctx, c.user, c.admin)
		if err != nil {
			t.Fatal(err)
		}
		if !templateTestEqual(templateTestNames(got), c.want) {
			t.Errorf("%s: got %v want %v", c.name, templateTestNames(got), c.want)
		}
	}

	only, err := s.ListSpaceTemplates(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if !templateTestEqual(templateTestNames(only), []string{"a in A", "b in A"}) {
		t.Fatalf("space list %v", templateTestNames(only))
	}
	empty, err := s.ListSpaceTemplates(ctx, uuid.Must(uuid.NewV7()))
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty list %v %v", empty, err)
	}
}

func TestTemplateSeedIfEmpty(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	admin := testutil.CreateUser(t, s, "admin@example.test", "", true)
	seeds := []store.TemplateSeed{
		{Name: "One", ContentJSON: json.RawMessage(`{"type":"doc"}`), ContentHTML: "<p>1</p>"},
		{Name: "Two", Description: "second"},
	}
	did, err := s.SeedTemplatesIfEmpty(ctx, admin.ID, seeds)
	if err != nil || !did {
		t.Fatalf("seed: %v %v", did, err)
	}
	did, err = s.SeedTemplatesIfEmpty(ctx, admin.ID, seeds)
	if err != nil || did {
		t.Fatalf("second seed: %v %v", did, err)
	}
	list, err := s.ListVisibleTemplates(ctx, admin.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !templateTestEqual(templateTestNames(list), []string{"One", "Two"}) {
		t.Fatalf("seeded %v", templateTestNames(list))
	}
	for _, v := range list {
		if v.SpaceID != nil || v.CreatedBy != admin.ID {
			t.Fatalf("seeded template %+v", v)
		}
	}
}

func TestTemplateCascadeOnSpaceDelete(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	admin := testutil.CreateUser(t, s, "admin@example.test", "", true)
	spaceID := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "ENG", CreatedBy: admin.ID})
	tpl, err := s.CreateTemplate(ctx, store.NewTemplate{SpaceID: &spaceID, Name: "x", CreatedBy: admin.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `DELETE FROM spaces WHERE id = $1`, spaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetTemplate(ctx, tpl.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("want cascade delete, got %v", err)
	}
}
