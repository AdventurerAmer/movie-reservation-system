package internal

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

type Movie struct {
	ID        int64    `json:"id"`
	CreatedAt string   `json:"created_at"`
	Title     string   `json:"title"`
	Runtime   int32    `json:"runtime"`
	Year      int32    `json:"year"`
	Genres    []string `json:"genres"`
	Version   int32    `json:"version"`
}

type MovieStorer interface {
	Create(title string, runtime int32, year int32, genres []string) (*Movie, error)
	GetByID(id int64) (*Movie, error)
	GetAll(title string, genres []string, page, pageSize int, sort string) ([]Movie, *MetaData, error)
	Update(m *Movie) error
	Delete(m *Movie) error
}

type movieStorage struct {
	queryTimeout time.Duration
	db           *sql.DB
}

func (s movieStorage) Create(title string, runtime int32, year int32, genres []string) (*Movie, error) {
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

func (s movieStorage) GetByID(id int64) (*Movie, error) {
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

func (s movieStorage) GetAll(title string, genres []string, page, pageSize int, sort string) ([]Movie, *MetaData, error) {
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

func (s movieStorage) Update(m *Movie) error {
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

func (s movieStorage) Delete(m *Movie) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `DELETE FROM movies
	          WHERE id = $1 AND version = $2`
	args := []any{m.ID, m.Version}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}
