package main

import (
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/shopspring/decimal"
)

func (app *Application) createCinemaHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Location string `json:"location"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}

	v := NewValidator()
	v.Check(req.Name != "", "name", "must be provided")
	v.Check(req.Location != "", "location", "must be provided")

	if v.HasErrors() {
		writeErrors(v, w)
		return
	}

	u := getUserFromRequestContext(r)
	if u == nil {
		writeServerErr(errors.New("user not authenticated"), w)
		return
	}

	c, err := app.storage.Cinemas.Create(u.ID, req.Name, req.Location)
	if err != nil {
		writeServerErr(err, w)
		return
	}

	res := map[string]any{
		"cinema": c,
	}
	writeJSON(res, http.StatusCreated, w)
}

func (app *Application) getCinemaHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	c, err := app.storage.Cinemas.GetByID(int32(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if c == nil {
		writeNotFound(w)
		return
	}
	res := map[string]any{
		"cinema": c,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) getCinemasHandler(w http.ResponseWriter, r *http.Request) {
	v := NewValidator()

	name := getQueryStringOr(r, "name", "")
	location := getQueryStringOr(r, "location", "")
	page := getQueryIntOr(r, "page", 1, v)
	pageSize := getQueryIntOr(r, "page_size", 20, v)
	sort := getQueryStringOr(r, "sort", "id")

	v.Check(page > 0 && page <= 10_000_000, "page", "must be between 1 and 10_000_000")
	v.Check(pageSize > 0 && pageSize <= 100, "page_size", "must be between 1 and 100")

	sortList := []string{"id", "-id", "name", "-name", "location", "-location"}
	v.Check(slices.Contains(sortList, sort), fmt.Sprintf("sort-%s", sort), "not supported")

	if v.HasErrors() {
		writeErrors(v, w)
		return
	}

	cinemas, meta, err := app.storage.Cinemas.GetAll(name, location, page, pageSize, sort)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"cinemas": cinemas,
		"meta":    meta,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) updateCinemaHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	var req struct {
		Name     *string `json:"name"`
		Location *string `json:"location"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}

	v := NewValidator()
	if req.Name != nil {
		v.Check(*req.Name != "", "name", "must be provided")
	}
	if req.Location != nil {
		v.Check(*req.Location != "location", "location", "must be provided")
	}
	v.Check(req.Name != nil || req.Location != nil, "name or location", "must be provided")

	if v.HasErrors() {
		writeErrors(v, w)
		return
	}

	u := getUserFromRequestContext(r)
	if u == nil {
		writeServerErr(errors.New("user is not authenticated"), w)
		return
	}

	c, err := app.storage.Cinemas.GetByID(int32(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if c == nil {
		writeNotFound(w)
		return
	}

	if c.OwnerID != u.ID {
		writeForbidden(w)
		return
	}

	if req.Name != nil {
		c.Name = *req.Name
	}

	if req.Location != nil {
		c.Location = *req.Location
	}

	err = app.storage.Cinemas.Update(c)
	if err != nil {
		writeServerErr(err, w)
		return
	}

	res := map[string]any{
		"cinema": c,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) deleteCinemaHandler(w http.ResponseWriter, r *http.Request) {
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
	c, err := app.storage.Cinemas.GetByID(int32(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if c == nil {
		writeNotFound(w)
		return
	}
	if c.OwnerID != u.ID {
		writeForbidden(w)
		return
	}
	err = app.storage.Cinemas.Delete(c)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"message": "resource deleted successfully",
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) createHallHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}

	var req struct {
		Name               string          `json:"name"`
		SeatingArrangement string          `json:"seat_arrangement"`
		SeatPrice          decimal.Decimal `json:"seat_price"`
	}

	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}

	v := NewValidator()
	v.Check(req.Name != "", "name", "must be provided")
	v.Check(req.SeatingArrangement != "", "seat_arrangement", "must be provided")
	v.Check(req.SeatPrice.GreaterThan(decimal.Zero), "seat_price", "must be greater than zero")

	if v.HasErrors() {
		writeErrors(v, w)
		return
	}

	u := getUserFromRequestContext(r)
	if u == nil {
		writeServerErr(errors.New("user is not authenticated"), w)
		return
	}
	c, err := app.storage.Cinemas.GetByID(int32(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if c == nil {
		writeNotFound(w)
		return
	}
	if c.OwnerID != u.ID {
		writeForbidden(w)
		return
	}
	h, err := app.storage.Halls.Create(req.Name, c.ID, req.SeatingArrangement, req.SeatPrice)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"hall": h,
	}
	writeJSON(res, http.StatusCreated, w)
}

func (app *Application) getHallsHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	halls, err := app.storage.Halls.GetAllForCinema(int32(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"halls": halls,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) updateHallHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	var req struct {
		Name            *string          `json:"name"`
		SeatArrangement *string          `json:"seat_arrangement"`
		SeatPrice       *decimal.Decimal `json:"seat_price"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}
	v := NewValidator()
	if req.Name != nil {
		v.Check(*req.Name != "", "name", "must be provided")
	}
	if req.SeatArrangement != nil {
		v.Check(*req.SeatArrangement != "", "seat_arrangement", "must be provided")
	}
	if req.SeatPrice != nil {
		v.Check(req.SeatPrice.GreaterThan(decimal.Zero), "seat_price", "must be provided")
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
	h, c, err := app.storage.Halls.GetCinema(int32(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if h == nil {
		writeNotFound(w)
		return
	}
	if c.OwnerID != u.ID {
		writeForbidden(w)
		return
	}
	if req.Name != nil {
		h.Name = *req.Name
	}
	if req.SeatArrangement != nil {
		h.SeatArrangement = *req.SeatArrangement
	}
	if req.SeatPrice != nil {
		h.SeatPrice = *req.SeatPrice
	}
	err = app.storage.Halls.Update(h)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"hall": h,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) deleteHallHandler(w http.ResponseWriter, r *http.Request) {
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
	h, c, err := app.storage.Halls.GetCinema(int32(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if h == nil {
		writeNotFound(w)
		return
	}
	if c.OwnerID != u.ID {
		writeForbidden(w)
		return
	}
	err = app.storage.Halls.Delete(h)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"message": "resource deleted successfully",
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) createSeatHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	var req struct {
		Coordinates string `json:"coordinates"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}
	v := NewValidator()
	v.Check(req.Coordinates != "", "coordinates", "must be provided")
	if v.HasErrors() {
		writeErrors(v, w)
		return
	}
	u := getUserFromRequestContext(r)
	if u == nil {
		writeServerErr(errors.New("user is not authenticated"), w)
		return
	}
	h, c, err := app.storage.Halls.GetCinema(int32(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if h == nil {
		writeNotFound(w)
		return
	}
	if c.OwnerID != u.ID {
		writeForbidden(w)
		return
	}
	seat, err := app.storage.Seats.Create(int32(id), req.Coordinates)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"seat": seat,
	}
	writeJSON(res, http.StatusCreated, w)
}

func (app *Application) getSeatsHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	seats, err := app.storage.Seats.GetAll(int32(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"seats": seats,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) updateSeatHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	var req struct {
		Coordinates string `json:"coordinates"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}
	v := NewValidator()
	v.Check(req.Coordinates != "", "coordinates", "must be provided")
	if v.HasErrors() {
		writeErrors(v, w)
		return
	}
	u := getUserFromRequestContext(r)
	if u == nil {
		writeServerErr(errors.New("user is not authenticated"), w)
		return
	}

	c, _, s, err := app.storage.Seats.GetCinemaHall(int32(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if c == nil {
		writeNotFound(w)
		return
	}
	if c.OwnerID != u.ID {
		writeForbidden(w)
		return
	}

	s.Coordinates = req.Coordinates
	err = app.storage.Seats.Update(s)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"seat": s,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) deleteSeatHandler(w http.ResponseWriter, r *http.Request) {
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

	c, _, s, err := app.storage.Seats.GetCinemaHall(int32(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if c == nil {
		writeNotFound(w)
		return
	}
	if c.OwnerID != u.ID {
		writeForbidden(w)
		return
	}

	err = app.storage.Seats.Delete(s)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"message": "resouce delete successfully",
	}
	writeJSON(res, http.StatusOK, w)
}
