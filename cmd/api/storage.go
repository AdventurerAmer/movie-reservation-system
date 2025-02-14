package main

import (
	"context"
	"database/sql"
	"errors"
	"time"

	_ "github.com/lib/pq"
)

type Storage struct {
	queryTimeout time.Duration
	db           *sql.DB
}

func NewStorage(dsn string, queryTimeout time.Duration) (*Storage, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = db.PingContext(ctx)
	if err != nil {
		return nil, err
	}
	return &Storage{db: db, queryTimeout: queryTimeout}, nil
}

func (s *Storage) CreateUser(name string, email string, passswordHash []byte) (*User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	var u User
	u.Name = name
	u.Email = email
	u.PasswordHash = passswordHash
	u.IsActivated = false

	query := `INSERT INTO users(name, email, password_hash)
	          VALUES ($1, $2, $3)
			  RETURNING id, created_at, version`
	args := []any{name, email, passswordHash}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&u.ID, &u.CreatedAt, &u.Version)
	if err != nil {
		return nil, err
	}
	return &u, err
}

func (s *Storage) GetUserByID(ID int64) (*User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	var u User
	u.ID = ID

	query := `SELECT created_at, name, email, password_hash, is_activated, version
	          FROM users
			  WHERE id = $1`
	args := []any{ID}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&u.CreatedAt, &u.Name, &u.Email, &u.PasswordHash, &u.IsActivated, &u.Version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &u, err
}

func (s *Storage) GetUserByEmail(email string) (*User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	var u User
	u.Email = email

	query := `SELECT id, created_at, name, password_hash, is_activated, version
	          FROM users
			  WHERE email = $1`
	args := []any{email}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&u.ID, &u.CreatedAt, &u.Name, &u.PasswordHash, &u.IsActivated, &u.Version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &u, err
}

func (s *Storage) UpdateUser(u *User) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `UPDATE users
	          SET name = $1, email = $2, password_hash = $3, is_activated = $4, version = version + 1
			  WHERE id = $5 AND version = $6
			  RETURNING version`
	args := []any{u.Name, u.Email, u.PasswordHash, u.IsActivated, u.ID, u.Version}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&u.Version)
	return err
}

func (s *Storage) DeleteUser(u *User) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `DELETE FROM users 
			  WHERE id = $1 AND version = $2`
	args := []any{u.ID, u.Version}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *Storage) CreateTokenForUser(userID int64, scope TokenScope, hash []byte, duration time.Duration) (*Token, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `INSERT INTO tokens(user_id, scope_id, hash, expires_at)
	          VALUES ($1, $2, $3, $4)
			  RETURNING id`
	token := Token{
		UserID:    userID,
		Scope:     scope,
		Hash:      hash,
		ExpiresAt: time.Now().Add(duration),
	}
	args := []any{userID, scope, hash, token.ExpiresAt}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&token.ID)
	if err != nil {
		return nil, err
	}
	return &token, nil
}

func (s *Storage) GetUserFromToken(scope TokenScope, hash []byte) (*User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `SELECT u.id, u.created_at, u.name, u.email, u.password_hash, u.is_activated, u.version
	          FROM tokens as t
			  INNER JOIN users as u
			  ON t.user_id = u.id
			  WHERE t.scope_id = $1 AND t.hash = $2 AND expires_at > NOW()`
	args := []any{scope, hash}
	var u User
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&u.ID, &u.CreatedAt, &u.Name, &u.Email, &u.PasswordHash, &u.IsActivated, &u.Version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}
