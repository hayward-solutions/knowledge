package store_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

func TestActivityListOrderCursorAndDraftHiding(t *testing.T) {
	ctx := context.Background()
	s := testutil.NewDB(t)
	creator := testutil.CreateUser(t, s, "creator@example.test", "", false)
	viewer := testutil.CreateUser(t, s, "viewer@example.test", "", false)
	space := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "ENG", CreatedBy: creator.ID})
	otherSpace := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "OPS", CreatedBy: creator.ID})
	published := testutil.CreatePage(t, s, space, creator.ID, "Public")
	draft := testutil.CreatePage(t, s, space, creator.ID, "Draft")
	if _, err := s.Pool.Exec(ctx, `UPDATE pages SET status = 'draft' WHERE id = $1`, draft); err != nil {
		t.Fatal(err)
	}

	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	add := func(spaceID uuid.UUID, page *uuid.UUID, i int, title string) {
		payload, _ := json.Marshal(map[string]any{"title": title})
		if err := s.InsertActivity(ctx, domain.Activity{
			ID: uuid.Must(uuid.NewV7()), SpaceID: spaceID, PageID: page, ActorID: creator.ID,
			Type: "page.published", Payload: payload, CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	add(space, &published, 0, "Public")
	add(space, &draft, 1, "Draft")
	add(space, &published, 2, "Public again")
	add(otherSpace, &published, 3, "Elsewhere")

	asViewer, err := s.ListSpaceActivity(ctx, space, viewer.ID, domain.RoleViewer, nil, 10)
	if err != nil || len(asViewer) != 2 || *asViewer[0].PageTitle != "Public again" || *asViewer[1].PageTitle != "Public" {
		t.Fatalf("viewer: %+v %v", asViewer, err)
	}
	asCreator, _ := s.ListSpaceActivity(ctx, space, creator.ID, domain.RoleEditor, nil, 10)
	if len(asCreator) != 3 || asCreator[0].Actor == nil || asCreator[0].Actor.ID != creator.ID {
		t.Fatalf("creator: %+v", asCreator)
	}
	asAdmin, _ := s.ListSpaceActivity(ctx, space, viewer.ID, domain.RoleAdmin, nil, 10)
	if len(asAdmin) != 3 {
		t.Fatalf("admin: %d", len(asAdmin))
	}
	cursor := asCreator[0].CreatedAt
	older, _ := s.ListSpaceActivity(ctx, space, creator.ID, domain.RoleEditor, &cursor, 1)
	if len(older) != 1 || *older[0].PageTitle != "Draft" {
		t.Fatalf("cursor: %+v", older)
	}
}

func TestActivityVisibilityByPageState(t *testing.T) {
	ctx := context.Background()
	s := testutil.NewDB(t)
	creator := testutil.CreateUser(t, s, "creator@example.test", "", false)
	editor := testutil.CreateUser(t, s, "editor@example.test", "", false)
	viewer := testutil.CreateUser(t, s, "viewer@example.test", "", false)
	space := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "ENG", CreatedBy: creator.ID})

	live := testutil.CreatePage(t, s, space, creator.ID, "Live")
	trashedPublished := testutil.CreatePage(t, s, space, creator.ID, "Trashed published")
	trashedNever := testutil.CreatePage(t, s, space, creator.ID, "Trashed never published")
	purged := testutil.CreatePage(t, s, space, creator.ID, "Purged")
	for _, q := range []string{
		`UPDATE pages SET status = 'trashed', version = 2, published_at = now() WHERE id = $1`,
	} {
		if _, err := s.Pool.Exec(ctx, q, trashedPublished); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE pages SET status = 'trashed' WHERE id = $1`, trashedNever); err != nil {
		t.Fatal(err)
	}
	for i, id := range []uuid.UUID{live, trashedPublished, trashedNever, purged} {
		page := id
		payload, _ := json.Marshal(map[string]any{"title": fmt.Sprintf("t%d", i)})
		if err := s.InsertActivity(ctx, domain.Activity{ID: uuid.Must(uuid.NewV7()), SpaceID: space, PageID: &page, ActorID: creator.ID, Type: "page.published", Payload: payload}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Pool.Exec(ctx, `DELETE FROM pages WHERE id = $1`, purged); err != nil {
		t.Fatal(err)
	}

	titles := func(u domain.User, role domain.Role) map[string]bool {
		list, err := s.ListSpaceActivity(ctx, space, u.ID, role, nil, 50)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, a := range list {
			out[*a.PageTitle] = true
		}
		return out
	}
	if got := titles(viewer, domain.RoleViewer); len(got) != 1 || !got["t0"] {
		t.Fatalf("viewer: %v", got)
	}
	if got := titles(editor, domain.RoleEditor); len(got) != 2 || !got["t0"] || !got["t1"] {
		t.Fatalf("editor: %v", got)
	}
	if got := titles(creator, domain.RoleEditor); len(got) != 3 || got["t3"] {
		t.Fatalf("creator: %v", got)
	}
	if got := titles(viewer, domain.RoleAdmin); len(got) != 3 || got["t3"] {
		t.Fatalf("admin: %v", got)
	}
}
