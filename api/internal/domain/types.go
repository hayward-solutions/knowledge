package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const SessionTTL = 30 * 24 * time.Hour

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleEditor Role = "editor"
	RoleViewer Role = "viewer"
)

func (r Role) Valid() bool {
	switch r {
	case RoleAdmin, RoleEditor, RoleViewer:
		return true
	}
	return false
}

func (r Role) rank() int {
	switch r {
	case RoleAdmin:
		return 3
	case RoleEditor:
		return 2
	case RoleViewer:
		return 1
	}
	return 0
}

func (r Role) AtLeast(min Role) bool { return r.rank() >= min.rank() && r.Valid() }

type Visibility string

const (
	VisibilityPrivate  Visibility = "private"
	VisibilityInternal Visibility = "internal"
)

func (v Visibility) Valid() bool { return v == VisibilityPrivate || v == VisibilityInternal }

type PageStatus string

const (
	PageStatusDraft     PageStatus = "draft"
	PageStatusPublished PageStatus = "published"
	PageStatusTrashed   PageStatus = "trashed"
)

func (s PageStatus) Valid() bool {
	switch s {
	case PageStatusDraft, PageStatusPublished, PageStatusTrashed:
		return true
	}
	return false
}

type WatchTarget string

const (
	WatchTargetPage  WatchTarget = "page"
	WatchTargetSpace WatchTarget = "space"
)

func (t WatchTarget) Valid() bool { return t == WatchTargetPage || t == WatchTargetSpace }

type UserRef struct {
	ID          uuid.UUID `json:"id"`
	DisplayName string    `json:"display_name"`
	Email       string    `json:"email"`
}

