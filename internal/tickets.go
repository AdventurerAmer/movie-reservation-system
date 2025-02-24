package internal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/shopspring/decimal"
)

type TicketState int16

const (
	TicketStateUnsold TicketState = iota
	TicketStateLocked
	TicketStateSold
)

func (s TicketState) String() string {
	switch s {
	case TicketStateUnsold:
		return "Unsold"
	case TicketStateLocked:
		return "Locked"
	case TicketStateSold:
		return "Sold"
	}
	return fmt.Sprintf("TicketState %d", s)
}

type Ticket struct {
	ID             int64           `json:"id"`
	CreatedAt      time.Time       `json:"created_at"`
	ScheduleID     int64           `json:"schedule_id"`
	SeatID         int32           `json:"seat_id"`
	Price          decimal.Decimal `json:"price"`
	StateID        TicketState     `json:"state_id"`
	StateChangedAt time.Time       `json:"state_changed_at"`
	Version        int32           `json:"version"`
}

type TicketSeat struct {
	Ticket Ticket `json:"ticket"`
	Seat   Seat   `json:"seat"`
}

type TicketStorer interface {
	CreateAll(schedule *Schedule) (int, error)
	GetByID(id int64) (*Ticket, error)
	GetAllForSchedule(schedule_id int64) ([]Ticket, error)
	GetSeatsForSchedule(schedule_id int64) ([]TicketSeat, error)
	Lock(t *Ticket, u *User) error
	Unlock(t *Ticket, u *User) error
	Update(t *Ticket) error
	Delete(t *Ticket) error
	UnlockAllExpired() (int64, error)
}

type ticketStorage struct {
	queryTimeout time.Duration
	db           *sql.DB
}

