package store_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/store"
	"github.com/hayward-solutions/knowledge/api/internal/testutil"
)

func TestSessions(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	u := testutil.CreateUser(t, s, "s@example.test", "", false)

	token, err := s.CreateSession(ctx, u.ID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(token) < 40 {
		t.Fatalf("token too short: %q", token)
	}
	var stored string
	if err := s.Pool.QueryRow(ctx, `SELECT id FROM sessions WHERE user_id = $1`, u.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == token || stored != store.HashSessionToken(token) || len(stored) != 64 {
		t.Fatalf("stored id %q is not the sha256 hex of the token", stored)
	}

	sess, err := s.GetValidSession(ctx, token)
	if err != nil || sess.UserID != u.ID {
		t.Fatalf("get valid: %v %+v", err, sess)
	}
	if _, err := s.GetValidSession(ctx, stored); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("hash must not be usable as a token: %v", err)
	}
	for _, bad := range []string{"", "nope", string(make([]byte, 500))} {
		if _, err := s.GetValidSession(ctx, bad); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("token %q: %v", bad, err)
		}
	}

	expired, err := s.CreateSession(ctx, u.ID, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetValidSession(ctx, expired); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expired session must be invalid: %v", err)
	}
	n, err := s.DeleteExpiredSessions(ctx)
	if err != nil || n != 1 {
		t.Fatalf("delete expired: %d %v", n, err)
	}
	if _, err := s.GetValidSession(ctx, token); err != nil {
		t.Fatalf("valid session was removed: %v", err)
	}

	if err := s.DeleteSession(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetValidSession(ctx, token); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted session still valid: %v", err)
	}

	a := testutil.NewSession(t, s, u.ID)
	b := testutil.NewSession(t, s, u.ID)
	if err := s.DeleteUserSessions(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	for _, tok := range []string{a, b} {
		if _, err := s.GetValidSession(ctx, tok); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("user sessions not revoked: %v", err)
		}
	}
}

func TestUserCrudAndConflicts(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	a := testutil.CreateUser(t, s, "a@example.test", "", false)

	dup := domain.User{ID: uuid.Must(uuid.NewV7()), Email: "A@EXAMPLE.TEST", DisplayName: "dup"}
	if err := s.CreateUser(ctx, dup, ""); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("citext duplicate must conflict: %v", err)
	}
	got, err := s.GetUserByEmail(ctx, "A@Example.Test")
	if err != nil || got.ID != a.ID || got.PasswordHash == nil {
		t.Fatalf("get by email: %v %+v", err, got)
	}
	plain, err := s.GetUser(ctx, a.ID)
	if err != nil || plain.PasswordHash != nil {
		t.Fatalf("GetUser must not load the hash: %v", err)
	}
	if _, err := s.GetUser(ctx, uuid.New()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing user: %v", err)
	}
	if err := s.SetUserPassword(ctx, uuid.New(), "x"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("set password on missing user: %v", err)
	}

	name := "Renamed"
	before := a.UpdatedAt
	time.Sleep(5 * time.Millisecond)
	upd, err := s.UpdateUserProfile(ctx, a.ID, nil, &name)
	if err != nil || upd.DisplayName != name || !upd.UpdatedAt.After(before) {
		t.Fatalf("update profile: %v %+v", err, upd)
	}
	if _, err := s.UpdateUserProfile(ctx, uuid.New(), nil, &name); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}

	refs, err := s.GetUserRefs(ctx, []uuid.UUID{a.ID, uuid.New()})
	if err != nil || len(refs) != 1 || refs[a.ID].DisplayName != name {
		t.Fatalf("refs: %v %v", err, refs)
	}
	if refs, err := s.GetUserRefs(ctx, nil); err != nil || refs == nil || len(refs) != 0 {
		t.Fatalf("empty refs: %v %v", err, refs)
	}
}

