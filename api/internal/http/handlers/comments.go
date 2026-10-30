package handlers

import "github.com/go-chi/chi/v5"

type CommentHandler struct {
	Deps
}

func (h *CommentHandler) Routes(r chi.Router) {}
