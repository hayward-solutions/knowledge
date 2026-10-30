package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
)

type DashboardPage struct {
	PageSummary
	SpaceKey  string          `json:"space_key"`
	SpaceName string          `json:"space_name"`
	UpdatedAt time.Time       `json:"updated_at"`
	Author    *domain.UserRef `json:"author"`
	authorID  uuid.UUID
}

type Dashboard struct {
	RecentUpdates  []DashboardPage `json:"recent_updates"`
	RecentlyViewed []DashboardPage `json:"recently_viewed"`
	MyDrafts       []DashboardPage `json:"my_drafts"`
	Starred        []DashboardPage `json:"starred"`
}

const (
	dashboardRecentLimit = 20
	dashboardViewedLimit = 10
	dashboardDraftsLimit = 20
	dashboardStarsLimit  = 20
)

const dashboardBase = `SELECT p.id, p.title, p.slug, p.parent_id, p.position, p.status,
		EXISTS (SELECT 1 FROM pages c WHERE c.parent_id = p.id AND c.status <> 'trashed'),
		s.key, s.name, %s, p.updated_by
	FROM pages p
	JOIN spaces s ON s.id = p.space_id AND s.deleted_at IS NULL
	JOIN users me ON me.id = $1
	LEFT JOIN space_members m ON m.space_id = s.id AND m.user_id = $1
	%s
	WHERE (me.is_system_admin OR m.user_id IS NOT NULL OR s.visibility = 'internal')
	  AND p.status <> 'trashed'
	  AND (p.status <> 'draft' OR p.created_by = $1 OR me.is_system_admin OR m.role = 'admin')
	  AND %s
	ORDER BY %s
	LIMIT %d`

func dashboardQuery(tsExpr, join, where, order string, limit int) string {
	return fmt.Sprintf(dashboardBase, tsExpr, join, where, order, limit)
}

func (s *Store) dashboardSection(ctx context.Context, sql string, userID uuid.UUID) ([]DashboardPage, error) {
	rows, err := s.Pool.Query(ctx, sql, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DashboardPage{}
	for rows.Next() {
		var d DashboardPage
		var status string
		if err := rows.Scan(&d.ID, &d.Title, &d.Slug, &d.ParentID, &d.Position, &status, &d.HasChildren,
			&d.SpaceKey, &d.SpaceName, &d.UpdatedAt, &d.authorID); err != nil {
			return nil, err
		}
		d.Status = domain.PageStatus(status)
		d.UpdatedAt = d.UpdatedAt.UTC()
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) GetDashboard(ctx context.Context, userID uuid.UUID) (Dashboard, error) {
	var d Dashboard
	var err error
	d.RecentUpdates, err = s.dashboardSection(ctx, dashboardQuery(
		"p.updated_at", "", "p.status = 'published'", "p.updated_at DESC, p.id DESC", dashboardRecentLimit), userID)
	if err != nil {
		return d, err
	}
	d.RecentlyViewed, err = s.dashboardSection(ctx, dashboardQuery(
		"p.updated_at", "JOIN page_views v ON v.page_id = p.id AND v.user_id = $1", "TRUE",
		"v.viewed_at DESC, p.id DESC", dashboardViewedLimit), userID)
	if err != nil {
		return d, err
	}
	d.MyDrafts, err = s.dashboardSection(ctx, dashboardQuery(
		"COALESCE(pd.updated_at, p.updated_at)",
		"LEFT JOIN page_drafts pd ON pd.page_id = p.id AND pd.user_id = $1",
		"((p.status = 'draft' AND p.created_by = $1) OR pd.user_id IS NOT NULL)",
		"COALESCE(pd.updated_at, p.updated_at) DESC, p.id DESC", dashboardDraftsLimit), userID)
	if err != nil {
		return d, err
	}
	d.Starred, err = s.dashboardSection(ctx, dashboardQuery(
		"p.updated_at", "JOIN page_stars st ON st.page_id = p.id AND st.user_id = $1", "TRUE",
		"st.created_at DESC, p.id DESC", dashboardStarsLimit), userID)
	if err != nil {
		return d, err
	}

	ids := map[uuid.UUID]struct{}{}
	for _, list := range [][]DashboardPage{d.RecentUpdates, d.RecentlyViewed, d.Starred} {
		for _, p := range list {
			ids[p.authorID] = struct{}{}
		}
	}
	ids[userID] = struct{}{}
	idList := make([]uuid.UUID, 0, len(ids))
	for id := range ids {
		idList = append(idList, id)
	}
	refs, err := s.GetUserRefs(ctx, idList)
	if err != nil {
		return d, err
	}
	for _, list := range [][]DashboardPage{d.RecentUpdates, d.RecentlyViewed, d.Starred} {
		for i := range list {
			if r, ok := refs[list[i].authorID]; ok {
				ref := r
				list[i].Author = &ref
			}
		}
	}
	if me, ok := refs[userID]; ok {
		for i := range d.MyDrafts {
			ref := me
			d.MyDrafts[i].Author = &ref
		}
	}
	return d, nil
}
