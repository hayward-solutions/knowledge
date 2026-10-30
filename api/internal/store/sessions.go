package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
)

const maxSessionTokenLen = 128

func NewSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func HashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Store) CreateSession(ctx context.Context, userID uuid.UUID, expiresAt time.Time) (string, error) {
	token, err := NewSessionToken()
	if err != nil {
		return "", err
	}
	_, err = s.Pool.Exec(ctx,
		`INSERT INTO sessions (id, user_id, expires_at) VALUES ($1, $2, $3)`,
		HashSessionToken(token), userID, expiresAt)
	if err != nil {
		return "", err
	}
	return token, nil
}

func (s *Store) GetValidSession(ctx context.Context, token string) (domain.Session, error) {
	var sess domain.Session
	if token == "" || len(token) > maxSessionTokenLen {
		return sess, ErrNotFound
	}
	err := s.Pool.QueryRow(ctx,
		`SELECT id, user_id, expires_at, created_at FROM sessions WHERE id = $1 AND expires_at > now()`,
		HashSessionToken(token)).
		Scan(&sess.ID, &sess.UserID, &sess.ExpiresAt, &sess.CreatedAt)
	return sess, notFoundIf(err)
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, HashSessionToken(token))
	return err
}

func (s *Store) DeleteUserSessions(ctx context.Context, userID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	return err
}

func (s *Store) DeleteOtherUserSessions(ctx context.Context, userID uuid.UUID, keepToken string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1 AND id <> $2`, userID, HashSessionToken(keepToken))
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= now()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