func (s ticketStorage) CreateAll(schedule *Schedule) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `INSERT INTO tickets (schedule_id, seat_id, price) 
	          SELECT $1, s.id, $2 + h.seat_price FROM seats as s
	          INNER JOIN halls as h
			  ON s.hall_id = h.id
			  WHERE h.id = $3
			  ON CONFLICT DO NOTHING`
	args := []any{schedule.ID, schedule.Price, schedule.HallID}
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func (s ticketStorage) GetByID(id int64) (*Ticket, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	t := Ticket{
		ID: id,
	}
	query := `SELECT created_at, schedule_id, seat_id, price, state_id, state_changed_at, version
	          FROM tickets
			  WHERE id = $1`
	args := []any{id}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&t.CreatedAt, &t.ScheduleID, &t.SeatID, &t.Price, &t.StateID, &t.StateChangedAt, &t.Version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

func (s ticketStorage) GetAllForSchedule(schedule_id int64) ([]Ticket, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `SELECT id, created_at, schedule_id, seat_id, price, state_id, state_changed_at
	          FROM tickets
			  WHERE schedule_id = $1`
	args := []any{schedule_id}
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
	var tickets []Ticket
	for rows.Next() {
		var t Ticket
		err := rows.Scan(&t.ID, &t.CreatedAt, &t.ScheduleID, &t.SeatID, &t.Price, &t.StateID, &t.StateChangedAt)
		if err != nil {
			return nil, err
		}
		tickets = append(tickets, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tickets, nil
}

func (s ticketStorage) GetSeatsForSchedule(schedule_id int64) ([]TicketSeat, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `SELECT t.id, t.created_at, t.schedule_id, t.seat_id, t.price, t.state_id, t.state_changed_at, t.version,
	          s.id, s.coordinates, s.hall_id, s.version
	          FROM tickets as t
			  INNER JOIN seats as s
			  ON t.seat_id = s.id
			  WHERE schedule_id = $1`
	args := []any{schedule_id}
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
	var ticketSeats []TicketSeat
	for rows.Next() {
		var ticket Ticket
		var seat Seat
		err := rows.Scan(&ticket.ID, &ticket.CreatedAt, &ticket.ScheduleID, &ticket.SeatID, &ticket.Price, &ticket.StateID, &ticket.StateChangedAt, &ticket.Version, &seat.ID, &seat.Coordinates, &seat.HallID, &seat.Version)
		if err != nil {
			return nil, err
		}
		ticketSeats = append(ticketSeats, TicketSeat{Ticket: ticket, Seat: seat})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ticketSeats, nil
}

func (s ticketStorage) Lock(t *Ticket, u *User) error {
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
			   SET state_id = 1, state_changed_at = NOW(), version = t.version + 1
			   FROM schedules AS sc  
			   WHERE t.schedule_id = sc.id 
			   AND NOW() < sc.starts_at 
			   AND t.id = $1 
			   AND t.version = $2 
			   AND state_id = 0
			   RETURNING state_id, state_changed_at, t.version`
	args0 := []any{t.ID, t.Version}
	err = tx.QueryRowContext(ctx, query0, args0...).Scan(&t.StateID, &t.StateChangedAt, &t.Version)
	if err != nil {
		tx.Rollback()
		return err
	}
	query1 := `INSERT INTO tickets_users(ticket_id, user_id)
	           VALUES ($1, $2)`
	args1 := []any{t.ID, u.ID}
	_, err = tx.ExecContext(ctx, query1, args1...)
	if err != nil {
		tx.Rollback()
		return err
	}
	err = tx.Commit()
	return err
}

func (s ticketStorage) Unlock(t *Ticket, u *User) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	opts := &sql.TxOptions{
		Isolation: sql.LevelSerializable,
	}
	tx, err := s.db.BeginTx(ctx, opts)
	if err != nil {
		return err
	}
	query0 := `DELETE FROM tickets_users
	           WHERE ticket_id = $1 AND user_id = $2`
	args0 := []any{t.ID, u.ID}
	result, err := tx.ExecContext(ctx, query0, args0...)
	if err != nil {
		tx.Rollback()
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		tx.Rollback()
		return err
	}
	if n != 1 {
		tx.Rollback()
		return err
	}
	query1 := `UPDATE tickets
	           SET state_id = 0, state_changed_at = NOW(), version = version + 1
			   WHERE id = $1 AND version = $2 AND state_id = 1
			   RETURNING state_id, state_changed_at, version`
	args1 := []any{t.ID, t.Version}
	err = tx.QueryRowContext(ctx, query1, args1...).Scan(&t.StateID, &t.StateChangedAt, &t.Version)
	if err != nil {
		tx.Rollback()
		return err
	}

	err = tx.Commit()
	return err
}

func (s ticketStorage) Update(t *Ticket) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `UPDATE tickets
	          SET state_id = $1, state_changed_at = NOW(), version = version + 1
			  WHERE id = $2 AND version = $3
			  RETURNING version`
	args := []any{t.StateID, t.ID, t.Version}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&t.Version)
	return err
}

func (s ticketStorage) Delete(t *Ticket) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `DELETE FROM tickets
			  WHERE id = $1 AND version = $2`
	args := []any{t.ID, t.Version}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s ticketStorage) UnlockAllExpired() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	opts := &sql.TxOptions{
		Isolation: sql.LevelSerializable,
	}
	tx, err := s.db.BeginTx(ctx, opts)
	if err != nil {
		return 0, err
	}
	query0 := `DELETE FROM tickets_users as tu
			   WHERE NOW() > tu.expires_at AND NOT EXISTS(SELECT 1 FROM checkout_sessions as cs WHERE cs.user_id = tu.user_id)`

	result, err := tx.ExecContext(ctx, query0)
	if err != nil {
		tx.Rollback()
		return 0, err
	}

	n, err := result.RowsAffected()
	if err != nil {
		tx.Rollback()
		return 0, err
	}

	query1 := `UPDATE tickets as t
	           SET state_id = 0, version = version + 1 
			   WHERE t.state_id = 1 AND NOW() > state_changed_at AND NOT EXISTS(SELECT 1 FROM tickets_users as tu WHERE tu.ticket_id = t.id)`
	_, err = tx.ExecContext(ctx, query1)
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	err = tx.Commit()
	return n, err
}
