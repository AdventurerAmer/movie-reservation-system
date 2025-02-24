package internal

import (
	"context"
	"database/sql"
	"time"
)

type Storage struct {
	Users       UserStorer
	Tokens      TokenStorer
	Permissions PermissionStorer
	Movies      MovieStorer
	Cinemas     CinemaStorer
	Halls       HallStorer
	Seats       SeatStorer
	Schedules   ScheduleStorer
	Tickets     TicketStorer
	Checkouts   CheckoutStorer
}

func NewStorage(dsn string, queryTimeout time.Duration) (*Storage, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxIdleConns(25)
	db.SetMaxOpenConns(25)
	db.SetConnMaxIdleTime(15 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	err = db.PingContext(ctx)
	if err != nil {
		return nil, err
	}

	s := &Storage{
		Users:       userStorage{db: db, queryTimeout: queryTimeout},
		Tokens:      tokenStorage{db: db, queryTimeout: queryTimeout},
		Permissions: permissionStorage{db: db, queryTimeout: queryTimeout},
		Movies:      movieStorage{db: db, queryTimeout: queryTimeout},
		Cinemas:     cinemaStorage{db: db, queryTimeout: queryTimeout},
		Halls:       hallStorage{db: db, queryTimeout: queryTimeout},
		Seats:       seatStorage{db: db, queryTimeout: queryTimeout},
		Schedules:   scheduleStorage{db: db, queryTimeout: queryTimeout},
		Tickets:     ticketStorage{db: db, queryTimeout: queryTimeout},
		Checkouts:   checkoutStorage{db: db, queryTimeout: queryTimeout},
	}
	return s, nil
}
