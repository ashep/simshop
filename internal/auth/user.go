package auth

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
)

const (
	ScopeAdmin Scope = "admin"
)

type Scope string

type User struct {
	ID     string
	APIKey string
	Scopes []Scope
}

func (u *User) IsAdmin() bool {
	return slices.Contains(u.Scopes, ScopeAdmin)
}

func (s *Service) GetUserByAPIKey(ctx context.Context, key string) (*User, error) {
	const q = `
		SELECT u.id, u.api_key, array_remove(array_agg(up.scope), NULL)
		FROM users u
		LEFT JOIN user_permissions up ON up.user_id = u.id
		WHERE u.api_key = $1
		GROUP BY u.id, u.api_key`

	var u User
	var scopes []string

	err := s.db.QueryRow(ctx, q, key).Scan(&u.ID, &u.APIKey, &scopes)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	} else if err != nil {
		return nil, err
	}

	u.Scopes = make([]Scope, len(scopes))
	for i, sc := range scopes {
		u.Scopes[i] = Scope(sc)
	}

	return &u, nil
}
