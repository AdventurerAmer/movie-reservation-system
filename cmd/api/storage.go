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
	"github.com/shopspring/decimal"
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

func (s *Storage) GetCinemaByID(id int32) (*Cinema, error) {
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

func (s *Storage) CreateHall(name string, cinemaID int32, seatArrangement string, seatPrice decimal.Decimal) (*Hall, error) {
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

func (s *Storage) GetHallByID(id int32) (*Hall, error) {
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

func (s *Storage) GetHallCinema(hallID int32) (*Hall, *Cinema, error) {
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

func (s *Storage) GetHallsForCinema(cinemaID int32) ([]Hall, error) {
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

func (s *Storage) UpdateHall(h *Hall) error {
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

func (s *Storage) DeleteHall(h *Hall) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	query := `DELETE FROM halls
			  WHERE id = $1 AND version = $2`
	args := []any{h.ID, h.Version}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *Storage) CreateSeat(hallID int32, coordinates string) (*Seat, error) {
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

func (s *Storage) GetSeatByID(id int32) (*Seat, error) {
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

func (s *Storage) GetSeatsForHall(hallID int32) ([]Seat, error) {
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

func (s *Storage) GetCinemaHallSeat(seatID int32) (*Cinema, *Hall, *Seat, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	seat := Seat{
		ID: seatID,
	}
	var h Hall
	var c Cinema
	query := `SELECT s.hall_id, s.coordinates, s.version, h.name, h.cinema_id, h.seat_arrangement, h.seat_price, h.version, c.id, c.location, c.owner_id, c.version
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

func (s *Storage) UpdateSeat(seat *Seat) error {
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

func (s *Storage) DeleteSeat(seat *Seat) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `DELETE FROM seats
			  WHERE id = $1 AND version = $2`
	args := []any{seat.ID, seat.Version}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *Storage) CreateSchedule(movieID int64, hallID int32, price decimal.Decimal, startsAt time.Time, endsAt time.Time) (*Schedule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	schedule := Schedule{
		MovieID:  movieID,
		HallID:   hallID,
		Price:    price,
		StartsAt: startsAt,
		EndsAt:   endsAt,
	}
	query := `INSERT INTO schedules(movie_id, hall_id, price, starts_at, ends_at)
	          VALUES ($1, $2, $3, $4, $5)
			  RETURNING id, version`
	args := []any{movieID, hallID, price, startsAt, endsAt}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&schedule.ID, &schedule.Version)
	if err != nil {
		return nil, err
	}
	return &schedule, nil
}

func (s *Storage) GetSchedule(movieID int64, hallID int32, starts_at time.Time, ends_at time.Time, execludingScheduleID int64) (*Schedule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	schedule := Schedule{
		MovieID: movieID,
		HallID:  hallID,
	}
	query := `SELECT id, created_at, price, starts_at, ends_at, version
	          FROM schedules
			  WHERE movie_id = $1 AND hall_id = $2 AND ((starts_at >= $3 AND starts_at <= $4) OR (ends_at >= $3 AND ends_at <= $4)) AND id != $5
			  LIMIT 1`
	args := []any{movieID, hallID, starts_at, ends_at, execludingScheduleID}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&schedule.ID, &schedule.CreatedAt, &schedule.Price, &schedule.StartsAt, &schedule.EndsAt, &schedule.Version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &schedule, nil
}

func (s *Storage) GetScheduleByID(id int64) (*Schedule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	schedule := Schedule{
		ID: id,
	}
	query := `SELECT id, created_at, movie_id, hall_id, price, starts_at, ends_at, version
	          FROM schedules
			  WHERE id = $1`
	args := []any{id}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&schedule.ID, &schedule.CreatedAt, &schedule.MovieID, &schedule.HallID, &schedule.Price, &schedule.StartsAt, &schedule.EndsAt, &schedule.Version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &schedule, nil
}

func (s *Storage) GetSchedules(movieID int64, hallID int32, sort string, page int, pageSize int) ([]Schedule, *MetaData, error) {
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

	query := fmt.Sprintf(`SELECT count(*) OVER(), id, movie_id, hall_id, created_at, price, starts_at, ends_at, version
						  FROM schedules
						  WHERE movie_id = $1 AND hall_id = $2 AND NOW() < ends_at
						  ORDER BY %s
						  LIMIT $3 OFFSET $4`, order)

	limit := pageSize
	offset := (page - 1) * pageSize
	args := []any{movieID, hallID, limit, offset}
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
	var schedules []Schedule

	for rows.Next() {
		var schedule Schedule
		err := rows.Scan(&totalRecords, &schedule.ID, &schedule.MovieID, &schedule.HallID, &schedule.CreatedAt, &schedule.Price, &schedule.StartsAt, &schedule.EndsAt, &schedule.Version)
		if err != nil {
			return nil, nil, err
		}
		schedules = append(schedules, schedule)
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
	return schedules, metaData, nil
}

func (s *Storage) UpdateSchedule(schedule *Schedule) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `UPDATE schedules
	          SET movie_id = $1, hall_id = $2, price = $3, starts_at = $4, ends_at = $5, version = version + 1 
			  WHERE id = $6 AND version = $7
			  RETURNING version`
	args := []any{schedule.MovieID, schedule.HallID, schedule.Price, schedule.StartsAt, schedule.EndsAt, schedule.ID, schedule.Version}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&schedule.Version)
	return err
}

func (s *Storage) DeleteSchedule(schedule *Schedule) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `DELETE FROM schedules
	          WHERE id = $1 AND version = $2`
	args := []any{schedule.ID, schedule.Version}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *Storage) CreateTicketsForSchedule(schedule *Schedule) (int, error) {
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

func (s *Storage) GetTicketByID(id int64) (*Ticket, error) {
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

func (s *Storage) GetTicketsForSchedule(schedule_id int64) ([]Ticket, error) {
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

func (s *Storage) GetTicketSeatsForSchedule(schedule_id int64) ([]TicketSeat, error) {
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

func (s *Storage) LockTicket(t *Ticket, u *User) error {
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

func (s *Storage) UnlockTicketByUser(t *Ticket, u *User) error {
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

func (s *Storage) UpdateTicket(t *Ticket) error {
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

func (s *Storage) DeleteTicket(t *Ticket) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `DELETE FROM tickets
			  WHERE id = $1 AND version = $2`
	args := []any{t.ID, t.Version}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *Storage) GetTicketsCheckoutForUser(u *User) ([]CheckoutTicket, decimal.Decimal, error) {
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
	args := []any{u.ID}
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
	var tickets []CheckoutTicket
	total := decimal.Zero
	for rows.Next() {
		ct := CheckoutTicket{}
		t := &ct.Ticket
		sc := &ct.Schedule
		m := &ct.Movie
		s := &ct.Seat
		h := &ct.Hall
		c := &ct.Cinema
		err = rows.Scan(&t.ID, &t.CreatedAt, &t.ScheduleID, &t.SeatID, &t.Price, &t.StateID, &t.StateChangedAt, &t.Version,
			&sc.ID, &sc.CreatedAt, &sc.MovieID, &sc.HallID, &sc.Price, &sc.StartsAt, &sc.EndsAt, &sc.Version,
			&m.ID, &m.CreatedAt, &m.Title, &m.Runtime, &m.Year, pq.Array(&m.Genres), &m.Version,
			&s.ID, &s.HallID, &s.Coordinates, &s.Version,
			&h.ID, &h.Name, &h.CinemaID, &h.SeatArrangement, &h.SeatPrice, &h.Version,
			&c.ID, &c.Name, &c.Location, &c.OwnerID, &c.Version)
		if err != nil {
			return nil, decimal.Zero, err
		}
		tickets = append(tickets, ct)
		total = total.Add(t.Price)
	}
	if err := rows.Err(); err != nil {
		return nil, decimal.Zero, err
	}
	return tickets, total, nil
}

func (s *Storage) CreateCheckoutSession(u *User, sessionID string) (*CheckoutSession, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	session := CheckoutSession{
		UserID:    u.ID,
		SessionID: sessionID,
	}
	query := `INSERT INTO checkout_sessions(user_id, session_id)
	          VALUES ($1, $2)
			  RETURNING expires_at`
	args := []any{u.ID, sessionID}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&session.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (s *Storage) GetCheckoutSessionByUserID(u *User) (*CheckoutSession, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	session := CheckoutSession{
		UserID: u.ID,
	}
	query := `SELECT id, session_id, expires_at FROM checkout_sessions
	          WHERE user_id = $1`
	args := []any{u.ID}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&session.ID, &session.SessionID, &session.ExpiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

func (s *Storage) GetCheckoutSessionBySessionID(sessionID string) (*CheckoutSession, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	session := CheckoutSession{
		SessionID: sessionID,
	}
	query := `SELECT id, user_id, expires_at FROM checkout_sessions
	          WHERE session_id = $1`
	args := []any{sessionID}
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&session.ID, &session.UserID, &session.ExpiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

func (s *Storage) DeleteUserCheckoutSession(UserID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `DELETE FROM checkout_sessions
	          WHERE user_id = $1`
	args := []any{UserID}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *Storage) DeleteCheckoutSessionBySessionID(sessionID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `DELETE FROM checkout_sessions
	          WHERE session_id = $1`
	args := []any{sessionID}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *Storage) GetExpiredCheckoutSessions(limit int64) ([]CheckoutSession, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `SELECT id, user_id, session_id, expires_at FROM checkout_sessions
	          WHERE NOW() > expires_at
			  ORDER BY id ASC
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
		err := rows.Scan(&cs.ID, &cs.UserID, &cs.SessionID, &cs.ExpiresAt)
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

func (s *Storage) UnlockExpiredTickets() (int64, error) {
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

func (s *Storage) FulfillCheckoutSessionForUser(sessionID string, userID int64) error {
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
