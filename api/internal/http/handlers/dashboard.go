package handlers

import "github.com/go-chi/chi/v5"

type DashboardHandler struct {
	Deps
}

func (h *DashboardHandler) Routes(r chi.Router) {}
