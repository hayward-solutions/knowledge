package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/events"
	mw "github.com/hayward-solutions/knowledge/api/internal/http/middleware"
	"github.com/hayward-solutions/knowledge/api/internal/notify"
	"github.com/hayward-solutions/knowledge/api/internal/store"
)

const (
	notificationStreamHeartbeat = 30 * time.Second
	notificationMaxReadIDs      = 500
)

type NotificationHandler struct {
	Deps
	Heartbeat time.Duration
}

func (h *NotificationHandler) Routes(r chi.Router) {
	auth := mw.RequireAuth(h.Store)
	r.With(auth).Get("/notifications", h.List)
	r.With(auth).Post("/notifications/read", h.MarkRead)
	r.With(auth).Get("/notifications/stream", h.Stream)
}

func (h *NotificationHandler) Subscribe() {
	notify.New(h.Store, h.Bus, h.Hub).Subscribe()
}

func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	u := mw.UserFrom(r.Context())
	f := store.NotificationFilter{UserID: u.ID, Limit: queryInt(r, "limit", 30, 1, 100)}
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("unread"))) {
	case "":
	case "true", "1":
		v := true
		f.Unread = &v
	case "false", "0":
		v := false
		f.Unread = &v
	default:
		httpErr(w, http.StatusBadRequest, "invalid unread")
		return
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("before")); raw != "" {
		t, err := time.Parse(time.RFC3339Nano, strings.ReplaceAll(raw, " ", "+"))
		if err != nil {
			httpErr(w, http.StatusBadRequest, "invalid before")
			return
		}
		f.Before = &t
	}
	items, err := h.Store.ListNotifications(r.Context(), f)
	if err != nil {
		internalErr(w, r, err, "failed to load notifications")
		return
	}
	count, err := h.Store.CountUnreadNotifications(r.Context(), u.ID)
	if err != nil {
		internalErr(w, r, err, "failed to load notifications")
		return
	}
	w.Header().Set("X-Unread-Count", strconv.Itoa(count))
	w.Header().Add("Access-Control-Expose-Headers", "X-Unread-Count")
	writeJSON(w, http.StatusOK, items)
}

func (h *NotificationHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	u := mw.UserFrom(r.Context())
	var body struct {
		IDs []string `json:"ids"`
		All bool     `json:"all"`
	}
	if err := readJSON(r, &body); err != nil {
		httpErr(w, http.StatusBadRequest, "bad body")
		return
	}
	if body.All {
		if _, err := h.Store.MarkAllNotificationsRead(r.Context(), u.ID); err != nil {
			internalErr(w, r, err, "failed to mark notifications read")
			return
		}
	} else {
		if len(body.IDs) == 0 {
			httpErr(w, http.StatusBadRequest, "ids or all is required")
			return
		}
		if len(body.IDs) > notificationMaxReadIDs {
			httpErr(w, http.StatusBadRequest, "too many ids")
			return
		}
		ids := make([]uuid.UUID, 0, len(body.IDs))
		for _, raw := range body.IDs {
			id, err := uuid.Parse(raw)
			if err != nil {
				httpErr(w, http.StatusBadRequest, "invalid id")
				return
			}
			ids = append(ids, id)
		}
		if _, err := h.Store.MarkNotificationsRead(r.Context(), u.ID, ids); err != nil {
			internalErr(w, r, err, "failed to mark notifications read")
			return
		}
	}
	h.publishUnreadCount(r, u.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *NotificationHandler) publishUnreadCount(r *http.Request, userID uuid.UUID) {
	if h.Hub == nil {
		return
	}
	count, err := h.Store.CountUnreadNotifications(r.Context(), userID)
	if err != nil {
		return
	}
	h.Hub.Publish(userID, notificationUnreadEvent(count))
}

func (h *NotificationHandler) Stream(w http.ResponseWriter, r *http.Request) {
	u := mw.UserFrom(r.Context())
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	if h.Hub == nil {
		httpErr(w, http.StatusServiceUnavailable, "streaming unavailable")
		return
	}
	ch, unsubscribe := h.Hub.Subscribe(u.ID)
	defer unsubscribe()

	count, err := h.Store.CountUnreadNotifications(r.Context(), u.ID)
	if err != nil {
		internalErr(w, r, err, "failed to open stream")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	hello, _ := json.Marshal(map[string]int{"unread_count": count})
	fmt.Fprintf(w, "event: hello\ndata: %s\n\n", hello)
	flusher.Flush()

	interval := h.Heartbeat
	if interval <= 0 {
		interval = notificationStreamHeartbeat
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !h.sessionStillValid(r) {
				return
			}
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case ev, open := <-ch:
			if !open {
				return
			}
			data, err := json.Marshal(ev.Payload)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Kind, data)
			flusher.Flush()
		}
	}
}

func (h *NotificationHandler) sessionStillValid(r *http.Request) bool {
	token, ok := mw.SessionTokenFrom(r.Context())
	if !ok {
		return false
	}
	sess, err := h.Store.GetValidSession(r.Context(), token)
	if err != nil {
		return false
	}
	user, err := h.Store.GetUser(r.Context(), sess.UserID)
	if err != nil || user.DisabledAt != nil {
		return false
	}
	return true
}

func notificationUnreadEvent(count int) events.Event {
	return events.Event{Kind: notify.EventUnreadCount, Payload: map[string]int{"count": count}}
}
