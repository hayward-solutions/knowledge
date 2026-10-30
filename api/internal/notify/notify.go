package notify

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/content"
	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/events"
	"github.com/hayward-solutions/knowledge/api/internal/store"
)

const (
	TypePagePublished    = "page.published"
	TypePageMentioned    = "page.mentioned"
	TypeCommentCreated   = "comment.created"
	TypeCommentReplied   = "comment.replied"
	TypeCommentMentioned = "comment.mentioned"
	TypeCommentResolved  = "comment.resolved"
	TypeSpaceAdded       = "space.added"

	ActivityPagePublished  = "page.published"
	ActivityPageMoved      = "page.moved"
	ActivityPageTrashed    = "page.trashed"
	ActivityPageRestored   = "page.restored"
	ActivityCommentCreated = "comment.created"

	EventNotification = "notification"
	EventUnreadCount  = "unread_count"

	ExcerptLength  = 140
	handlerTimeout = 30 * time.Second
)

type Service struct {
	store *store.Store
	bus   *events.Bus
	hub   *events.Hub
}

func New(s *store.Store, bus *events.Bus, hub *events.Hub) *Service {
	return &Service{store: s, bus: bus, hub: hub}
}

func (s *Service) Subscribe() {
	if s == nil || s.bus == nil || s.store == nil {
		return
	}
	s.bus.Subscribe(s.Handle)
}

func (s *Service) Handle(e any) {
	ctx, cancel := context.WithTimeout(context.Background(), handlerTimeout)
	defer cancel()
	var err error
	switch ev := e.(type) {
	case domain.PagePublished:
		err = s.pagePublished(ctx, ev)
	case domain.PageMoved:
		err = s.pageMoved(ctx, ev)
	case domain.PageTrashed:
		err = s.pageSimpleActivity(ctx, ev.PageID, ev.SpaceID, ev.ActorID, ActivityPageTrashed)
	case domain.PageRestored:
		err = s.pageSimpleActivity(ctx, ev.PageID, ev.SpaceID, ev.ActorID, ActivityPageRestored)
	case domain.CommentCreated:
		err = s.commentCreated(ctx, ev)
	case domain.CommentResolved:
		err = s.commentResolved(ctx, ev)
	case domain.SpaceMemberAdded:
		err = s.spaceMemberAdded(ctx, ev)
	default:
		return
	}
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		log.Printf("notify: handling %T: %v", e, err)
	}
}

type recipients struct {
	actor uuid.UUID
	order []uuid.UUID
	types map[uuid.UUID]string
}

func newRecipients(actor uuid.UUID) *recipients {
	return &recipients{actor: actor, types: map[uuid.UUID]string{}}
}

func (r *recipients) add(id uuid.UUID, typ string) {
	if id == r.actor || id == uuid.Nil {
		return
	}
	if _, ok := r.types[id]; ok {
		return
	}
	r.types[id] = typ
	r.order = append(r.order, id)
}

type delivery struct {
	spaceID   uuid.UUID
	pageID    *uuid.UUID
	commentID *uuid.UUID
	actorID   uuid.UUID
	payload   map[string]any
}

func (s *Service) actorName(ctx context.Context, id uuid.UUID) string {
	ref, err := s.store.GetUserRef(ctx, id)
	if err != nil {
		return ""
	}
	return ref.DisplayName
}

