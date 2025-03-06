package internal

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

type CheckoutItem struct {
	Ticket   Ticket   `json:"ticket"`
	Schedule Schedule `json:"schedule"`
	Movie    Movie    `json:"movie"`
	Seat     Seat     `json:"seat"`
	Hall     Hall     `json:"hall"`
	Cinema   Cinema   `json:"cinema"`
}

type CheckoutSession struct {
	UserID    int64     `json:"user_id"`
	SessionID string    `json:"session_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type CheckoutStorer interface {
	GetItems(userID int64) ([]CheckoutItem, decimal.Decimal, error)
	Create(userID int64, sessionID string) (*CheckoutSession, error)
	GetByUserID(userID int64) (*CheckoutSession, error)
	GetBySessionID(sessionID string) (*CheckoutSession, error)
	DeleteByUserID(UserID int64) error
	DeleteBySessionID(sessionID string) error
	GetAllExpired(limit int64) ([]CheckoutSession, error)
	Fulfill(sessionID string, userID int64) error
}

type checkoutStorage struct {
	queryTimeout time.Duration
	db           *sql.DB
}

func (s checkoutStorage) GetItems(userID int64) ([]CheckoutItem, decimal.Decimal, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `SELECT t.id, t.created_at, t.schedule_id, t.seat_id, t.price, t.state_id, t.state_changed_at, t.version,
			  sc.id, sc.created_at, sc.movie_id, sc.hall_id, sc.price, sc.starts_at, sc.ends_at, sc.version,
	          m.id, m.created_at, m.title, m.runtime, m.year, m.genres, m.version,
			  s.id, s.hall_id, s.coordinates, s.version,
			  h.id, h.name, h.cinema_id, h.seat_arrangement, h.seat_price, h.version,
			  c.id, c.name, c.location, c.owner_id, c.version
			  FROM tickets_users as tu
			  INNER JOIN tickets as t
			  ON t.id = tu.ticket_id
			  INNER JOIN schedules as sc
			  ON t.schedule_id = sc.id
			  INNER JOIN movies as m
			  ON sc.movie_id = m.id
			  INNER JOIN seats as s
			  ON s.id = t.seat_id
			  INNER JOIN halls as h
			  ON h.id = s.hall_id
			  INNER JOIN cinemas as c
			  ON c.id = h.cinema_id
			  WHERE tu.user_id = $1 AND NOW() < sc.starts_at`
	args := []any{userID}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, decimal.Zero, nil
		}
		return nil, decimal.Zero, err
	}
	defer func() {
		err := rows.Close()
		if err != nil {
			log.Println(err)
		}
	}()
	var items []CheckoutItem
	total := decimal.Zero
	for rows.Next() {
		item := CheckoutItem{}
		t := &item.Ticket
		sc := &item.Schedule
		m := &item.Movie
		s := &item.Seat
		h := &item.Hall
		c := &item.Cinema
		err = rows.Scan(&t.ID, &t.CreatedAt, &t.ScheduleID, &t.SeatID, &t.Price, &t.StateID, &t.StateChangedAt, &t.Version,
			&sc.ID, &sc.CreatedAt, &sc.MovieID, &sc.HallID, &sc.Price, &sc.StartsAt, &sc.EndsAt, &sc.Version,
			&m.ID, &m.CreatedAt, &m.Title, &m.Runtime, &m.Year, pq.Array(&m.Genres), &m.Version,
			&s.ID, &s.HallID, &s.Coordinates, &s.Version,
			&h.ID, &h.Name, &h.CinemaID, &h.SeatArrangement, &h.SeatPrice, &h.Version,
			&c.ID, &c.Name, &c.Location, &c.OwnerID, &c.Version)
		if err != nil {
			return nil, decimal.Zero, err
		}
		items = append(items, item)
		total = total.Add(t.Price)
	}
	if err := rows.Err(); err != nil {
		return nil, decimal.Zero, err
	}
	return items, total, nil
}

func (s checkoutStorage) Create(userID int64, sessionID string) (*CheckoutSession, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	session := CheckoutSession{
		UserID:    userID,
		SessionID: sessionID,
	}
	query := `INSERT INTO checkout_sessions(user_id, session_id)
	          VALUES ($1, $2)
			  RETURNING expires_at`
	args := []any{userID, sessionID}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&session.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (s checkoutStorage) GetByUserID(userID int64) (*CheckoutSession, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	session := CheckoutSession{
		UserID: userID,
	}
	query := `SELECT session_id, expires_at FROM checkout_sessions
	          WHERE user_id = $1`
	args := []any{userID}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&session.SessionID, &session.ExpiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

func (s checkoutStorage) GetBySessionID(sessionID string) (*CheckoutSession, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	session := CheckoutSession{
		SessionID: sessionID,
	}
	query := `SELECT user_id, expires_at FROM checkout_sessions
	          WHERE session_id = $1`
	args := []any{sessionID}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&session.UserID, &session.ExpiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

func (s checkoutStorage) DeleteByUserID(UserID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `DELETE FROM checkout_sessions
	          WHERE user_id = $1`
	args := []any{UserID}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s checkoutStorage) DeleteBySessionID(sessionID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `DELETE FROM checkout_sessions
	          WHERE session_id = $1`
	args := []any{sessionID}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s checkoutStorage) GetAllExpired(limit int64) ([]CheckoutSession, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `SELECT user_id, session_id, expires_at FROM checkout_sessions
	          WHERE NOW() > expires_at
			  LIMIT $1`
	args := []any{limit}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var sessions []CheckoutSession
	for rows.Next() {
		var cs CheckoutSession
		err := rows.Scan(&cs.UserID, &cs.SessionID, &cs.ExpiresAt)
		if err != nil {
			return sessions, err
		}
		sessions = append(sessions, cs)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sessions, nil
}

func (s checkoutStorage) Fulfill(sessionID string, userID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	opts := &sql.TxOptions{
		Isolation: sql.LevelSerializable,
	}
	tx, err := s.db.BeginTx(ctx, opts)
	if err != nil {
		return err
	}
	query0 := `UPDATE tickets AS t
			   SET state_id = 2, state_changed_at = NOW(), version = t.version + 1
			   FROM tickets_users AS tu
			   WHERE t.id = tu.ticket_id AND tu.user_id = $1 AND t.state_id = 1`
	args0 := []any{userID}
	_, err = tx.ExecContext(ctx, query0, args0...)
	if err != nil {
		tx.Rollback()
		return err
	}
	query1 := `INSERT INTO transactions(ticket_id, user_id)
			   SELECT ticket_id, user_id FROM tickets_users
			   WHERE user_id = $1`
	args1 := []any{userID}
	_, err = tx.ExecContext(ctx, query1, args1...)
	if err != nil {
		tx.Rollback()
		return err
	}
	query2 := `DELETE FROM tickets_users
			   WHERE user_id = $1`
	args2 := []any{userID}
	_, err = tx.ExecContext(ctx, query2, args2...)
	if err != nil {
		tx.Rollback()
		return err
	}
	query3 := `DELETE FROM checkout_sessions
	           WHERE user_id = $1 AND session_id = $2`
	args3 := []any{userID, sessionID}
	_, err = tx.ExecContext(ctx, query3, args3...)
	if err != nil {
		tx.Rollback()
		return err
	}
	err = tx.Commit()
	return err
}