func TestUpsertOIDCUser(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	const issuer = "https://issuer.test"
	yes, no := true, false

	testutil.CreateUser(t, s, "spare@example.test", "", true)
	existing := testutil.CreateUser(t, s, "local@example.test", "", true)
	linked, err := s.UpsertOIDCUser(ctx, issuer, "sub-local", "Local@Example.test", "Local SSO", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if linked.ID != existing.ID || !linked.IsSystemAdmin || linked.DisplayName != "Local SSO" {
		t.Fatalf("link by email must keep admin when flag is nil: %+v", linked)
	}
	reload, _ := s.GetUserByEmail(ctx, "local@example.test")
	if reload.PasswordHash == nil {
		t.Fatal("linking must not drop the local password")
	}

	again, err := s.UpsertOIDCUser(ctx, issuer, "sub-local", "local@example.test", "Local SSO 2", &no, true)
	if err != nil || again.ID != existing.ID || again.IsSystemAdmin || again.DisplayName != "Local SSO 2" {
		t.Fatalf("subject match with admin=false: %v %+v", err, again)
	}

	fresh, err := s.UpsertOIDCUser(ctx, issuer, "sub-new", "new@example.test", "New", &yes, true)
	if err != nil || !fresh.IsSystemAdmin || fresh.ID == uuid.Nil {
		t.Fatalf("create: %v %+v", err, fresh)
	}
	withHash, _ := s.GetUserByEmail(ctx, "new@example.test")
	if withHash.PasswordHash != nil {
		t.Fatal("oidc-created user must have no password")
	}

	other, err := s.UpsertOIDCUser(ctx, "https://other.test", "sub-new", "other@example.test", "Other", nil, true)
	if err != nil || other.ID == fresh.ID {
		t.Fatalf("same subject under another issuer must be a different identity: %v %+v", err, other)
	}

	moved, err := s.UpsertOIDCUser(ctx, issuer, "sub-new", "local@example.test", "New", nil, true)
	if err != nil || moved.ID != fresh.ID || moved.Email != "new@example.test" {
		t.Fatalf("email collision on update must keep the old email: %v %+v", err, moved)
	}

	var n int
	_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM oidc_identities`).Scan(&n)
	if n != 3 {
		t.Fatalf("identities = %d, want 3", n)
	}
}

func TestSearchUserRefs(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	testutil.CreateUser(t, s, "100%@example.test", "", false)
	testutil.CreateUser(t, s, "plain@example.test", "", false)
	off := testutil.CreateUser(t, s, "disabled@example.test", "", false)
	yes := true
	if _, err := s.AdminUpdateUser(ctx, off.ID, store.UserUpdate{Disabled: &yes}); err != nil {
		t.Fatal(err)
	}

	got, _ := s.SearchUserRefs(ctx, "%", 10)
	if len(got) != 1 || got[0].Email != "100%@example.test" {
		t.Fatalf("percent must be literal: %+v", got)
	}
	got, _ = s.SearchUserRefs(ctx, "_", 10)
	if len(got) != 0 {
		t.Fatalf("underscore must be literal: %+v", got)
	}
	got, _ = s.SearchUserRefs(ctx, "disabled", 10)
	if len(got) != 0 {
		t.Fatalf("disabled users must be excluded: %+v", got)
	}
	got, _ = s.SearchUserRefs(ctx, "EXAMPLE", 10)
	if len(got) != 2 {
		t.Fatalf("case-insensitive search: %+v", got)
	}
	got, _ = s.SearchUserRefs(ctx, "", 1)
	if len(got) != 1 {
		t.Fatalf("limit: %+v", got)
	}
}

func TestSpaceAccessLookups(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	owner := testutil.CreateUser(t, s, "o@example.test", "", false)
	member := testutil.CreateUser(t, s, "m@example.test", "", false)
	spaceID := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "ENG", Visibility: domain.VisibilityPrivate, CreatedBy: owner.ID})
	testutil.AddMember(t, s, spaceID, member.ID, domain.RoleEditor)
	pageID := testutil.CreatePage(t, s, spaceID, owner.ID, "Hello")

	byKey, err := s.GetSpaceAccessByKey(ctx, "ENG", member.ID)
	if err != nil || byKey.ID != spaceID || byKey.MemberRole == nil || *byKey.MemberRole != domain.RoleEditor {
		t.Fatalf("by key: %v %+v", err, byKey)
	}
	byID, err := s.GetSpaceAccessByID(ctx, spaceID, owner.ID)
	if err != nil || byID.MemberRole != nil || byID.Visibility != domain.VisibilityPrivate {
		t.Fatalf("by id: %v %+v", err, byID)
	}
	byPage, err := s.GetSpaceAccessByPage(ctx, pageID, member.ID)
	if err != nil || byPage.Key != "ENG" {
		t.Fatalf("by page: %v %+v", err, byPage)
	}
	if sid, err := s.GetPageSpaceID(ctx, pageID); err != nil || sid != spaceID {
		t.Fatalf("page space id: %v %v", err, sid)
	}
	if _, err := s.GetSpaceAccessByKey(ctx, "NOPE", member.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing key: %v", err)
	}
	if _, err := s.GetPageSpaceID(ctx, uuid.New()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing page: %v", err)
	}
}

func migrationsPath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "migrations")
}

func TestMigrationsDownAndUp(t *testing.T) {
	_, dsn := testutil.NewDBWithDSN(t)
	ctx := context.Background()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	p, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(migrationsPath()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.DownTo(ctx, 0); err != nil {
		t.Fatalf("down: %v", err)
	}
	var tables int
	if err := db.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name <> 'goose_db_version'`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Fatalf("%d tables left after full down", tables)
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("up again: %v", err)
	}
}

