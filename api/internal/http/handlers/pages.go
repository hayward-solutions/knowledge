package handlers

import "github.com/go-chi/chi/v5"

type PageHandler struct {
	Deps
}

func (h *PageHandler) Routes(r chi.Router) {}
