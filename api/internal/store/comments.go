package store

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
)

type CommentView struct {
	domain.Comment
	Author  *domain.UserRef `json:"author"`
	Replies []CommentView   `json:"replies"`
}

type CommentPageInfo struct {
	PageID    uuid.UUID
	SpaceID   uuid.UUID
	Status    string
	CreatedBy uuid.UUID
}

const commentColumns = `c.id, c.page_id, c.parent_id, c.author_id, c.body_json, c.body_html, c.inline_anchor,
	c.resolved_at, c.resolved_by, c.created_at, c.updated_at, c.deleted_at`

const commentReturning = `id, page_id, parent_id, author_id, body_json, body_html, inline_anchor,
	resolved_at, resolved_by, created_at, updated_at, deleted_at`

func commentDests(c *domain.Comment) []any {
	return []any{&c.ID, &c.PageID, &c.ParentID, &c.AuthorID, &c.BodyJSON, &c.BodyHTML, &c.InlineAnchor,
		&c.ResolvedAt, &c.ResolvedBy, &c.CreatedAt, &c.UpdatedAt, &c.DeletedAt}
}

func commentJSONArg(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return []byte(raw)
}

func (s *Store) CreateComment(ctx context.Context, c domain.Comment) (domain.Comment, error) {
	row := s.Pool.QueryRow(ctx,
		`INSERT INTO comments (id, page_id, parent_id, author_id, body_json, body_html, inline_anchor)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+commentReturning,
		c.ID, c.PageID, c.ParentID, c.AuthorID, commentJSONArg(c.BodyJSON), c.BodyHTML, commentJSONArg(c.InlineAnchor))
	var out domain.Comment
	if err := row.Scan(commentDests(&out)...); err != nil {
		return domain.Comment{}, err
	}
	return out, nil
}

func (s *Store) GetComment(ctx context.Context, id uuid.UUID) (domain.Comment, error) {
	var c domain.Comment
	err := s.Pool.QueryRow(ctx, `SELECT `+commentColumns+` FROM comments c WHERE c.id = $1`, id).Scan(commentDests(&c)...)
	return c, notFoundIf(err)
}

func (s *Store) GetCommentSpaceID(ctx context.Context, commentID uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.Pool.QueryRow(ctx,
		`SELECT p.space_id FROM comments c JOIN pages p ON p.id = c.page_id WHERE c.id = $1`, commentID).Scan(&id)
	return id, notFoundIf(err)
}

func (s *Store) GetCommentDepth(ctx context.Context, id uuid.UUID) (int, error) {
	var depth int
	err := s.Pool.QueryRow(ctx,
		`WITH RECURSIVE chain(id, parent_id, depth) AS (
			SELECT id, parent_id, 0 FROM comments WHERE id = $1
			UNION ALL
			SELECT c.id, c.parent_id, chain.depth + 1 FROM comments c JOIN chain ON c.id = chain.parent_id
		)
		SELECT max(depth) FROM chain`, id).Scan(&depth)
	return depth, notFoundIf(err)
}

func (s *Store) GetCommentPageInfo(ctx context.Context, pageID uuid.UUID) (CommentPageInfo, error) {
	var info CommentPageInfo
	err := s.Pool.QueryRow(ctx,
		`SELECT id, space_id, status, created_by FROM pages WHERE id = $1`, pageID).
		Scan(&info.PageID, &info.SpaceID, &info.Status, &info.CreatedBy)
	return info, notFoundIf(err)
}

func (s *Store) GetCommentWithPage(ctx context.Context, id uuid.UUID) (domain.Comment, CommentPageInfo, error) {
	var c domain.Comment
	var info CommentPageInfo
	dests := append(commentDests(&c), &info.SpaceID, &info.Status, &info.CreatedBy)
	err := s.Pool.QueryRow(ctx,
		`SELECT `+commentColumns+`, p.space_id, p.status, p.created_by
		 FROM comments c JOIN pages p ON p.id = c.page_id
		 WHERE c.id = $1`, id).Scan(dests...)
	if err != nil {
		return c, info, notFoundIf(err)
	}
	info.PageID = c.PageID
	return c, info, nil
}

type commentNode struct {
	view     CommentView
	children []*commentNode
}

func (n *commentNode) finish() (CommentView, bool) {
	replies := make([]CommentView, 0, len(n.children))
	for _, ch := range n.children {
		if v, ok := ch.finish(); ok {
			replies = append(replies, v)
		}
	}
	v := n.view
	v.Replies = replies
	if v.DeletedAt != nil {
		if len(replies) == 0 {
			return v, false
		}
		v.BodyJSON = nil
		v.BodyHTML = ""
	}
	return v, true
}

func commentBuildTree(rows []CommentView) []CommentView {
	nodes := make(map[uuid.UUID]*commentNode, len(rows))
	order := make([]*commentNode, 0, len(rows))
	for _, v := range rows {
		n := &commentNode{view: v}
		nodes[v.ID] = n
		order = append(order, n)
	}
	var roots []*commentNode
	for _, n := range order {
		if n.view.ParentID != nil {
			if p, ok := nodes[*n.view.ParentID]; ok {
				p.children = append(p.children, n)
				continue
			}
		}
		roots = append(roots, n)
	}
	out := make([]CommentView, 0, len(roots))
	for _, r := range roots {
		if v, ok := r.finish(); ok {
			out = append(out, v)
		}
	}
	return out
}

func (s *Store) ListPageComments(ctx context.Context, pageID uuid.UUID) ([]CommentView, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+commentColumns+`, u.display_name, u.email
		 FROM comments c
		 JOIN users u ON u.id = c.author_id
		 WHERE c.page_id = $1
		 ORDER BY c.created_at, c.id`, pageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var views []CommentView
	for rows.Next() {
		var v CommentView
		ref := domain.UserRef{}
		dests := append(commentDests(&v.Comment), &ref.DisplayName, &ref.Email)
		if err := rows.Scan(dests...); err != nil {
			return nil, err
		}
		ref.ID = v.AuthorID
		v.Author = &ref
		views = append(views, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return commentBuildTree(views), nil
}

func commentFind(list []CommentView, id uuid.UUID) (CommentView, bool) {
	for _, v := range list {
		if v.ID == id {
			return v, true
		}
		if found, ok := commentFind(v.Replies, id); ok {
			return found, true
		}
	}
	return CommentView{}, false
}

func (s *Store) GetCommentView(ctx context.Context, id uuid.UUID) (CommentView, error) {
	c, err := s.GetComment(ctx, id)
	if err != nil {
		return CommentView{}, err
	}
	tree, err := s.ListPageComments(ctx, c.PageID)
	if err != nil {
		return CommentView{}, err
	}
	v, ok := commentFind(tree, id)
	if !ok {
		return CommentView{}, ErrNotFound
	}
	return v, nil
}

func (s *Store) UpdateCommentBody(ctx context.Context, id uuid.UUID, bodyJSON json.RawMessage, bodyHTML string) error {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE comments SET body_json = $2, body_html = $3, updated_at = now()
		 WHERE id = $1 AND deleted_at IS NULL`, id, commentJSONArg(bodyJSON), bodyHTML)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SoftDeleteComment(ctx context.Context, id uuid.UUID) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE comments SET deleted_at = now(), body_json = NULL, body_html = '', updated_at = now()
		 WHERE id = $1 AND deleted_at IS NULL`, id)
	return err
}

func (s *Store) ResolveComment(ctx context.Context, id, userID uuid.UUID) (bool, error) {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE comments SET resolved_at = now(), resolved_by = $2
		 WHERE id = $1 AND deleted_at IS NULL AND parent_id IS NULL AND resolved_at IS NULL`, id, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) ReopenComment(ctx context.Context, id uuid.UUID) (bool, error) {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE comments SET resolved_at = NULL, resolved_by = NULL
		 WHERE id = $1 AND deleted_at IS NULL AND parent_id IS NULL AND resolved_at IS NOT NULL`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) ListPageCommentCounts(ctx context.Context, pageIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	out := map[uuid.UUID]int{}
	if len(pageIDs) == 0 {
		return out, nil
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT page_id, count(*) FROM comments
		 WHERE page_id = ANY($1::uuid[]) AND deleted_at IS NULL
		 GROUP BY page_id`, pageIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}
