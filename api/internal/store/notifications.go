package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
)

type NotificationView struct {
	domain.Notification
	Actor     *domain.UserRef `json:"actor"`
	SpaceKey  *string         `json:"space_key"`
	PageTitle *string         `json:"page_title"`
}

type NotificationFilter struct {
	UserID uuid.UUID
	Unread *bool
	Before *time.Time
	Limit  int
}

func notifyPayload(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

func (s *Store) InsertNotification(ctx context.Context, n domain.Notification) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO notifications (id, user_id, type, actor_id, space_id, page_id, comment_id, payload, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, COALESCE($9, now()))`,
		n.ID, n.UserID, n.Type, n.ActorID, n.SpaceID, n.PageID, n.CommentID, []byte(notifyPayload(n.Payload)), zeroTimeNil(n.CreatedAt))
	return err
}

func zeroTimeNil(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

const notificationViewSelect = `SELECT n.id, n.user_id, n.type, n.actor_id, n.space_id, n.page_id, n.comment_id,
		n.payload, n.read_at, n.created_at,
		u.id, u.display_name, u.email,
		sp.key,
		n.payload->>'page_title'
	 FROM notifications n
	 LEFT JOIN users u ON u.id = n.actor_id
	 LEFT JOIN spaces sp ON sp.id = n.space_id`

func scanNotificationView(row pgx.Row) (NotificationView, error) {
	var v NotificationView
	var uid *uuid.UUID
	var name, email *string
	err := row.Scan(&v.ID, &v.UserID, &v.Type, &v.ActorID, &v.SpaceID, &v.PageID, &v.CommentID,
		&v.Payload, &v.ReadAt, &v.CreatedAt, &uid, &name, &email, &v.SpaceKey, &v.PageTitle)
	if err != nil {
		return v, notFoundIf(err)
	}
	v.CreatedAt = v.CreatedAt.UTC()
	if v.ReadAt != nil {
		t := v.ReadAt.UTC()
		v.ReadAt = &t
	}
	if uid != nil && name != nil && email != nil {
		v.Actor = &domain.UserRef{ID: *uid, DisplayName: *name, Email: *email}
	}
	if len(v.Payload) == 0 {
		v.Payload = json.RawMessage(`{}`)
	}
	return v, nil
}

func (s *Store) GetNotificationView(ctx context.Context, id uuid.UUID) (NotificationView, error) {
	return scanNotificationView(s.Pool.QueryRow(ctx, notificationViewSelect+` WHERE n.id = $1`, id))
}

func (s *Store) ListNotifications(ctx context.Context, f NotificationFilter) ([]NotificationView, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 30
	}
	var before any
	if f.Before != nil {
		before = *f.Before
	}
	var unread any
	if f.Unread != nil {
		unread = *f.Unread
	}
	rows, err := s.Pool.Query(ctx,
		notificationViewSelect+`
		 WHERE n.user_id = $1
		   AND ($2::boolean IS NULL OR (n.read_at IS NULL) = $2::boolean)
		   AND ($3::timestamptz IS NULL OR n.created_at < $3::timestamptz)
		 ORDER BY n.created_at DESC, n.id DESC
		 LIMIT $4`, f.UserID, unread, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NotificationView{}
	for rows.Next() {
		v, err := scanNotificationView(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) CountUnreadNotifications(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx,
		`SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`, userID).Scan(&n)
	return n, err
}

func (s *Store) MarkNotificationsRead(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	tag, err := s.Pool.Exec(ctx,
		`UPDATE notifications SET read_at = now()
		 WHERE user_id = $1 AND id = ANY($2::uuid[]) AND read_at IS NULL`, userID, ids)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Store) MarkAllNotificationsRead(ctx context.Context, userID uuid.UUID) (int64, error) {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE notifications SET read_at = now() WHERE user_id = $1 AND read_at IS NULL`, userID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

type NotifyPageInfo struct {
	ID        uuid.UUID
	SpaceID   uuid.UUID
	SpaceKey  string
	SpaceName string
	Title     string
	Slug      string
	Status    domain.PageStatus
	CreatedBy uuid.UUID
	Version   int
}

func (s *Store) GetNotifyPageInfo(ctx context.Context, pageID uuid.UUID) (NotifyPageInfo, error) {
	var i NotifyPageInfo
	var status string
	err := s.Pool.QueryRow(ctx,
		`SELECT p.id, p.space_id, s.key, s.name, p.title, p.slug, p.status, p.created_by, p.version
		 FROM pages p JOIN spaces s ON s.id = p.space_id WHERE p.id = $1`, pageID).
		Scan(&i.ID, &i.SpaceID, &i.SpaceKey, &i.SpaceName, &i.Title, &i.Slug, &status, &i.CreatedBy, &i.Version)
	if err != nil {
		return i, notFoundIf(err)
	}
	i.Status = domain.PageStatus(status)
	return i, nil
}

func (s *Store) GetNotifySpaceInfo(ctx context.Context, spaceID uuid.UUID) (key, name string, err error) {
	err = s.Pool.QueryRow(ctx, `SELECT key, name FROM spaces WHERE id = $1`, spaceID).Scan(&key, &name)
	return key, name, notFoundIf(err)
}

func (s *Store) GetPublishMessage(ctx context.Context, pageID uuid.UUID, version int) (string, error) {
	var msg *string
	err := s.Pool.QueryRow(ctx,
		`SELECT message FROM page_versions WHERE page_id = $1 AND version = $2`, pageID, version).Scan(&msg)
	if err != nil {
		return "", notFoundIf(err)
	}
	if msg == nil {
		return "", nil
	}
	return *msg, nil
}

func (s *Store) ListCommentThreadAuthorIDs(ctx context.Context, commentID uuid.UUID) ([]uuid.UUID, error) {
	return s.watcherIDs(ctx,
		`WITH RECURSIVE up(id, depth) AS (
			SELECT id, 0 FROM comments WHERE id = $1
			UNION
			SELECT c.parent_id, up.depth + 1 FROM comments c JOIN up ON c.id = up.id
			WHERE c.parent_id IS NOT NULL AND up.depth < 1000
		),
		root AS (
			SELECT up.id FROM up JOIN comments c ON c.id = up.id WHERE c.parent_id IS NULL
		),
		down(id, depth) AS (
			SELECT id, 0 FROM root
			UNION
			SELECT c.id, down.depth + 1 FROM comments c JOIN down ON c.parent_id = down.id WHERE down.depth < 1000
		)
		SELECT DISTINCT c.author_id FROM comments c JOIN down ON c.id = down.id
		WHERE c.deleted_at IS NULL
		ORDER BY c.author_id`, commentID)
}

func (s *Store) ListUnreadPageMentionUserIDs(ctx context.Context, pageID uuid.UUID, userIDs []uuid.UUID) ([]uuid.UUID, error) {
	if len(userIDs) == 0 {
		return []uuid.UUID{}, nil
	}
	return s.watcherIDs(ctx,
		`SELECT DISTINCT user_id FROM notifications
		 WHERE type = 'page.mentioned' AND page_id = $1 AND read_at IS NULL AND user_id = ANY($2::uuid[])
		 ORDER BY user_id`, pageID, userIDs)
}
