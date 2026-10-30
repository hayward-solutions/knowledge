package handlers

import "github.com/go-chi/chi/v5"

type SpaceHandler struct {
	Deps
}

func (h *SpaceHandler) Routes(r chi.Router) {}
