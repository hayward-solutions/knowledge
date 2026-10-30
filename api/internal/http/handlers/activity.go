package handlers

import "github.com/go-chi/chi/v5"

type ActivityHandler struct {
	Deps
}

func (h *ActivityHandler) Routes(r chi.Router) {}
