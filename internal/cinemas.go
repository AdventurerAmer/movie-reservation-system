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
)

type Cinema struct {
	ID       int32  `json:"id"`
	Name     string `json:"name"`
	Location string `json:"location"`
	OwnerID  int64  `json:"ower_id"`
	Version  int32  `json:"version"`
}

type CinemaStorer interface {
	Create(ownerID int64, name string, location string) (*Cinema, error)
	GetByID(id int32) (*Cinema, error)
	GetAll(name string, location string, page, pageSize int, sort string) ([]Cinema, *MetaData, error)
	Update(c *Cinema) error
	Delete(c *Cinema) error
}

type cinemaStorage struct {
	queryTimeout time.Duration
	db           *sql.DB
}

func (s cinemaStorage) Create(ownerID int64, name string, location string) (*Cinema, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	c := Cinema{
		OwnerID:  ownerID,
		Name:     name,
		Location: location,
	}
	query := `INSERT INTO cinemas(owner_id, name, location)
	          VALUES ($1, $2, $3)
			  RETURNING id, version`
	args := []any{ownerID, name, location}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&c.ID, &c.Version)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s cinemaStorage) GetByID(id int32) (*Cinema, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	c := Cinema{
		ID: id,
	}
	query := `SELECT name, location, owner_id, version 
	          FROM cinemas
			  WHERE id = $1`
	args := []any{id}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&c.Name, &c.Location, &c.OwnerID, &c.Version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (s cinemaStorage) GetAll(name string, location string, page, pageSize int, sort string) ([]Cinema, *MetaData, error) {
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
	FROM cinemas
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

func (s cinemaStorage) Update(c *Cinema) error {
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

func (s cinemaStorage) Delete(c *Cinema) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `DELETE FROM cinemas 
			  WHERE id = $1`
	args := []any{c.ID}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}
