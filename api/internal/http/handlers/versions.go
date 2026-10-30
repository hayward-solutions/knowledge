package handlers

import "github.com/go-chi/chi/v5"

type VersionHandler struct {
	Deps
}

func (h *VersionHandler) Routes(r chi.Router) {}
