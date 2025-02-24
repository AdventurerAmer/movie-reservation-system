package internal

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"github.com/shopspring/decimal"
)

type Hall struct {
	ID              int32           `json:"id"`
	Name            string          `json:"name"`
	CinemaID        int32           `json:"cinema_id"`
	SeatArrangement string          `json:"seat_arrangement"`
	SeatPrice       decimal.Decimal `json:"seat_price"`
	Version         int32           `json:"version"`
}

type HallStorer interface {
	Create(name string, cinemaID int32, seatArrangement string, seatPrice decimal.Decimal) (*Hall, error)
	Get(id int32) (*Hall, error)
	GetCinema(hallID int32) (*Hall, *Cinema, error)
	GetAllForCinema(cinemaID int32) ([]Hall, error)
	Update(h *Hall) error
	Delete(h *Hall) error
}

type hallStorage struct {
	queryTimeout time.Duration
	db           *sql.DB
}

func (s hallStorage) Create(name string, cinemaID int32, seatArrangement string, seatPrice decimal.Decimal) (*Hall, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	h := Hall{
		Name:            name,
		CinemaID:        cinemaID,
		SeatArrangement: seatArrangement,
		SeatPrice:       seatPrice,
	}
	query := `INSERT INTO halls(name, cinema_id, seat_arrangement, seat_price)
	          VALUES ($1, $2, $3, $4)
			  RETURNING id, version`
	args := []any{name, cinemaID, seatArrangement, seatPrice}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&h.ID, &h.Version)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

func (s hallStorage) Get(id int32) (*Hall, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	h := Hall{
		ID: id,
	}
	query := `SELECT name, cinema_id, seat_arrangement, seat_price, version
			  FROM halls
	          WHERE id = $1`
	args := []any{id}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&h.Name, &h.CinemaID, &h.SeatArrangement, &h.SeatPrice, &h.Version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &h, nil
}

func (s hallStorage) GetCinema(hallID int32) (*Hall, *Cinema, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	h := Hall{
		ID: hallID,
	}
	var c Cinema
	query := `SELECT h.name, h.cinema_id, h.seat_arrangement, h.seat_price, h.version, c.id, c.location, c.owner_id, c.version
			  FROM halls as h
			  INNER JOIN cinemas as c
			  ON c.id = h.cinema_id
	          WHERE h.id = $1`
	args := []any{hallID}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&h.Name, &h.CinemaID, &h.SeatArrangement, &h.SeatPrice, &h.Version, &c.ID, &c.Location, &c.OwnerID, &c.Version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	return &h, &c, err
}

func (s hallStorage) GetAllForCinema(cinemaID int32) ([]Hall, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	query := `SELECT id, name, seat_arrangement, seat_price, version
			  FROM halls
			  WHERE cinema_id = $1
			  ORDER BY name ASC, id ASC`
	args := []any{cinemaID}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	defer func() {
		err := rows.Close()
		if err != nil {
			log.Println(err)
		}
	}()

	var halls []Hall
	for rows.Next() {
		h := Hall{
			CinemaID: cinemaID,
		}
		err = rows.Scan(&h.ID, &h.Name, &h.SeatArrangement, &h.SeatPrice, &h.Version)
		if err != nil {
			return nil, err
		}
		halls = append(halls, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return halls, nil
}

func (s hallStorage) Update(h *Hall) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	query := `UPDATE halls
	          SET name = $1, seat_arrangement = $2, seat_price = $3, version = version + 1
			  WHERE id = $4 AND version = $5
			  RETURNING version`
	args := []any{h.Name, h.SeatArrangement, h.SeatPrice, h.ID, h.Version}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&h.Version)
	return err
}

func (s hallStorage) Delete(h *Hall) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	query := `DELETE FROM halls
			  WHERE id = $1`
	args := []any{h.ID, h.Version}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}
