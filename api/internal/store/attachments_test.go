package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/store"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

func newAttachment(pageID, spaceID, uploaderID uuid.UUID, filename string) *domain.Attachment {
	id := uuid.Must(uuid.NewV7())
	return &domain.Attachment{
		ID:          id,
		PageID:      pageID,
		SpaceID:     spaceID,
		UploaderID:  uploaderID,
		Filename:    filename,
		ContentType: "text/plain",
		Size:        42,
		S3Key:       "spaces/" + spaceID.String() + "/pages/" + pageID.String() + "/" + id.String() + "/" + filename,
	}
}

func TestAttachmentsCRUD(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	u1 := testutil.CreateUser(t, s, "a1@example.test", "", false)
	u2 := testutil.CreateUser(t, s, "a2@example.test", "", false)
	spaceID := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "ATT", CreatedBy: u1.ID})
	pageID := testutil.CreatePage(t, s, spaceID, u1.ID, "Page")
	otherPage := testutil.CreatePage(t, s, spaceID, u1.ID, "Other")

	first := newAttachment(pageID, spaceID, u1.ID, "first.txt")
	if err := s.CreateAttachment(ctx, first); err != nil {
		t.Fatal(err)
	}
	if first.CreatedAt.IsZero() || time.Since(first.CreatedAt) > time.Minute {
		t.Fatalf("created_at not populated: %v", first.CreatedAt)
	}
	second := newAttachment(pageID, spaceID, u2.ID, "second.txt")
	if err := s.CreateAttachment(ctx, second); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAttachment(ctx, newAttachment(otherPage, spaceID, u1.ID, "elsewhere.txt")); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetAttachment(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Filename != "first.txt" || got.ContentType != "text/plain" || got.Size != 42 || got.S3Key != first.S3Key || got.UploaderID != u1.ID || got.DeletedAt != nil {
		t.Fatalf("unexpected attachment: %+v", got)
	}

	gotSpace, err := s.GetAttachmentSpaceID(ctx, first.ID)
	if err != nil || gotSpace != spaceID {
		t.Fatalf("space id: %v %v", gotSpace, err)
	}
	if _, err := s.GetAttachmentSpaceID(ctx, uuid.Must(uuid.NewV7())); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing space id: %v", err)
	}
	if _, err := s.GetAttachment(ctx, uuid.Must(uuid.NewV7())); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing attachment: %v", err)
	}

	list, err := s.ListPageAttachments(ctx, pageID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != second.ID || list[1].ID != first.ID {
		t.Fatalf("expected newest first, got %+v", list)
	}
	if list[0].Uploader.ID != u2.ID || list[0].Uploader.Email != u2.Email || list[0].Uploader.DisplayName != u2.DisplayName {
		t.Fatalf("uploader not joined: %+v", list[0].Uploader)
	}

	n, err := s.CountPageAttachments(ctx, pageID)
	if err != nil || n != 2 {
		t.Fatalf("count: %d %v", n, err)
	}

	deleted, err := s.SoftDeleteAttachment(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.S3Key != first.S3Key || deleted.DeletedAt == nil {
		t.Fatalf("soft delete result: %+v", deleted)
	}
	if _, err := s.SoftDeleteAttachment(ctx, first.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
	if _, err := s.GetAttachment(ctx, first.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted attachment still readable: %v", err)
	}
	if _, err := s.GetAttachmentSpaceID(ctx, first.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted attachment still resolves: %v", err)
	}
	list, err = s.ListPageAttachments(ctx, pageID)
	if err != nil || len(list) != 1 || list[0].ID != second.ID {
		t.Fatalf("list after delete: %v %+v", err, list)
	}
	n, err = s.CountPageAttachments(ctx, pageID)
	if err != nil || n != 1 {
		t.Fatalf("count after delete: %d %v", n, err)
	}
}

func TestAttachmentsEmptyListIsNotNil(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	u := testutil.CreateUser(t, s, "empty@example.test", "", false)
	spaceID := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "EMP", CreatedBy: u.ID})
	pageID := testutil.CreatePage(t, s, spaceID, u.ID, "Page")
	list, err := s.ListPageAttachments(ctx, pageID)
	if err != nil || list == nil || len(list) != 0 {
		t.Fatalf("expected empty non-nil list, got %v %v", list, err)
	}
	n, err := s.CountPageAttachments(ctx, pageID)
	if err != nil || n != 0 {
		t.Fatalf("count: %d %v", n, err)
	}
}

