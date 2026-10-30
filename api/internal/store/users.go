package store

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
)

const userCols = `id, email, display_name, is_system_admin, disabled_at, created_at, updated_at`

func scanUser(row pgx.Row) (domain.User, error) {
	var u domain.User
	err := row.Scan(&u.ID, &u.Email, &u.DisplayName, &u.IsSystemAdmin, &u.DisabledAt, &u.CreatedAt, &u.UpdatedAt)
	return u, notFoundIf(err)
}

func (s *Store) CreateUser(ctx context.Context, u domain.User, passwordHash string) error {
	var pw *string
	if passwordHash != "" {
		pw = &passwordHash
	}
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, display_name, is_system_admin)
		 VALUES ($1, $2, $3, $4, $5)`,
		u.ID, u.Email, pw, u.DisplayName, u.IsSystemAdmin)
	if IsUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) CountActiveSystemAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx,
		`SELECT count(*) FROM users WHERE is_system_admin AND disabled_at IS NULL`).Scan(&n)
	return n, err
}

func (s *Store) GetUser(ctx context.Context, id uuid.UUID) (domain.User, error) {
	return scanUser(s.Pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id))
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (domain.User, error) {
	var u domain.User
	err := s.Pool.QueryRow(ctx,
		`SELECT `+userCols+`, password_hash FROM users WHERE email = $1`, email).
		Scan(&u.ID, &u.Email, &u.DisplayName, &u.IsSystemAdmin, &u.DisabledAt, &u.CreatedAt, &u.UpdatedAt, &u.PasswordHash)
	return u, notFoundIf(err)
}

func (s *Store) GetUserWithHash(ctx context.Context, id uuid.UUID) (domain.User, error) {
	var u domain.User
	err := s.Pool.QueryRow(ctx,
		`SELECT `+userCols+`, password_hash FROM users WHERE id = $1`, id).
		Scan(&u.ID, &u.Email, &u.DisplayName, &u.IsSystemAdmin, &u.DisabledAt, &u.CreatedAt, &u.UpdatedAt, &u.PasswordHash)
	return u, notFoundIf(err)
}

func (s *Store) ListUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+userCols+` FROM users ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

