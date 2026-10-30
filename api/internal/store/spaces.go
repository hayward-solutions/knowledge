package store

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
)

const (
	maxSlugRunes          = 80
	emptyDocJSON          = `{"type":"doc","content":[{"type":"paragraph"}]}`
	emptyDocHTML          = `<p></p>`
	homePageVersionNote   = "Created"
	spaceViewSelect       = `SELECT s.id, s.key, s.name, s.description, s.icon, s.visibility, s.home_page_id, s.created_by, s.created_at, s.updated_at, s.archived_at, s.deleted_at, (SELECT count(*) FROM space_members sm WHERE sm.space_id = s.id)::int AS member_count, (SELECT count(*) FROM pages p WHERE p.space_id = s.id AND p.status = 'published')::int AS page_count`
	spaceMemberViewSelect = `SELECT m.space_id, m.user_id, m.role, m.created_at, u.id, u.display_name, u.email FROM space_members m JOIN users u ON u.id = m.user_id`
)

var ErrInvalidHomePage = errors.New("home page must be a non-trashed page in this space")

type SpaceView struct {
	domain.Space
	Role        *domain.Role `json:"role"`
	MemberCount int          `json:"member_count"`
	PageCount   int          `json:"page_count"`
}

type SpaceMemberView struct {
	SpaceID   uuid.UUID      `json:"space_id"`
	UserID    uuid.UUID      `json:"user_id"`
	User      domain.UserRef `json:"user"`
	Role      domain.Role    `json:"role"`
	CreatedAt time.Time      `json:"created_at"`
}

type SpacePageSummary struct {
	ID          uuid.UUID         `json:"id"`
	Title       string            `json:"title"`
	Slug        string            `json:"slug"`
	ParentID    *uuid.UUID        `json:"parent_id"`
	Position    int               `json:"position"`
	Status      domain.PageStatus `json:"status"`
	HasChildren bool              `json:"has_children"`
}

type SpaceTrashedPage struct {
	SpacePageSummary
	TrashedAt     time.Time       `json:"trashed_at"`
	TrashedBy     *uuid.UUID      `json:"trashed_by"`
	TrashedByUser *domain.UserRef `json:"trashed_by_user"`
}

type CreateSpaceParams struct {
	Key         string
	Name        string
	Description string
	Icon        *string
	Visibility  domain.Visibility
	CreatedBy   uuid.UUID
}

type SpaceUpdate struct {
	Name        *string
	Description *string
	SetIcon     bool
	Icon        *string
	Visibility  *domain.Visibility
	SetHomePage bool
	HomePageID  *uuid.UUID
}

type spaceRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

var reservedSlugs = map[string]bool{"edit": true, "history": true, "attachments": true}

func SlugFromTitle(title string) string {
	var b strings.Builder
	runes := 0
	pendingDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if pendingDash && b.Len() > 0 {
				b.WriteByte('-')
				runes++
			}
			pendingDash = false
			if runes >= maxSlugRunes {
				break
			}
			b.WriteRune(r)
			runes++
			continue
		}
		pendingDash = true
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "untitled"
	}
	if reservedSlugs[slug] {
		return slug + "-page"
	}
	return slug
}

func scanSpaceView(row pgx.Row) (SpaceView, error) {
	var v SpaceView
	var vis string
	err := row.Scan(&v.ID, &v.Key, &v.Name, &v.Description, &v.Icon, &vis, &v.HomePageID, &v.CreatedBy,
		&v.CreatedAt, &v.UpdatedAt, &v.ArchivedAt, &v.DeletedAt, &v.MemberCount, &v.PageCount)
	if err != nil {
		return v, notFoundIf(err)
	}
	v.Visibility = domain.Visibility(vis)
	return v, nil
}

func scanSpaceViewWithRole(row pgx.Row) (SpaceView, error) {
	var v SpaceView
	var vis string
	var role *string
	err := row.Scan(&v.ID, &v.Key, &v.Name, &v.Description, &v.Icon, &vis, &v.HomePageID, &v.CreatedBy,
		&v.CreatedAt, &v.UpdatedAt, &v.ArchivedAt, &v.DeletedAt, &v.MemberCount, &v.PageCount, &role)
	if err != nil {
		return v, notFoundIf(err)
	}
	v.Visibility = domain.Visibility(vis)
	if role != nil {
		r := domain.Role(*role)
		v.Role = &r
	}
	return v, nil
}

func (s *Store) GetSpaceView(ctx context.Context, spaceID uuid.UUID) (SpaceView, error) {
	return scanSpaceView(s.Pool.QueryRow(ctx, spaceViewSelect+` FROM spaces s WHERE s.id = $1`, spaceID))
}

