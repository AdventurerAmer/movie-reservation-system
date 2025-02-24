package internal

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

type TokenScope int16

const (
	TokenScopeActivation TokenScope = iota
	TokenScopeAuthentication
	TokenScopePasswordReset
)

func (s TokenScope) String() string {
	switch s {
	case TokenScopeActivation:
		return "Activation"
	case TokenScopeAuthentication:
		return "Authentication"
	case TokenScopePasswordReset:
		return "PasswordReset"
	}
	return fmt.Sprintf("TokenScope %d", s)
}

func GenerateToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	token := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
	return token
}

func HashToken(token string) []byte {
	hash := sha256.Sum256([]byte(token))
	return hash[:]
}

type Token struct {
	ID        int64      `json:"-"`
	UserID    int64      `json:"user_id"`
	Scope     TokenScope `json:"scope"`
	Hash      []byte     `json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
}

type TokenStorer interface {
	Create(userID int64, scope TokenScope, token string, duration time.Duration) (*Token, error)
	GetUser(scope TokenScope, token string) (*User, error)
	DeleteAll(userID int64, scopes []TokenScope) error
	DeleteAllExpired() (int, error)
}

type tokenStorage struct {
	queryTimeout time.Duration
	db           *sql.DB
}

func (s tokenStorage) Create(userID int64, scope TokenScope, token string, expires_after time.Duration) (*Token, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	t := Token{
		UserID:    userID,
		Scope:     scope,
		Hash:      HashToken(token),
		ExpiresAt: time.Now().Add(expires_after),
	}

	query := `INSERT INTO tokens(user_id, scope_id, hash, expires_at)
	          VALUES ($1, $2, $3, $4)
			  RETURNING id`
	args := []any{userID, scope, t.Hash, t.ExpiresAt}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&t.ID)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s tokenStorage) GetUser(scope TokenScope, token string) (*User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	var u User

	query := `SELECT u.id, u.created_at, u.name, u.email, u.password_hash, u.is_activated, u.version
	          FROM tokens as t
			  INNER JOIN users as u
			  ON t.user_id = u.id
			  WHERE t.scope_id = $1 AND t.hash = $2 AND expires_at > NOW()`

	args := []any{scope, HashToken(token)}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&u.ID, &u.CreatedAt, &u.Name, &u.Email, &u.PasswordHash, &u.IsActivated, &u.Version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

func (s tokenStorage) DeleteAll(userID int64, scopes []TokenScope) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	query := `DELETE FROM tokens
	          WHERE user_id = $1 AND scope_id = ANY($2)`

	args := []any{userID, pq.Array(scopes)}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s tokenStorage) DeleteAllExpired() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	query := `DELETE FROM tokens
	          WHERE NOW() > expires_at`

	result, err := s.db.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}
