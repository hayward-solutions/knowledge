package testutil

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/auth"
	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/store"
)

const DefaultPassword = "Correct-Horse-9-Battery"

var (
	hashMu    sync.Mutex
	hashCache = map[string]string{}
)

func hashFor(t *testing.T, password string) string {
	t.Helper()
	hashMu.Lock()
	defer hashMu.Unlock()
	if h, ok := hashCache[password]; ok {
		return h
	}
	h, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	hashCache[password] = h
	return h
}

func CreateUser(t *testing.T, s *store.Store, email, password string, admin bool) domain.User {
	t.Helper()
	if password == "" {
		password = DefaultPassword
	}
	u := domain.User{
		ID:            uuid.Must(uuid.NewV7()),
		Email:         email,
		DisplayName:   email,
		IsSystemAdmin: admin,
	}
	if err := s.CreateUser(context.Background(), u, hashFor(t, password)); err != nil {
		t.Fatalf("create user %s: %v", email, err)
	}
	got, err := s.GetUser(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	return got
}

func NewSession(t *testing.T, s *store.Store, userID uuid.UUID) string {
	t.Helper()
	token, err := s.CreateSession(context.Background(), userID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return token
}

type SpaceOpts struct {
	Key        string
	Visibility domain.Visibility
	Archived   bool
	Deleted    bool
	CreatedBy  uuid.UUID
}

func CreateSpace(t *testing.T, s *store.Store, o SpaceOpts) uuid.UUID {
	t.Helper()
	if o.Visibility == "" {
		o.Visibility = domain.VisibilityInternal
	}
	id := uuid.Must(uuid.NewV7())
	_, err := s.Pool.Exec(context.Background(),
		`INSERT INTO spaces (id, key, name, visibility, created_by, archived_at, deleted_at)
		 VALUES ($1, $2, $2, $3, $4,
		         CASE WHEN $5 THEN now() END,
		         CASE WHEN $6 THEN now() END)`,
		id, o.Key, string(o.Visibility), o.CreatedBy, o.Archived, o.Deleted)
	if err != nil {
		t.Fatalf("create space %s: %v", o.Key, err)
	}
	return id
}

func AddMember(t *testing.T, s *store.Store, spaceID, userID uuid.UUID, role domain.Role) {
	t.Helper()
	_, err := s.Pool.Exec(context.Background(),
		`INSERT INTO space_members (space_id, user_id, role) VALUES ($1, $2, $3)`,
		spaceID, userID, string(role))
	if err != nil {
		t.Fatalf("add member: %v", err)
	}
}

func CreatePage(t *testing.T, s *store.Store, spaceID, createdBy uuid.UUID, title string) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	_, err := s.Pool.Exec(context.Background(),
		`INSERT INTO pages (id, space_id, title, slug, status, created_by, updated_by)
		 VALUES ($1, $2, $3, $3, 'published', $4, $4)`,
		id, spaceID, title, createdBy)
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	return id
}
