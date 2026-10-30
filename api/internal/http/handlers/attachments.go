package handlers

import "github.com/go-chi/chi/v5"

type AttachmentHandler struct {
	Deps
}

func (h *AttachmentHandler) Routes(r chi.Router) {}
