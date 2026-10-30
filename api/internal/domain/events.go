package domain

import "github.com/google/uuid"

type PagePublished struct {
	PageID           uuid.UUID
	SpaceID          uuid.UUID
	ActorID          uuid.UUID
	Version          int
	MentionedUserIDs []uuid.UUID
}

type PageMoved struct {
	PageID      uuid.UUID
	SpaceID     uuid.UUID
	ActorID     uuid.UUID
	OldParentID *uuid.UUID
	NewParentID *uuid.UUID
}

type PageTrashed struct {
	PageID  uuid.UUID
	SpaceID uuid.UUID
	ActorID uuid.UUID
}

type PageRestored struct {
	PageID  uuid.UUID
	SpaceID uuid.UUID
	ActorID uuid.UUID
}

type CommentCreated struct {
	CommentID        uuid.UUID
	PageID           uuid.UUID
	SpaceID          uuid.UUID
	ActorID          uuid.UUID
	ParentID         *uuid.UUID
	Inline           bool
	MentionedUserIDs []uuid.UUID
}

type CommentResolved struct {
	CommentID uuid.UUID
	PageID    uuid.UUID
	SpaceID   uuid.UUID
	ActorID   uuid.UUID
}

type SpaceMemberAdded struct {
	SpaceID uuid.UUID
	UserID  uuid.UUID
	ActorID uuid.UUID
	Role    Role
}

type NotificationCreated struct {
	UserID         uuid.UUID
	NotificationID uuid.UUID
}
