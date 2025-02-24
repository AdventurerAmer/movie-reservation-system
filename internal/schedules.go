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

	"github.com/shopspring/decimal"
)

type Schedule struct {
	ID        int64           `json:"id"`
	CreatedAt time.Time       `json:"created_at"`
	MovieID   int64           `json:"movie_id"`
	HallID    int32           `json:"hall_id"`
	Price     decimal.Decimal `json:"price"`
	StartsAt  time.Time       `json:"starts_at"`
	EndsAt    time.Time       `json:"ends_at"`
	Version   int32           `json:"version"`
}

type ScheduleStorer interface {
	Create(movieID int64, hallID int32, price decimal.Decimal, startsAt time.Time, endsAt time.Time) (*Schedule, error)
	Get(movieID int64, hallID int32, starts_at time.Time, ends_at time.Time, execludingScheduleID int64) (*Schedule, error)
	GetByID(id int64) (*Schedule, error)
	GetAll(movieID int64, hallID int32, sort string, page int, pageSize int) ([]Schedule, *MetaData, error)
	Update(schedule *Schedule) error
	Delete(schedule *Schedule) error
}

type scheduleStorage struct {
	queryTimeout time.Duration
	db           *sql.DB
}

func (s scheduleStorage) Create(movieID int64, hallID int32, price decimal.Decimal, startsAt time.Time, endsAt time.Time) (*Schedule, error) {
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

func (s scheduleStorage) Get(movieID int64, hallID int32, starts_at time.Time, ends_at time.Time, execludingScheduleID int64) (*Schedule, error) {
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

func (s scheduleStorage) GetByID(id int64) (*Schedule, error) {
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

func (s scheduleStorage) GetAll(movieID int64, hallID int32, sort string, page int, pageSize int) ([]Schedule, *MetaData, error) {
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

func (s scheduleStorage) Update(schedule *Schedule) error {
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

func (s scheduleStorage) Delete(schedule *Schedule) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()
	query := `DELETE FROM schedules
	          WHERE id = $1`
	args := []any{schedule.ID, schedule.Version}
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}
