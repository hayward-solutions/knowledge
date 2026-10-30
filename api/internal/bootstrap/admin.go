package bootstrap

import (
	"context"
	"log"
	"strings"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/auth"
	"github.com/hayward-solutions/knowledge/api/internal/config"
	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/store"
)

func Admin(ctx context.Context, s *store.Store, cfg config.AdminSettings) error {
	n, err := s.CountUsers(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		log.Printf("admin bootstrap skipped: users already exist")
		return nil
	}
	email := strings.ToLower(strings.TrimSpace(cfg.Email))
	if email == "" || cfg.Password == "" {
		log.Printf("admin bootstrap skipped: ADMIN_EMAIL / ADMIN_PASSWORD not set")
		return nil
	}
	name := cfg.DisplayName
	if name == "" {
		name = "Admin"
	}
	hash, err := auth.HashPassword(cfg.Password)
	if err != nil {
		return err
	}
	u := domain.User{
		ID:            uuid.Must(uuid.NewV7()),
		Email:         email,
		DisplayName:   name,
		IsSystemAdmin: true,
	}
	if err := s.CreateUser(ctx, u, hash); err != nil {
		return err
	}
	log.Printf("admin bootstrapped: %s (%s)", u.Email, u.ID)
	return nil
}
