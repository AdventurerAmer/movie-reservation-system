package internal

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"
)

type Seat struct {
	ID          int32  `json:"id"`
	Coordinates string `json:"coordinates"`
	HallID      int32  `json:"hall_id"`
	Version     int32  `json:"version"`
}

type SeatStorer interface {
	Create(hallID int32, coordinates string) (*Seat, error)
	Get(id int32) (*Seat, error)
	GetAll(hallID int32) ([]Seat, error)
	GetWithCinemaAndHall(seatID int32) (*Cinema, *Hall, *Seat, error)
	Update(seat *Seat) error
	Delete(seat *Seat) error
}

type seatStorage struct {
	queryTimeout time.Duration
	db           *sql.DB
}

func (s seatStorage) Create(hallID int32, coordinates string) (*Seat, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	seat := Seat{
		HallID:      hallID,
		Coordinates: coordinates,
	}
	query := `INSERT INTO seats(hall_id, coordinates)
	          VALUES ($1, $2)
			  RETURNING id, version`
	args := []any{hallID, coordinates}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&seat.ID, &seat.Version)
	if err != nil {
		return nil, err
	}
	return &seat, nil
}

func (s seatStorage) Get(id int32) (*Seat, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	seat := Seat{
		ID: id,
	}
	query := `SELECT hall_id, coordinates, version
	          FROM seats
			  WHERE id = $1`
	args := []any{id}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&seat.HallID, &seat.Coordinates, &seat.Version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &seat, nil
}

func (s seatStorage) GetAll(hallID int32) ([]Seat, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `SELECT id, coordinates, version
	          FROM seats
			  WHERE hall_id = $1
			  ORDER BY coordinates ASC, id ASC`
	args := []any{hallID}
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
	var seats []Seat
	for rows.Next() {
		seat := Seat{
			HallID: hallID,
		}
		err = rows.Scan(&seat.ID, &seat.Coordinates, &seat.Version)
		if err != nil {
			return nil, err
		}
		seats = append(seats, seat)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return seats, nil
}

func (s seatStorage) GetWithCinemaAndHall(seatID int32) (*Cinema, *Hall, *Seat, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	seat := Seat{
		ID: seatID,
	}
	var h Hall
	var c Cinema
	query := `SELECT s.hall_id, s.coordinates, s.version,
	          h.name, h.cinema_id, h.seat_arrangement, h.seat_price, h.version,
			  c.id, c.location, c.owner_id, c.version
	          FROM seats as s
			  INNER JOIN halls as h
			  ON s.hall_id = h.id
			  INNER JOIN cinemas as c
			  ON c.id = h.cinema_id
			  WHERE s.id = $1`
	args := []any{seatID}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&seat.HallID, &seat.Coordinates, &seat.Version, &h.Name, &h.CinemaID, &h.SeatArrangement, &h.SeatPrice, &h.Version, &c.ID, &c.Location, &c.OwnerID, &c.Version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, nil, nil
		}
		return nil, nil, nil, err
	}
	return &c, &h, &seat, nil
}

func (s seatStorage) Update(seat *Seat) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `UPDATE seats
	          SET coordinates = $1, version = version + 1
			  WHERE id = $2 AND version = $3
			  RETURNING version`
	args := []any{seat.Coordinates, seat.ID, seat.Version}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&seat.Version)
	return err
}

func (s seatStorage) Delete(seat *Seat) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `DELETE FROM seats
			  WHERE id = $1`
	args := []any{seat.ID, seat.Version}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}