type User struct {
	ID            uuid.UUID  `json:"id"`
	Email         string     `json:"email"`
	DisplayName   string     `json:"display_name"`
	PasswordHash  *string    `json:"-"`
	IsSystemAdmin bool       `json:"is_system_admin"`
	DisabledAt    *time.Time `json:"disabled_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (u User) Ref() UserRef {
	return UserRef{ID: u.ID, DisplayName: u.DisplayName, Email: u.Email}
}

type Session struct {
	ID        string    `json:"-"`
	UserID    uuid.UUID `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type OIDCIdentity struct {
	UserID  uuid.UUID `json:"user_id"`
	Issuer  string    `json:"issuer"`
	Subject string    `json:"subject"`
}

type Space struct {
	ID          uuid.UUID  `json:"id"`
	Key         string     `json:"key"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Icon        *string    `json:"icon"`
	Visibility  Visibility `json:"visibility"`
	HomePageID  *uuid.UUID `json:"home_page_id"`
	CreatedBy   uuid.UUID  `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ArchivedAt  *time.Time `json:"archived_at"`
	DeletedAt   *time.Time `json:"deleted_at"`
}

type SpaceMember struct {
	SpaceID   uuid.UUID `json:"space_id"`
	UserID    uuid.UUID `json:"user_id"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type Page struct {
	ID          uuid.UUID       `json:"id"`
	SpaceID     uuid.UUID       `json:"space_id"`
	ParentID    *uuid.UUID      `json:"parent_id"`
	Title       string          `json:"title"`
	Slug        string          `json:"slug"`
	Position    int             `json:"position"`
	Status      PageStatus      `json:"status"`
	Version     int             `json:"version"`
	ContentJSON json.RawMessage `json:"content_json"`
	ContentHTML string          `json:"content_html"`
	ContentText string          `json:"-"`
	CreatedBy   uuid.UUID       `json:"created_by"`
	UpdatedBy   uuid.UUID       `json:"updated_by"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	PublishedAt *time.Time      `json:"published_at"`
	TrashedAt   *time.Time      `json:"trashed_at"`
	TrashedBy   *uuid.UUID      `json:"trashed_by"`
}

type PageVersion struct {
	ID          uuid.UUID       `json:"id"`
	PageID      uuid.UUID       `json:"page_id"`
	Version     int             `json:"version"`
	Title       string          `json:"title"`
	ContentJSON json.RawMessage `json:"content_json"`
	ContentHTML string          `json:"content_html"`
	AuthorID    uuid.UUID       `json:"author_id"`
	Message     *string         `json:"message"`
	CreatedAt   time.Time       `json:"created_at"`
}

type PageDraft struct {
	PageID      uuid.UUID       `json:"page_id"`
	UserID      uuid.UUID       `json:"user_id"`
	Title       string          `json:"title"`
	ContentJSON json.RawMessage `json:"content_json"`
	ContentHTML string          `json:"content_html"`
	BaseVersion int             `json:"base_version"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type PageView struct {
	UserID   uuid.UUID `json:"user_id"`
	PageID   uuid.UUID `json:"page_id"`
	ViewedAt time.Time `json:"viewed_at"`
}

type PageStar struct {
	UserID    uuid.UUID `json:"user_id"`
	PageID    uuid.UUID `json:"page_id"`
	CreatedAt time.Time `json:"created_at"`
}

type Label struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type PageLabel struct {
	PageID  uuid.UUID `json:"page_id"`
	LabelID uuid.UUID `json:"label_id"`
}

type InlineAnchor struct {
	QuotedText string `json:"quoted_text"`
}

type Comment struct {
	ID           uuid.UUID       `json:"id"`
	PageID       uuid.UUID       `json:"page_id"`
	ParentID     *uuid.UUID      `json:"parent_id"`
	AuthorID     uuid.UUID       `json:"author_id"`
	BodyJSON     json.RawMessage `json:"body_json"`
	BodyHTML     string          `json:"body_html"`
	InlineAnchor json.RawMessage `json:"inline_anchor"`
	ResolvedAt   *time.Time      `json:"resolved_at"`
	ResolvedBy   *uuid.UUID      `json:"resolved_by"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	DeletedAt    *time.Time      `json:"deleted_at"`
}

type Attachment struct {
	ID          uuid.UUID  `json:"id"`
	PageID      uuid.UUID  `json:"page_id"`
	SpaceID     uuid.UUID  `json:"space_id"`
	UploaderID  uuid.UUID  `json:"uploader_id"`
	Filename    string     `json:"filename"`
	ContentType string     `json:"content_type"`
	Size        int64      `json:"size"`
	S3Key       string     `json:"-"`
	CreatedAt   time.Time  `json:"created_at"`
	DeletedAt   *time.Time `json:"deleted_at"`
}

type Template struct {
	ID          uuid.UUID       `json:"id"`
	SpaceID     *uuid.UUID      `json:"space_id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	ContentJSON json.RawMessage `json:"content_json"`
	ContentHTML string          `json:"content_html"`
	CreatedBy   uuid.UUID       `json:"created_by"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type Watch struct {
	UserID     uuid.UUID   `json:"user_id"`
	TargetType WatchTarget `json:"target_type"`
	TargetID   uuid.UUID   `json:"target_id"`
	CreatedAt  time.Time   `json:"created_at"`
}

type Notification struct {
	ID        uuid.UUID       `json:"id"`
	UserID    uuid.UUID       `json:"user_id"`
	Type      string          `json:"type"`
	ActorID   uuid.UUID       `json:"actor_id"`
	SpaceID   *uuid.UUID      `json:"space_id"`
	PageID    *uuid.UUID      `json:"page_id"`
	CommentID *uuid.UUID      `json:"comment_id"`
	Payload   json.RawMessage `json:"payload"`
	ReadAt    *time.Time      `json:"read_at"`
	CreatedAt time.Time       `json:"created_at"`
}

type Activity struct {
	ID        uuid.UUID       `json:"id"`
	SpaceID   uuid.UUID       `json:"space_id"`
	PageID    *uuid.UUID      `json:"page_id"`
	ActorID   uuid.UUID       `json:"actor_id"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}