func TestPagesSchemaSearchVector(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	u := testutil.CreateUser(t, s, "p@example.test", "", false)
	spaceID := testutil.CreateSpace(t, s, testutil.SpaceOpts{Key: "DOC", CreatedBy: u.ID})
	pageID := testutil.CreatePage(t, s, spaceID, u.ID, "Deployment guide")
	if _, err := s.Pool.Exec(ctx, `UPDATE pages SET content_text = 'kubernetes rollout steps' WHERE id = $1`, pageID); err != nil {
		t.Fatal(err)
	}

	var titleHit, bodyHit bool
	err := s.Pool.QueryRow(ctx,
		`SELECT search_vector @@ websearch_to_tsquery('english', 'deployment'),
		        search_vector @@ websearch_to_tsquery('english', 'kubernetes')
		 FROM pages WHERE id = $1`, pageID).Scan(&titleHit, &bodyHit)
	if err != nil || !titleHit || !bodyHit {
		t.Fatalf("search vector: %v %v %v", err, titleHit, bodyHit)
	}
	var titleRank, bodyRank float32
	_ = s.Pool.QueryRow(ctx,
		`SELECT ts_rank(search_vector, to_tsquery('english', 'deployment')), ts_rank(search_vector, to_tsquery('english', 'kubernetes'))
		 FROM pages WHERE id = $1`, pageID).Scan(&titleRank, &bodyRank)
	if titleRank <= bodyRank {
		t.Fatalf("title (weight A) should outrank body (weight B): %v vs %v", titleRank, bodyRank)
	}

	if _, err := s.Pool.Exec(ctx, `UPDATE pages SET status = 'bogus' WHERE id = $1`, pageID); err == nil {
		t.Fatal("status check constraint missing")
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE spaces SET home_page_id = $1 WHERE id = $2`, pageID, spaceID); err != nil {
		t.Fatalf("home page fk: %v", err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE spaces SET home_page_id = $1 WHERE id = $2`, uuid.New(), spaceID); err == nil {
		t.Fatal("home_page_id fk missing")
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO spaces (id, key, name, created_by) VALUES ($1, 'lower', 'x', $2)`, uuid.New(), u.ID); err == nil {
		t.Fatal("key upper check constraint missing")
	}
}

func TestUpsertOIDCUserRequiresVerificationToLink(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	const issuer = "https://issuer.test"
	existing := testutil.CreateUser(t, s, "victim@example.test", "", true)

	if _, err := s.UpsertOIDCUser(ctx, issuer, "attacker", "victim@example.test", "Attacker", nil, false); !errors.Is(err, store.ErrEmailNotVerified) {
		t.Fatalf("unverified link must be refused: %v", err)
	}
	var n int
	_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM oidc_identities`).Scan(&n)
	if n != 0 {
		t.Fatalf("no identity may be created on refusal, got %d", n)
	}
	got, _ := s.GetUser(ctx, existing.ID)
	if got.DisplayName != existing.DisplayName {
		t.Fatalf("refused link must not modify the account: %+v", got)
	}

	fresh, err := s.UpsertOIDCUser(ctx, issuer, "newbie", "newbie@example.test", "Newbie", nil, false)
	if err != nil || fresh.ID == existing.ID {
		t.Fatalf("unverified login that creates a new account is allowed: %v %+v", err, fresh)
	}
	if _, err := s.UpsertOIDCUser(ctx, issuer, "newbie", "newbie@example.test", "Newbie", nil, false); err != nil {
		t.Fatalf("already linked identity must keep working without verification: %v", err)
	}

	linked, err := s.UpsertOIDCUser(ctx, issuer, "owner", "victim@example.test", "Owner", nil, true)
	if err != nil || linked.ID != existing.ID {
		t.Fatalf("verified link: %v %+v", err, linked)
	}
}

