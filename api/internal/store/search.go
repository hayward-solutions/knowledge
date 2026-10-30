package store

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/search"
)

type SearchPageSummary struct {
	ID          uuid.UUID         `json:"id"`
	Title       string            `json:"title"`
	Slug        string            `json:"slug"`
	ParentID    *uuid.UUID        `json:"parent_id"`
	Position    int               `json:"position"`
	Status      domain.PageStatus `json:"status"`
	HasChildren bool              `json:"has_children"`
}

type SearchHit struct {
	Page      SearchPageSummary `json:"page"`
	SpaceKey  string            `json:"space_key"`
	Snippet   string            `json:"snippet"`
	UpdatedAt time.Time         `json:"updated_at"`
	Author    *domain.UserRef   `json:"author"`
}

type SearchResult struct {
	Hits  []SearchHit `json:"hits"`
	Total int         `json:"total"`
	Page  int         `json:"page"`
	Limit int         `json:"limit"`
}

type QuickSearchHit struct {
	Page      SearchPageSummary `json:"page"`
	SpaceKey  string            `json:"space_key"`
	SpaceName string            `json:"space_name"`
}

func (s *Store) SearchPages(ctx context.Context, p search.Params) (SearchResult, error) {
	st, err := search.Build(p)
	if err != nil {
		return SearchResult{}, err
	}
	res := SearchResult{Hits: []SearchHit{}, Page: st.Params.Page, Limit: st.Params.Limit}
	if err := s.Pool.QueryRow(ctx, st.Count.SQL, st.Count.Args...).Scan(&res.Total); err != nil {
		return SearchResult{}, err
	}
	if res.Total == 0 {
		return res, nil
	}
	rows, err := s.Pool.Query(ctx, st.List.SQL, st.List.Args...)
	if err != nil {
		return SearchResult{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var h SearchHit
		var status string
		var authorID *uuid.UUID
		var authorName, authorEmail *string
		if err := rows.Scan(&h.Page.ID, &h.Page.Title, &h.Page.Slug, &h.Page.ParentID, &h.Page.Position, &status,
			&h.Page.HasChildren, &h.SpaceKey, &h.Snippet, &h.UpdatedAt, &authorID, &authorName, &authorEmail); err != nil {
			return SearchResult{}, err
		}
		h.Page.Status = domain.PageStatus(status)
		h.Snippet = search.SafeSnippet(h.Snippet)
		if authorID != nil {
			ref := domain.UserRef{ID: *authorID}
			if authorName != nil {
				ref.DisplayName = *authorName
			}
			if authorEmail != nil {
				ref.Email = *authorEmail
			}
			h.Author = &ref
		}
		res.Hits = append(res.Hits, h)
	}
	if err := rows.Err(); err != nil {
		return SearchResult{}, err
	}
	return res, nil
}

func (s *Store) QuickSearchPages(ctx context.Context, p search.QuickParams) ([]QuickSearchHit, error) {
	q, err := search.BuildQuick(p)
	if err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, q.SQL, q.Args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []QuickSearchHit{}
	for rows.Next() {
		var h QuickSearchHit
		var status string
		if err := rows.Scan(&h.Page.ID, &h.Page.Title, &h.Page.Slug, &h.Page.ParentID, &h.Page.Position, &status,
			&h.Page.HasChildren, &h.SpaceKey, &h.SpaceName); err != nil {
			return nil, err
		}
		h.Page.Status = domain.PageStatus(status)
		out = append(out, h)
	}
	return out, rows.Err()
}