func (s *Store) ListSpacesAccessible(ctx context.Context, userID uuid.UUID, isSystemAdmin bool) ([]SpaceView, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT * FROM (`+spaceViewSelect+`,
		        CASE WHEN $2::boolean THEN 'admin' WHEN m.role IS NOT NULL THEN m.role WHEN s.visibility = 'internal' THEN 'viewer' END AS eff_role
		 FROM spaces s
		 LEFT JOIN space_members m ON m.space_id = s.id AND m.user_id = $1
		 WHERE s.deleted_at IS NULL) x
		 WHERE x.eff_role IS NOT NULL
		 ORDER BY lower(x.name), x.key`, userID, isSystemAdmin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SpaceView{}
	for rows.Next() {
		v, err := scanSpaceViewWithRole(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) ListAdminSpaces(ctx context.Context) ([]SpaceView, error) {
	rows, err := s.Pool.Query(ctx, spaceViewSelect+` FROM spaces s ORDER BY s.key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SpaceView{}
	for rows.Next() {
		v, err := scanSpaceView(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) CreateSpace(ctx context.Context, p CreateSpaceParams) (SpaceView, error) {
	spaceID := uuid.Must(uuid.NewV7())
	pageID := uuid.Must(uuid.NewV7())
	versionID := uuid.Must(uuid.NewV7())
	slug := SlugFromTitle(p.Name)
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO spaces (id, key, name, description, icon, visibility, created_by)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			spaceID, p.Key, p.Name, p.Description, p.Icon, string(p.Visibility), p.CreatedBy); err != nil {
			if IsUniqueViolation(err) {
				return ErrConflict
			}
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO space_members (space_id, user_id, role) VALUES ($1, $2, 'admin')`,
			spaceID, p.CreatedBy); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO pages (id, space_id, parent_id, title, slug, position, status, version,
			                    content_json, content_html, content_text, created_by, updated_by, published_at)
			 VALUES ($1, $2, NULL, $3, $4, 0, 'published', 1, $5::jsonb, $6, '', $7, $7, now())`,
			pageID, spaceID, p.Name, slug, emptyDocJSON, emptyDocHTML, p.CreatedBy); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO page_versions (id, page_id, version, title, content_json, content_html, author_id, message)
			 VALUES ($1, $2, 1, $3, $4::jsonb, $5, $6, $7)`,
			versionID, pageID, p.Name, emptyDocJSON, emptyDocHTML, p.CreatedBy, homePageVersionNote); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE spaces SET home_page_id = $2 WHERE id = $1`, spaceID, pageID)
		return err
	})
	if err != nil {
		return SpaceView{}, err
	}
	v, err := s.GetSpaceView(ctx, spaceID)
	if err != nil {
		return v, err
	}
	admin := domain.RoleAdmin
	v.Role = &admin
	return v, nil
}

