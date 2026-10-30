package handlers

import "github.com/go-chi/chi/v5"

type LabelHandler struct {
	Deps
}

func (h *LabelHandler) Routes(r chi.Router) {}
