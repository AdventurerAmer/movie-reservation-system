package internal

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"github.com/lib/pq"
)

type Permission string

type PermissionStorer interface {
	Get(userID int64) ([]Permission, error)
	Grant(userID int64, permissions []Permission) error
}

type permissionStorage struct {
	queryTimeout time.Duration
	db           *sql.DB
}

func (s permissionStorage) Get(userID int64) ([]Permission, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	query := `SELECT p.code
	          FROM permissions as p
			  INNER JOIN users_permissions as up
			  ON p.id = up.permission_id
			  WHERE up.user_id = $1`

	args := []any{userID}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	defer func() {
		err = rows.Close()
		if err != nil {
			log.Println(err)
		}
	}()
	var permissions []Permission
	for rows.Next() {
		var p Permission
		err := rows.Scan(&p)
		if err != nil {
			return nil, err
		}
		permissions = append(permissions, p)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return permissions, nil
}

func (s permissionStorage) Grant(userID int64, permissions []Permission) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	query := `INSERT INTO user_permissions
			  SELECT $1, p.id FROM permissions WHERE p.code = ANY($2)
			  ON CONFLICT DO NOTHING`

	args := []any{userID, pq.Array(permissions)}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}