func (s *Store) UpdateSpace(ctx context.Context, spaceID uuid.UUID, u SpaceUpdate) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if u.SetHomePage && u.HomePageID != nil {
			var ok bool
			err := tx.QueryRow(ctx,
				`SELECT EXISTS (SELECT 1 FROM pages WHERE id = $1 AND space_id = $2 AND status <> 'trashed')`,
				*u.HomePageID, spaceID).Scan(&ok)
			if err != nil {
				return err
			}
			if !ok {
				return ErrInvalidHomePage
			}
		}
		var vis *string
		if u.Visibility != nil {
			v := string(*u.Visibility)
			vis = &v
		}
		tag, err := tx.Exec(ctx,
			`UPDATE spaces SET
			    name = COALESCE($2, name),
			    description = COALESCE($3, description),
			    icon = CASE WHEN $4::boolean THEN $5::text ELSE icon END,
			    visibility = COALESCE($6, visibility),
			    home_page_id = CASE WHEN $7::boolean THEN $8::uuid ELSE home_page_id END,
			    updated_at = now()
			 WHERE id = $1 AND deleted_at IS NULL`,
			spaceID, u.Name, u.Description, u.SetIcon, u.Icon, vis, u.SetHomePage, u.HomePageID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func (s *Store) SoftDeleteSpace(ctx context.Context, spaceID uuid.UUID) error {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE spaces SET deleted_at = now(), updated_at = now() WHERE id = $1 AND deleted_at IS NULL`, spaceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetSpaceArchived(ctx context.Context, spaceID uuid.UUID, archived bool) error {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE spaces SET
		    archived_at = CASE WHEN $2::boolean THEN COALESCE(archived_at, now()) ELSE NULL END,
		    updated_at = now()
		 WHERE id = $1 AND deleted_at IS NULL`, spaceID, archived)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanSpaceMemberView(row pgx.Row) (SpaceMemberView, error) {
	var m SpaceMemberView
	var role string
	err := row.Scan(&m.SpaceID, &m.UserID, &role, &m.CreatedAt, &m.User.ID, &m.User.DisplayName, &m.User.Email)
	if err != nil {
		return m, notFoundIf(err)
	}
	m.Role = domain.Role(role)
	return m, nil
}

func getSpaceMemberView(ctx context.Context, q spaceRower, spaceID, userID uuid.UUID) (SpaceMemberView, error) {
	return scanSpaceMemberView(q.QueryRow(ctx,
		spaceMemberViewSelect+` WHERE m.space_id = $1 AND m.user_id = $2`, spaceID, userID))
}

func (s *Store) GetSpaceMemberView(ctx context.Context, spaceID, userID uuid.UUID) (SpaceMemberView, error) {
	return getSpaceMemberView(ctx, s.Pool, spaceID, userID)
}

func (s *Store) ListSpaceMembers(ctx context.Context, spaceID uuid.UUID) ([]SpaceMemberView, error) {
	rows, err := s.Pool.Query(ctx,
		spaceMemberViewSelect+` WHERE m.space_id = $1 ORDER BY lower(u.display_name), u.id`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SpaceMemberView{}
	for rows.Next() {
		m, err := scanSpaceMemberView(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func lockSpaceAdmins(ctx context.Context, tx pgx.Tx, spaceID uuid.UUID) (map[uuid.UUID]struct{}, error) {
	rows, err := tx.Query(ctx,
		`SELECT user_id FROM space_members WHERE space_id = $1 AND role = 'admin' ORDER BY user_id FOR UPDATE`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	admins := map[uuid.UUID]struct{}{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		admins[id] = struct{}{}
	}
	return admins, rows.Err()
}

func (s *Store) UpsertSpaceMember(ctx context.Context, spaceID, userID uuid.UUID, role domain.Role) (SpaceMemberView, bool, error) {
	var out SpaceMemberView
	var inserted bool
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		admins, err := lockSpaceAdmins(ctx, tx, spaceID)
		if err != nil {
			return err
		}
		if _, isAdmin := admins[userID]; isAdmin && role != domain.RoleAdmin && len(admins) == 1 {
			return ErrLastAdmin
		}
		err = tx.QueryRow(ctx,
			`INSERT INTO space_members (space_id, user_id, role) VALUES ($1, $2, $3)
			 ON CONFLICT (space_id, user_id) DO UPDATE SET role = EXCLUDED.role
			 RETURNING (xmax = 0)`, spaceID, userID, string(role)).Scan(&inserted)
		if err != nil {
			if IsForeignKeyViolation(err) {
				return ErrNotFound
			}
			return err
		}
		out, err = getSpaceMemberView(ctx, tx, spaceID, userID)
		return err
	})
	if err != nil {
		return SpaceMemberView{}, false, err
	}
	return out, inserted, nil
}

func (s *Store) RemoveSpaceMember(ctx context.Context, spaceID, userID uuid.UUID) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		admins, err := lockSpaceAdmins(ctx, tx, spaceID)
		if err != nil {
			return err
		}
		if _, isAdmin := admins[userID]; isAdmin && len(admins) == 1 {
			return ErrLastAdmin
		}
		tag, err := tx.Exec(ctx, `DELETE FROM space_members WHERE space_id = $1 AND user_id = $2`, spaceID, userID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func (s *Store) ListSpaceTree(ctx context.Context, spaceID, viewerID uuid.UUID, viewerIsAdmin bool) ([]SpacePageSummary, error) {
	rows, err := s.Pool.Query(ctx,
		`WITH visible AS (
		     SELECT id, parent_id, title, slug, position, status
		     FROM pages
		     WHERE space_id = $1
		       AND (status = 'published' OR (status = 'draft' AND ($3::boolean OR created_by = $2)))
		 )
		 SELECT v.id, v.title, v.slug, v.parent_id, v.position, v.status,
		        EXISTS (SELECT 1 FROM visible c WHERE c.parent_id = v.id)
		 FROM visible v
		 ORDER BY v.parent_id NULLS FIRST, v.position, v.title, v.id`, spaceID, viewerID, viewerIsAdmin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SpacePageSummary{}
	for rows.Next() {
		var p SpacePageSummary
		var status string
		if err := rows.Scan(&p.ID, &p.Title, &p.Slug, &p.ParentID, &p.Position, &status, &p.HasChildren); err != nil {
			return nil, err
		}
		p.Status = domain.PageStatus(status)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) ListSpaceTrash(ctx context.Context, spaceID, viewerID uuid.UUID, viewerIsAdmin bool) ([]SpaceTrashedPage, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT p.id, p.title, p.slug, p.parent_id, p.position, p.status,
		        EXISTS (SELECT 1 FROM pages c WHERE c.parent_id = p.id AND c.status = 'trashed'),
		        COALESCE(p.trashed_at, p.updated_at), p.trashed_by, u.id, u.display_name, u.email
		 FROM pages p
		 LEFT JOIN users u ON u.id = p.trashed_by
		 WHERE p.space_id = $1 AND p.status = 'trashed'
		   AND (p.version > 0 OR p.created_by = $2 OR $3::boolean)
		 ORDER BY COALESCE(p.trashed_at, p.updated_at) DESC, p.id`, spaceID, viewerID, viewerIsAdmin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SpaceTrashedPage{}
	for rows.Next() {
		var p SpaceTrashedPage
		var status string
		var uid *uuid.UUID
		var name, email *string
		if err := rows.Scan(&p.ID, &p.Title, &p.Slug, &p.ParentID, &p.Position, &status, &p.HasChildren,
			&p.TrashedAt, &p.TrashedBy, &uid, &name, &email); err != nil {
			return nil, err
		}
		p.Status = domain.PageStatus(status)
		if uid != nil && name != nil && email != nil {
			p.TrashedByUser = &domain.UserRef{ID: *uid, DisplayName: *name, Email: *email}
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