func (s *Service) deliver(ctx context.Context, rs *recipients, d delivery) error {
	if len(rs.order) == 0 {
		return nil
	}
	allowed, err := s.store.FilterNotifiable(ctx, d.spaceID, d.pageID, rs.order)
	if err != nil {
		return err
	}
	ok := make(map[uuid.UUID]struct{}, len(allowed))
	for _, id := range allowed {
		ok[id] = struct{}{}
	}
	payload := map[string]any{"actor_name": s.actorName(ctx, d.actorID)}
	for k, v := range d.payload {
		payload[k] = v
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var firstErr error
	for _, uid := range rs.order {
		if _, allowedUser := ok[uid]; !allowedUser {
			continue
		}
		space := d.spaceID
		n := domain.Notification{
			ID:        uuid.Must(uuid.NewV7()),
			UserID:    uid,
			Type:      rs.types[uid],
			ActorID:   d.actorID,
			SpaceID:   &space,
			PageID:    d.pageID,
			CommentID: d.commentID,
			Payload:   raw,
		}
		if err := s.store.InsertNotification(ctx, n); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		s.publish(ctx, n)
	}
	return firstErr
}

func (s *Service) publish(ctx context.Context, n domain.Notification) {
	if s.hub != nil {
		view, err := s.store.GetNotificationView(ctx, n.ID)
		if err == nil {
			s.hub.Publish(n.UserID, events.Event{Kind: EventNotification, Payload: view})
		}
		if count, err := s.store.CountUnreadNotifications(ctx, n.UserID); err == nil {
			s.hub.Publish(n.UserID, events.Event{Kind: EventUnreadCount, Payload: map[string]int{"count": count}})
		}
	}
	if s.bus != nil {
		s.bus.Publish(domain.NotificationCreated{UserID: n.UserID, NotificationID: n.ID})
	}
}

func (s *Service) recordActivity(ctx context.Context, spaceID uuid.UUID, pageID *uuid.UUID, actorID uuid.UUID, typ string, payload map[string]any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.store.InsertActivity(ctx, domain.Activity{
		ID:      uuid.Must(uuid.NewV7()),
		SpaceID: spaceID,
		PageID:  pageID,
		ActorID: actorID,
		Type:    typ,
		Payload: raw,
	})
}

func Excerpt(c domain.Comment) string {
	text := content.ExtractText(c.BodyJSON)
	if strings.TrimSpace(text) == "" {
		text = content.ExtractTextFromHTML(c.BodyHTML)
	}
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) > ExcerptLength {
		runes = runes[:ExcerptLength]
	}
	return strings.TrimSpace(string(runes))
}

func (s *Service) pagePublished(ctx context.Context, ev domain.PagePublished) error {
	page, err := s.store.GetNotifyPageInfo(ctx, ev.PageID)
	if err != nil {
		return err
	}
	message, _ := s.store.GetPublishMessage(ctx, ev.PageID, ev.Version)
	activity := map[string]any{"title": page.Title, "page_title": page.Title, "version": ev.Version}
	if message != "" {
		activity["message"] = message
	}
	pageID := ev.PageID
	if err := s.recordActivity(ctx, ev.SpaceID, &pageID, ev.ActorID, ActivityPagePublished, activity); err != nil {
		return err
	}
	if ev.Version == 1 && page.CreatedBy == ev.ActorID {
		if err := s.store.SetWatch(ctx, ev.ActorID, domain.WatchTargetPage, ev.PageID); err != nil {
			log.Printf("notify: auto-watch page: %v", err)
		}
	}

	rs := newRecipients(ev.ActorID)
	for _, id := range ev.MentionedUserIDs {
		rs.add(id, TypePageMentioned)
	}
	watchers, err := s.store.ListPublishWatcherIDs(ctx, ev.PageID, ev.SpaceID)
	if err != nil {
		return err
	}
	for _, id := range watchers {
		rs.add(id, TypePagePublished)
	}
	payload := map[string]any{
		"page_title": page.Title,
		"page_slug":  page.Slug,
		"space_key":  page.SpaceKey,
		"version":    ev.Version,
	}
	if message != "" {
		payload["message"] = message
	}
	return s.deliver(ctx, rs, delivery{
		spaceID: ev.SpaceID,
		pageID:  &pageID,
		actorID: ev.ActorID,
		payload: payload,
	})
}

func (s *Service) pageMoved(ctx context.Context, ev domain.PageMoved) error {
	page, err := s.store.GetNotifyPageInfo(ctx, ev.PageID)
	if err != nil {
		return err
	}
	pageID := ev.PageID
	return s.recordActivity(ctx, ev.SpaceID, &pageID, ev.ActorID, ActivityPageMoved, map[string]any{
		"title":         page.Title,
		"page_title":    page.Title,
		"old_parent_id": ev.OldParentID,
		"new_parent_id": ev.NewParentID,
	})
}

