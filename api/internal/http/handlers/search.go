package handlers

import "github.com/go-chi/chi/v5"

type SearchHandler struct {
	Deps
}

func (h *SearchHandler) Routes(r chi.Router) {}
