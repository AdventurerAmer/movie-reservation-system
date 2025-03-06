package main

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/shopspring/decimal"
)

func (app *Application) createScheduleHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MovieID  *int64           `json:"movie_id"`
		HallID   *int32           `json:"hall_id"`
		Price    *decimal.Decimal `json:"price"`
		StartsAt *time.Time       `json:"starts_at"`
		EndsAt   *time.Time       `json:"ends_at"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}
	v := NewValidator()
	v.Check(req.MovieID != nil, "movie_id", "must be provided")
	v.Check(req.HallID != nil, "hall_id", "must be provided")
	v.Check(req.Price != nil, "price", "must be provided")
	v.Check(req.StartsAt != nil, "starts_at", "must be provided")
	v.Check(req.EndsAt != nil, "ends_at", "must be provided")

	if req.MovieID != nil {
		v.Check(*req.MovieID > 0, "movie_id", "must be greater then zero")
	}
	if req.HallID != nil {
		v.Check(*req.HallID > 0, "hall_id", "must be greater then zero")
	}
	if req.Price != nil {
		v.Check(req.Price.GreaterThanOrEqual(decimal.Zero), "price", "must be greater than or equal to zero")
	}
	if req.StartsAt != nil {
		v.Check(req.StartsAt.After(time.Now()), "starts_at", "invalid time")
	}
	if req.EndsAt != nil {
		v.Check(req.EndsAt.After(time.Now()), "ends_at", "invalid time")
	}
	if req.StartsAt != nil && req.EndsAt != nil {
		v.Check(req.EndsAt.After(*req.StartsAt), "ends_at", "must come after starts_at")
	}

	if v.HasErrors() {
		writeErrors(v, w)
		return
	}

	u := getUserFromRequestContext(r)
	if u == nil {
		writeServerErr(errors.New("user is not authenticated"), w)
		return
	}

	m, err := app.storage.Movies.GetByID(*req.MovieID)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if m == nil {
		writeError(fmt.Errorf("couldn't find movie with id %d", *req.MovieID), http.StatusNotFound, w)
		return
	}

	_, c, err := app.storage.Halls.GetAndCinema(*req.HallID)
	if err != nil {
		writeServerErr(err, w)
		return
	}

	if c == nil {
		writeError(fmt.Errorf("couldn't find hall with id %d", *req.HallID), http.StatusNotFound, w)
		return
	}

	if c.OwnerID != u.ID {
		writeForbidden(w)
		return
	}

	s, err := app.storage.Schedules.Get(*req.MovieID, *req.HallID, *req.StartsAt, *req.EndsAt, 0)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if s != nil {
		res := map[string]any{
			"message":  "there is already a schedule that intersets with this schedule",
			"schedule": s,
		}
		writeJSON(res, http.StatusConflict, w)
		return
	}
	s, err = app.storage.Schedules.Create(*req.MovieID, *req.HallID, *req.Price, *req.StartsAt, *req.EndsAt)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"schedule": s,
	}
	writeJSON(res, http.StatusCreated, w)
}

func (app *Application) getSchedulesHandler(w http.ResponseWriter, r *http.Request) {
	v := NewValidator()
	movie_id := getQueryIntOr(r, "movie_id", 0, v)
	hall_id := getQueryIntOr(r, "hall_id", 0, v)
	page := getQueryIntOr(r, "page", 1, v)
	pageSize := getQueryIntOr(r, "page_size", 20, v)
	sort := getQueryStringOr(r, "sort", "starts_at")

	v.Check(movie_id > 0, "movie_id", "must be greater than zero")
	v.Check(hall_id > 0, "hall_id", "must be greater than zero")
	v.Check(page >= 1 && page <= 10_000_000, "page", "must be between 1 and 10_000_000")
	v.Check(pageSize >= 1 && pageSize <= 100, "page", "must be between 1 and 100")
	sortList := []string{"id", "-id", "price", "-price", "starts_at", "-starts_at", "ends_at", "-ends_at"}
	v.Check(slices.Contains(sortList, sort), "sort", "not supported")

	if v.HasErrors() {
		writeErrors(v, w)
		return
	}

	s, m, err := app.storage.Schedules.GetAll(int64(movie_id), int32(hall_id), sort, page, pageSize)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"schedules": s,
		"meta":      m,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) updateScheduleHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	var req struct {
		Price    *decimal.Decimal `json:"price"`
		StartsAt *time.Time       `json:"starts_at"`
		EndsAt   *time.Time       `json:"ends_at"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}
	v := NewValidator()
	if req.Price != nil {
		v.Check(req.Price.GreaterThanOrEqual(decimal.Zero), "price", "must be greater than or equal to zero")
	}
	if req.StartsAt != nil {
		v.Check(req.StartsAt.After(time.Now()), "starts_at", "invalid time")
	}
	if req.EndsAt != nil {
		v.Check(req.EndsAt.After(time.Now()), "ends_at", "invalid time")
	}
	if req.StartsAt != nil && req.EndsAt != nil {
		v.Check(req.EndsAt.After(*req.EndsAt), "ends_at", "must come after starts_at")
	}
	if v.HasErrors() {
		writeErrors(v, w)
		return
	}

	s, err := app.storage.Schedules.GetByID(int64(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}

	if s == nil {
		writeNotFound(w)
		return
	}

	if req.Price != nil {
		s.Price = *req.Price
	}

	if req.StartsAt != nil {
		s.StartsAt = *req.StartsAt
	}

	if req.EndsAt != nil {
		s.EndsAt = *req.EndsAt
	}

	if req.StartsAt != nil || req.EndsAt != nil {
		conflictingSchedule, err := app.storage.Schedules.Get(s.MovieID, s.HallID, s.StartsAt, s.EndsAt, s.ID)
		if err != nil {
			writeServerErr(err, w)
			return
		}
		if conflictingSchedule != nil {
			res := map[string]any{
				"message":  "there is already a schedule that intersets with this schedule",
				"schedule": conflictingSchedule,
			}
			writeJSON(res, http.StatusConflict, w)
			return
		}
	}

	err = app.storage.Schedules.Update(s)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"schedule": s,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) deleteScheduleHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	u := getUserFromRequestContext(r)
	if u == nil {
		writeServerErr(errors.New("user is not authenticated"), w)
		return
	}
	s, err := app.storage.Schedules.GetByID(int64(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if s == nil {
		writeNotFound(w)
		return
	}
	err = app.storage.Schedules.Delete(s)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"message": "resource deleted successfully",
	}
	writeJSON(res, http.StatusOK, w)
}
