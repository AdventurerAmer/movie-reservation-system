package internal

import (
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

func NewStorage(db *sql.DB, queryTimeout time.Duration) *Storage {
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
	return s
}