func (s *Store) SearchUserRefs(ctx context.Context, q string, limit int) ([]domain.UserRef, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	pattern := "%" + escapeLike(strings.TrimSpace(q)) + "%"
	rows, err := s.Pool.Query(ctx,
		`SELECT id, display_name, email FROM users
		 WHERE disabled_at IS NULL
		   AND (display_name ILIKE $1 OR email::text ILIKE $1)
		 ORDER BY display_name, id
		 LIMIT $2`, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.UserRef{}
	for rows.Next() {
		var r domain.UserRef
		if err := rows.Scan(&r.ID, &r.DisplayName, &r.Email); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetUserRef(ctx context.Context, id uuid.UUID) (domain.UserRef, error) {
	var r domain.UserRef
	err := s.Pool.QueryRow(ctx, `SELECT id, display_name, email FROM users WHERE id = $1`, id).
		Scan(&r.ID, &r.DisplayName, &r.Email)
	return r, notFoundIf(err)
}

func (s *Store) GetUserRefs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.UserRef, error) {
	out := map[uuid.UUID]domain.UserRef{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT id, display_name, email FROM users WHERE id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r domain.UserRef
		if err := rows.Scan(&r.ID, &r.DisplayName, &r.Email); err != nil {
			return nil, err
		}
		out[r.ID] = r
	}
	return out, rows.Err()
}

type UserUpdate struct {
	Email         *string
	DisplayName   *string
	IsSystemAdmin *bool
	Disabled      *bool
}

func (s *Store) UpdateUserProfile(ctx context.Context, id uuid.UUID, email, displayName *string) (domain.User, error) {
	return s.AdminUpdateUser(ctx, id, UserUpdate{Email: email, DisplayName: displayName})
}

func (s *Store) AdminUpdateUser(ctx context.Context, id uuid.UUID, upd UserUpdate) (domain.User, error) {
	var out domain.User
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		target, err := scanUser(tx.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1 FOR UPDATE`, id))
		if err != nil {
			return err
		}

		demoting := upd.IsSystemAdmin != nil && !*upd.IsSystemAdmin && target.IsSystemAdmin
		disabling := upd.Disabled != nil && *upd.Disabled && target.IsSystemAdmin && target.DisabledAt == nil
		if (demoting || disabling) && target.DisabledAt == nil {
			rows, err := tx.Query(ctx,
				`SELECT id FROM users WHERE is_system_admin AND disabled_at IS NULL FOR UPDATE`)
			if err != nil {
				return err
			}
			n := 0
			for rows.Next() {
				n++
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
			if n <= 1 {
				return ErrLastAdmin
			}
		}

		sets := []string{"updated_at = now()"}
		args := []any{id}
		add := func(col string, v any) {
			args = append(args, v)
			sets = append(sets, col+" = $"+strconv.Itoa(len(args)))
		}
		if upd.Email != nil {
			add("email", *upd.Email)
		}
		if upd.DisplayName != nil {
			add("display_name", *upd.DisplayName)
		}
		if upd.IsSystemAdmin != nil {
			add("is_system_admin", *upd.IsSystemAdmin)
		}
		if upd.Disabled != nil {
			if *upd.Disabled {
				if target.DisabledAt == nil {
					add("disabled_at", time.Now().UTC())
				}
			} else {
				sets = append(sets, "disabled_at = NULL")
			}
		}
		out, err = scanUser(tx.QueryRow(ctx,
			`UPDATE users SET `+strings.Join(sets, ", ")+` WHERE id = $1 RETURNING `+userCols, args...))
		if IsUniqueViolation(err) {
			return ErrConflict
		}
		return err
	})
	return out, err
}

func (s *Store) SetUserPassword(ctx context.Context, id uuid.UUID, hash string) error {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1`, id, hash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpsertOIDCUser(ctx context.Context, issuer, subject, email, displayName string, isSystemAdmin *bool) (domain.User, error) {
	var u domain.User
	email = strings.ToLower(strings.TrimSpace(email))
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		var userID uuid.UUID
		err := tx.QueryRow(ctx,
			`SELECT user_id FROM oidc_identities WHERE issuer = $1 AND subject = $2`, issuer, subject).Scan(&userID)
		switch {
		case err == nil:
			_, err = tx.Exec(ctx,
				`UPDATE users SET
				   display_name = $2,
				   email = CASE WHEN EXISTS (SELECT 1 FROM users o WHERE o.email = $3 AND o.id <> $1) THEN email ELSE $3 END,
				   is_system_admin = COALESCE($4, is_system_admin),
				   updated_at = now()
				 WHERE id = $1`,
				userID, displayName, email, isSystemAdmin)
			if err != nil {
				return err
			}
		case errors.Is(err, pgx.ErrNoRows):
			err = tx.QueryRow(ctx, `SELECT id FROM users WHERE email = $1 FOR UPDATE`, email).Scan(&userID)
			switch {
			case err == nil:
				_, err = tx.Exec(ctx,
					`UPDATE users SET display_name = $2, is_system_admin = COALESCE($3, is_system_admin), updated_at = now()
					 WHERE id = $1`, userID, displayName, isSystemAdmin)
				if err != nil {
					return err
				}
			case errors.Is(err, pgx.ErrNoRows):
				userID = uuid.Must(uuid.NewV7())
				admin := false
				if isSystemAdmin != nil {
					admin = *isSystemAdmin
				}
				if _, err = tx.Exec(ctx,
					`INSERT INTO users (id, email, display_name, is_system_admin) VALUES ($1, $2, $3, $4)`,
					userID, email, displayName, admin); err != nil {
					return err
				}
			default:
				return err
			}
			if _, err = tx.Exec(ctx,
				`INSERT INTO oidc_identities (user_id, issuer, subject) VALUES ($1, $2, $3)
				 ON CONFLICT (issuer, subject) DO NOTHING`, userID, issuer, subject); err != nil {
				return err
			}
		default:
			return err
		}
		u, err = scanUser(tx.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, userID))
		return err
	})
	return u, err
}

func (s *Store) FirstSystemAdminID(ctx context.Context) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.Pool.QueryRow(ctx,
		`SELECT id FROM users WHERE is_system_admin AND disabled_at IS NULL ORDER BY created_at, id LIMIT 1`).Scan(&id)
	return id, notFoundIf(err)
}