func TestUpsertOIDCUserNeverDemotesLastAdmin(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	const issuer = "https://issuer.test"
	no := false

	only := testutil.CreateUser(t, s, "only@example.test", "", true)
	if _, err := s.UpsertOIDCUser(ctx, issuer, "only", "only@example.test", "Only", nil, true); err != nil {
		t.Fatal(err)
	}
	got, err := s.UpsertOIDCUser(ctx, issuer, "only", "only@example.test", "Only", &no, true)
	if err != nil || !got.IsSystemAdmin || got.ID != only.ID {
		t.Fatalf("existing identity: last admin must be kept: %v %+v", err, got)
	}

	other := testutil.CreateUser(t, s, "other@example.test", "", true)
	got, err = s.UpsertOIDCUser(ctx, issuer, "other", "other@example.test", "Other", &no, true)
	if err != nil || got.IsSystemAdmin || got.ID != other.ID {
		t.Fatalf("link by email with a spare admin must demote: %v %+v", err, got)
	}
	got, err = s.UpsertOIDCUser(ctx, issuer, "only", "only@example.test", "Only", &no, true)
	if err != nil || !got.IsSystemAdmin {
		t.Fatalf("now the only admin again: must be kept: %v %+v", err, got)
	}
	if n, _ := s.CountActiveSystemAdmins(ctx); n != 1 {
		t.Fatalf("active admins = %d", n)
	}
}

func TestAdminUpdateUserConcurrentDemotions(t *testing.T) {
	s := testutil.NewDB(t)
	ctx := context.Background()
	a := testutil.CreateUser(t, s, "a@example.test", "", true)
	b := testutil.CreateUser(t, s, "b@example.test", "", true)
	no := false

	for round := 0; round < 25; round++ {
		if _, err := s.Pool.Exec(ctx, `UPDATE users SET is_system_admin = true, disabled_at = NULL`); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		errs := make([]error, 2)
		start := make(chan struct{})
		for i, id := range []uuid.UUID{a.ID, b.ID} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				if i == 0 {
					_, errs[i] = s.AdminUpdateUser(ctx, id, store.UserUpdate{IsSystemAdmin: &no})
				} else {
					disabled := true
					_, errs[i] = s.AdminUpdateUser(ctx, id, store.UserUpdate{Disabled: &disabled})
				}
			}()
		}
		close(start)
		wg.Wait()

		ok, last := 0, 0
		for _, err := range errs {
			switch {
			case err == nil:
				ok++
			case errors.Is(err, store.ErrLastAdmin):
				last++
			default:
				t.Fatalf("round %d: unexpected error (deadlock?): %v", round, err)
			}
		}
		if ok != 1 || last != 1 {
			t.Fatalf("round %d: ok=%d last=%d errs=%v", round, ok, last, errs)
		}
		if n, _ := s.CountActiveSystemAdmins(ctx); n != 1 {
			t.Fatalf("round %d: active admins = %d", round, n)
		}
	}
}
