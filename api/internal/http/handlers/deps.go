package handlers

import (
	"github.com/hayward-solutions/knowledge/api/internal/auth"
	"github.com/hayward-solutions/knowledge/api/internal/config"
	"github.com/hayward-solutions/knowledge/api/internal/events"
	"github.com/hayward-solutions/knowledge/api/internal/storage"
	"github.com/hayward-solutions/knowledge/api/internal/store"
)

type Deps struct {
	Store    *store.Store
	Bus      *events.Bus
	Hub      *events.Hub
	Presence *events.Presence
	Storage  storage.ObjectStore
	Config   *config.Config
	OIDC     *auth.OIDCConfig
	Version  string
}
