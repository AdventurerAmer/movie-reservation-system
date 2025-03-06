package internal

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// User represents a user in the system
type User struct {
	ID           int64     `json:"id"`           // ID
	CreatedAt    time.Time `json:"created_at"`   // CreatedAt
	Name         string    `json:"name"`         // Name
	Email        string    `json:"email"`        // Email
	PasswordHash []byte    `json:"-"`            // PasswordHash
	IsActivated  bool      `json:"is_activated"` // IsActivated
	Version      int32     `json:"-"`            // Version
}

type UserStorer interface {
	Create(name string, email string, passswordHash []byte) (*User, error)
	GetByID(id int64) (*User, error)
	GetByEmail(email string) (*User, error)
	Update(*User) error
	Delete(*User) error
}

type userStorage struct {
	queryTimeout time.Duration
	db           *sql.DB
}

func (s userStorage) Create(name string, email string, passswordHash []byte) (*User, error) {
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

func (s userStorage) GetByID(id int64) (*User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	var u User
	u.ID = id

	query := `SELECT created_at, name, email, password_hash, is_activated, version
	          FROM users
			  WHERE id = $1`
	args := []any{id}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&u.CreatedAt, &u.Name, &u.Email, &u.PasswordHash, &u.IsActivated, &u.Version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &u, err
}

func (s userStorage) GetByEmail(email string) (*User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	u := User{
		Email: email,
	}

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

func (s userStorage) Update(u *User) error {
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

func (s userStorage) Delete(u *User) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	query := `DELETE FROM users 
			  WHERE id = $1`
	args := []any{u.ID}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}
