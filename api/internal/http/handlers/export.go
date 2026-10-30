package handlers

import "github.com/go-chi/chi/v5"

type ExportHandler struct {
	Deps
}

func (h *ExportHandler) Routes(r chi.Router) {}
