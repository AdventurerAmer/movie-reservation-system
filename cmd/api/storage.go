package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/lib/pq"
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
	db.SetConnMaxIdleTime(30 * time.Minute)
	db.SetMaxIdleConns(25)
	db.SetMaxOpenConns(25)
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

func (s *Storage) CreateTokenForUser(userID int64, scope TokenScope, token string, duration time.Duration) (*Token, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	t := Token{
		UserID:    userID,
		Scope:     scope,
		Hash:      hashToken(token),
		ExpiresAt: time.Now().Add(duration),
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

func (s *Storage) GetUserFromToken(scope TokenScope, token string) (*User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	var u User

	query := `SELECT u.id, u.created_at, u.name, u.email, u.password_hash, u.is_activated, u.version
	          FROM tokens as t
			  INNER JOIN users as u
			  ON t.user_id = u.id
			  WHERE t.scope_id = $1 AND t.hash = $2 AND expires_at > NOW()`

	args := []any{scope, hashToken(token)}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&u.ID, &u.CreatedAt, &u.Name, &u.Email, &u.PasswordHash, &u.IsActivated, &u.Version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

func (s *Storage) DeleteAllTokensForUser(userID int64, scopes []TokenScope) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	query := `DELETE FROM tokens
	          WHERE user_id = $1 AND scope_id = ANY($2)`

	args := []any{userID, pq.Array(scopes)}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *Storage) DeleteAllExpiredTokens() (int, error) {
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

func (s *Storage) GetPermissions(userID int64) ([]Permission, error) {
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

func (s *Storage) GrantPermissions(userID int64, permissions []Permission) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	query := `INSERT INTO user_permissions
			  SELECT $1, p.id FROM permissions WHERE p.code = ANY($2)
			  ON CONFLICT DO NOTHING`

	args := []any{userID, pq.Array(permissions)}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *Storage) CreateMovie(title string, runtime int32, year int32, genres []string) (*Movie, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	m := Movie{
		Title:   title,
		Runtime: runtime,
		Year:    year,
		Genres:  genres,
	}
	query := `INSERT INTO movies(title, runtime, year, genres)
	          VALUES ($1, $2, $3, $4)
			  RETURNING id, created_at, version`
	args := []any{title, runtime, year, pq.Array(genres)}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&m.ID, &m.CreatedAt, &m.Version)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Storage) GetMovieByID(id int64) (*Movie, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	m := Movie{
		ID: id,
	}
	query := `SELECT created_at, title, runtime, year, genres, version FROM movies WHERE id = $1`
	args := []any{id}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&m.CreatedAt, &m.Title, &m.Runtime, &m.Year, pq.Array(&m.Genres), &m.Version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

func (s *Storage) GetMovies(title string, genres []string, page, pageSize int, sort string) ([]Movie, *MetaData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	op := "ASC"
	if strings.HasPrefix(sort, "-") {
		sort = strings.TrimPrefix(sort, "-")
		op = "DESC"
	}

	order := ""
	if sort == "id" {
		order = fmt.Sprintf("id %s", op)
	} else {
		order = fmt.Sprintf("%s %s, id ASC", sort, op)
	}
	query := fmt.Sprintf(`
	SELECT count(*) OVER(), id, created_at, title, year, runtime, genres, version
	FROM movies
	WHERE (to_tsvector('simple', title) @@ plainto_tsquery('simple', $1) OR $1 = '')
	AND (genres @> $2 OR $2 = '{}')
	ORDER BY %s
	LIMIT $3 OFFSET $4`, order)

	limit := pageSize
	offset := (page - 1) * pageSize
	args := []any{title, pq.Array(genres), limit, offset}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	defer func() {
		err = rows.Close()
		if err != nil {
			log.Println(err)
		}
	}()

	totalRecords := 0
	var movies []Movie

	for rows.Next() {
		var m Movie
		err := rows.Scan(&totalRecords, &m.ID, &m.CreatedAt, &m.Title, &m.Year, &m.Runtime, pq.Array(&m.Genres), &m.Version)
		if err != nil {
			return nil, nil, err
		}
		movies = append(movies, m)
	}

	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	metaData := &MetaData{}
	if totalRecords != 0 {
		metaData = &MetaData{
			CurrentPage:  page,
			PageSize:     pageSize,
			FirstPage:    1,
			LastPage:     int(math.Ceil(float64(totalRecords) / float64(pageSize))),
			TotalRecords: totalRecords,
		}
	}
	return movies, metaData, nil
}

func (s *Storage) UpdateMovie(m *Movie) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	query := `UPDATE movies
			  SET title = $1, runtime = $2, year = $3, genres = $4, version = version + 1
			  WHERE id = $5 AND version = $6
			  RETURNING version`
	args := []any{m.Title, m.Runtime, m.Year, pq.Array(m.Genres), m.ID, m.Version}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&m.Version)
	return err
}

func (s *Storage) DeleteMovie(m *Movie) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `DELETE FROM movies
	          WHERE id = $1 AND version = $2`
	args := []any{m.ID, m.Version}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *Storage) CreateCinema(ownerID int64, name string, location string) (*Cinema, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	c := Cinema{
		OwnerID:  ownerID,
		Name:     name,
		Location: location,
	}
	query := `INSERT INTO cinema(owner_id, name, location)
	          VALUES ($1, $2, $3)
			  RETURNING id, version`
	args := []any{ownerID, name, location}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&c.ID, &c.Version)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Storage) GetCinema(cinemaID int32) (*Cinema, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	c := Cinema{
		ID: cinemaID,
	}
	query := `SELECT name, location, owner_id, version 
	          FROM cinemas
			  WHERE id = $1`
	args := []any{cinemaID}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&c.Name, &c.Location, &c.OwnerID, &c.Version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (s *Storage) GetCinemas(name string, location string, page, pageSize int, sort string) ([]Cinema, *MetaData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	op := "ASC"
	if strings.HasPrefix(sort, "-") {
		sort = strings.TrimPrefix(sort, "-")
		op = "DESC"
	}

	order := ""
	if sort == "id" {
		order = fmt.Sprintf("id %s", op)
	} else {
		order = fmt.Sprintf("%s %s, id ASC", sort, op)
	}
	query := fmt.Sprintf(`
	SELECT count(*) OVER(), id, name, location, owner_id, version
	FROM movies
	WHERE (to_tsvector('simple', name) @@ plainto_tsquery('simple', $1) OR $1 = '')
	AND (to_tsvector('simple', location) @@ plainto_tsquery('simple', $2) OR $2 = '')
	ORDER BY %s
	LIMIT $3 OFFSET $4`, order)

	limit := pageSize
	offset := (page - 1) * pageSize
	args := []any{name, location, limit, offset}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	defer func() {
		err = rows.Close()
		if err != nil {
			log.Println(err)
		}
	}()

	totalRecords := 0
	var cinemas []Cinema

	for rows.Next() {
		var c Cinema
		err := rows.Scan(&totalRecords, &c.ID, &c.Name, &c.Location, &c.OwnerID, &c.Version)
		if err != nil {
			return nil, nil, err
		}
		cinemas = append(cinemas, c)
	}

	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	metaData := &MetaData{}
	if totalRecords != 0 {
		metaData = &MetaData{
			CurrentPage:  page,
			PageSize:     pageSize,
			FirstPage:    1,
			LastPage:     int(math.Ceil(float64(totalRecords) / float64(pageSize))),
			TotalRecords: totalRecords,
		}
	}
	return cinemas, metaData, nil
}

func (s *Storage) UpdateCinema(c *Cinema) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `UPDATE cinemas
	          SET name = $1, location = $2, owner_id = $3, version = version + 1
			  WHERE id = $4 AND version = $5
			  RETURNING version`
	args := []any{c.Name, c.Location, c.OwnerID, c.ID, c.Version}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&c.Version)
	return err
}

func (s *Storage) DeleteCinema(c *Cinema) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `DELETE FROM cinemas 
			  WHERE id = $1 AND version = $2`
	args := []any{c.ID, c.Version}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}