func TestAttachmentPageMeta(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	u := testutil.CreateUser(t, s, "meta@example.test", "", false)
	spaceID := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "MET", CreatedBy: u.ID})
	pageID := testutil.CreatePage(t, s, spaceID, u.ID, "Page")

	m, err := s.GetAttachmentPageMeta(ctx, pageID)
	if err != nil {
		t.Fatal(err)
	}
	if m.SpaceID != spaceID || m.Status != domain.PageStatusPublished || m.CreatedBy != u.ID {
		t.Fatalf("meta: %+v", m)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE pages SET status = 'draft' WHERE id = $1`, pageID); err != nil {
		t.Fatal(err)
	}
	m, err = s.GetAttachmentPageMeta(ctx, pageID)
	if err != nil || m.Status != domain.PageStatusDraft {
		t.Fatalf("draft meta: %+v %v", m, err)
	}
	if _, err := s.GetAttachmentPageMeta(ctx, uuid.Must(uuid.NewV7())); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing page: %v", err)
	}
}

func TestAttachmentsCascadeWithPage(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	u := testutil.CreateUser(t, s, "cascade@example.test", "", false)
	spaceID := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "CAS", CreatedBy: u.ID})
	pageID := testutil.CreatePage(t, s, spaceID, u.ID, "Page")
	a := newAttachment(pageID, spaceID, u.ID, "a.txt")
	if err := s.CreateAttachment(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `DELETE FROM pages WHERE id = $1`, pageID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetAttachment(ctx, a.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("attachment should cascade with page: %v", err)
	}
}

func TestAttachmentsRejectUnknownPage(t *testing.T) {
	s := testutil.NewDB(t)
	u := testutil.CreateUser(t, s, "fk@example.test", "", false)
	spaceID := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "FKS", CreatedBy: u.ID})
	err := s.CreateAttachment(context.Background(), newAttachment(uuid.Must(uuid.NewV7()), spaceID, u.ID, "x.txt"))
	if !store.IsForeignKeyViolation(err) {
		t.Fatalf("expected foreign key violation, got %v", err)
	}
}

func TestListAttachmentObjectKeysUnderPage(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	u := testutil.CreateUser(t, s, "keys@example.test", "", false)
	spaceID := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "KEY", CreatedBy: u.ID})
	root := testutil.CreatePage(t, s, spaceID, u.ID, "Root")
	child := testutil.CreatePage(t, s, spaceID, u.ID, "Child")
	grandchild := testutil.CreatePage(t, s, spaceID, u.ID, "Grandchild")
	sibling := testutil.CreatePage(t, s, spaceID, u.ID, "Sibling")
	for _, q := range []struct{ id, parent uuid.UUID }{{child, root}, {grandchild, child}} {
		if _, err := s.Pool.Exec(ctx, `UPDATE pages SET parent_id = $2 WHERE id = $1`, q.id, q.parent); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE pages SET status = 'trashed' WHERE id = $1`, grandchild); err != nil {
		t.Fatal(err)
	}

	add := func(pageID uuid.UUID, name string) *domain.Attachment {
		a := newAttachment(pageID, spaceID, u.ID, name)
		if err := s.CreateAttachment(ctx, a); err != nil {
			t.Fatal(err)
		}
		return a
	}
	onRoot := add(root, "root.txt")
	onChild := add(child, "child.txt")
	onGrand := add(grandchild, "grand.txt")
	removed := add(child, "removed.txt")
	add(sibling, "sibling.txt")
	if _, err := s.SoftDeleteAttachment(ctx, removed.ID); err != nil {
		t.Fatal(err)
	}

	keys, err := s.ListAttachmentObjectKeysUnderPage(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{onRoot.S3Key, onChild.S3Key, onGrand.S3Key}
	if len(keys) != len(want) {
		t.Fatalf("keys %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("keys %v, want %v", keys, want)
		}
	}

	keys, err = s.ListAttachmentObjectKeysUnderPage(ctx, child)
	if err != nil || len(keys) != 2 {
		t.Fatalf("child subtree: %v %v", keys, err)
	}
	keys, err = s.ListAttachmentObjectKeysUnderPage(ctx, uuid.Must(uuid.NewV7()))
	if err != nil || keys == nil || len(keys) != 0 {
		t.Fatalf("unknown page: %v %v", keys, err)
	}
}

func TestListAttachmentObjectKeysUnderPageSurvivesCycle(t *testing.T) {
	s := testutil.NewDB(t)
	u := testutil.CreateUser(t, s, "cycle@example.test", "", false)
	spaceID := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "CYC", CreatedBy: u.ID})
	a := testutil.CreatePage(t, s, spaceID, u.ID, "A")
	b := testutil.CreatePage(t, s, spaceID, u.ID, "B")
	setup := context.Background()
	for _, q := range []struct{ id, parent uuid.UUID }{{b, a}, {a, b}} {
		if _, err := s.Pool.Exec(setup, `UPDATE pages SET parent_id = $2 WHERE id = $1`, q.id, q.parent); err != nil {
			t.Fatal(err)
		}
	}
	onA := newAttachment(a, spaceID, u.ID, "a.txt")
	onB := newAttachment(b, spaceID, u.ID, "b.txt")
	for _, att := range []*domain.Attachment{onA, onB} {
		if err := s.CreateAttachment(setup, att); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithTimeout(setup, 10*time.Second)
	defer cancel()
	start := time.Now()
	keys, err := s.ListAttachmentObjectKeysUnderPage(ctx, a)
	if err != nil {
		t.Fatalf("cycle: %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("cycle walk took %v", time.Since(start))
	}
	if len(keys) != 2 || keys[0] != onA.S3Key || keys[1] != onB.S3Key {
		t.Fatalf("keys %v", keys)
	}
}