func (s *Service) pageSimpleActivity(ctx context.Context, pageID, spaceID, actorID uuid.UUID, typ string) error {
	page, err := s.store.GetNotifyPageInfo(ctx, pageID)
	if err != nil {
		return err
	}
	return s.recordActivity(ctx, spaceID, &pageID, actorID, typ, map[string]any{"title": page.Title, "page_title": page.Title})
}

func (s *Service) commentCreated(ctx context.Context, ev domain.CommentCreated) error {
	comment, err := s.store.GetComment(ctx, ev.CommentID)
	if err != nil {
		return err
	}
	page, err := s.store.GetNotifyPageInfo(ctx, ev.PageID)
	if err != nil {
		return err
	}
	excerpt := Excerpt(comment)
	pageID := ev.PageID
	commentID := ev.CommentID
	err = s.recordActivity(ctx, ev.SpaceID, &pageID, ev.ActorID, ActivityCommentCreated, map[string]any{
		"title":           page.Title,
		"page_title":      page.Title,
		"comment_id":      ev.CommentID,
		"excerpt":         excerpt,
		"comment_excerpt": excerpt,
		"inline":          ev.Inline,
		"reply":           ev.ParentID != nil,
	})
	if err != nil {
		return err
	}
	if err := s.store.SetWatch(ctx, ev.ActorID, domain.WatchTargetPage, ev.PageID); err != nil {
		log.Printf("notify: auto-watch page: %v", err)
	}

	rs := newRecipients(ev.ActorID)
	for _, id := range ev.MentionedUserIDs {
		rs.add(id, TypeCommentMentioned)
	}
	if ev.ParentID != nil {
		parent, err := s.store.GetComment(ctx, *ev.ParentID)
		if err == nil {
			rs.add(parent.AuthorID, TypeCommentReplied)
		} else if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		participants, err := s.store.ListCommentThreadAuthorIDs(ctx, ev.CommentID)
		if err != nil {
			return err
		}
		for _, id := range participants {
			rs.add(id, TypeCommentReplied)
		}
	}
	watchers, err := s.store.ListPageWatcherIDs(ctx, ev.PageID)
	if err != nil {
		return err
	}
	for _, id := range watchers {
		rs.add(id, TypeCommentCreated)
	}
	return s.deliver(ctx, rs, delivery{
		spaceID:   ev.SpaceID,
		pageID:    &pageID,
		commentID: &commentID,
		actorID:   ev.ActorID,
		payload: map[string]any{
			"page_title":      page.Title,
			"page_slug":       page.Slug,
			"space_key":       page.SpaceKey,
			"comment_excerpt": excerpt,
			"inline":          ev.Inline,
		},
	})
}

func (s *Service) commentResolved(ctx context.Context, ev domain.CommentResolved) error {
	comment, err := s.store.GetComment(ctx, ev.CommentID)
	if err != nil {
		return err
	}
	page, err := s.store.GetNotifyPageInfo(ctx, ev.PageID)
	if err != nil {
		return err
	}
	rs := newRecipients(ev.ActorID)
	rs.add(comment.AuthorID, TypeCommentResolved)
	pageID := ev.PageID
	commentID := ev.CommentID
	return s.deliver(ctx, rs, delivery{
		spaceID:   ev.SpaceID,
		pageID:    &pageID,
		commentID: &commentID,
		actorID:   ev.ActorID,
		payload: map[string]any{
			"page_title":      page.Title,
			"page_slug":       page.Slug,
			"space_key":       page.SpaceKey,
			"comment_excerpt": Excerpt(comment),
			"inline":          len(comment.InlineAnchor) > 0 && string(comment.InlineAnchor) != "null",
		},
	})
}

func (s *Service) spaceMemberAdded(ctx context.Context, ev domain.SpaceMemberAdded) error {
	key, name, err := s.store.GetNotifySpaceInfo(ctx, ev.SpaceID)
	if err != nil {
		return err
	}
	rs := newRecipients(ev.ActorID)
	rs.add(ev.UserID, TypeSpaceAdded)
	return s.deliver(ctx, rs, delivery{
		spaceID: ev.SpaceID,
		actorID: ev.ActorID,
		payload: map[string]any{
			"space_key":  key,
			"space_name": name,
			"role":       string(ev.Role),
		},
	})
}
