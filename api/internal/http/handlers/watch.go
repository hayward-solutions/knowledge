package handlers

import "github.com/go-chi/chi/v5"

type WatchHandler struct {
	Deps
}

func (h *WatchHandler) Routes(r chi.Router) {}
