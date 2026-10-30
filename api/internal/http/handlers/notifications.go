package handlers

import "github.com/go-chi/chi/v5"

type NotificationHandler struct {
	Deps
}

func (h *NotificationHandler) Routes(r chi.Router) {}

func (h *NotificationHandler) Subscribe() {}
