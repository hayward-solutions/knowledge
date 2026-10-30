package handlers

import "github.com/go-chi/chi/v5"

type TemplateHandler struct {
	Deps
}

func (h *TemplateHandler) Routes(r chi.Router) {}
