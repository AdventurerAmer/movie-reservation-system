package main

import (
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/AdventurerAmer/movie-reservation-system/internal"
	"github.com/shopspring/decimal"
)

type CreateCinemaResponse struct {
	Cinema *internal.Cinema `json:"cinema"`
}

// createCinemaHandler godoc
//
//	@Summary		Creates a cinema
//	@Description	creates a cinema
//	@Tags			cinemas
//	@Accept			json
//	@Produce		json
//	@Param			name		body		string	true	"name"
//	@Param			location	body		string	true	"location"
//	@Success		201			{object}	CreateCinemaResponse
//	@Failure		400			{object}	ViolationsMessage
//	@Failure		500			{object}	ResponseError
//	@Router			/cinemas [post]
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

	writeJSON(CreateCinemaResponse{Cinema: c}, http.StatusCreated, w)
}

type GetCinemaResponse struct {
	Cinema *internal.Cinema `json:"cinema"`
}

// getCinemaHandler godoc
//
//	@Summary		Gets a cinema
//	@Description	gets a cinema by id
//	@Tags			cinemas
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"id"
//	@Success		200	{object}	GetCinemaResponse
//	@Failure		404	{object}	ResponseMessage
//	@Failure		500	{object}	ResponseError
//	@Router			/cinemas/{id} [get]
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
	writeJSON(GetCinemaResponse{Cinema: c}, http.StatusOK, w)
}

type GetCinemasResponse struct {
	Cinemas  []internal.Cinema  `json:"cinemas"`
	MetaData *internal.MetaData `json:"meta_data"`
}

// getCinemasHandler godoc
//
//	@Summary		Get a list of cinemas
//	@Description	gets a list of cinemas by search parameters
//	@Tags			cinemas
//	@Accept			json
//	@Produce		json
//	@Param			name		query		string	false	"name"
//	@Param			location	query		string	false	"location"
//	@Param			page		query		int		false	"page number"
//	@Param			page_size	query		int		false	"page size"
//	@Param			sort		query		string	false	"sort params are (name, location) prefix with - to sort descending"
//
//	@Success		200			{object}	CreateCinemaResponse
//	@Failure		404			{object}	ResponseMessage
//	@Failure		500			{object}	ResponseError
//	@Router			/cinemas [get]
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

	cinemas, metaData, err := app.storage.Cinemas.GetAll(name, location, page, pageSize, sort)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	writeJSON(GetCinemasResponse{Cinemas: cinemas, MetaData: metaData}, http.StatusOK, w)
}

type UpdateCinemaResponse struct {
	Cinema *internal.Cinema `json:"cinema"`
}

// updateCinemaHandler godoc
//
//	@Summary		Updates a cinema
//	@Description	updates a cinema
//	@Tags			cinemas
//	@Accept			json
//	@Produce		json
//	@Param			name		body		string	false	"name"
//	@Param			location	body		string	false	"location"
//	@Success		200			{object}	UpdateCinemaResponse
//	@Failure		404			{object}	ResponseMessage
//	@Failure		409			{object}	ResponseMessage
//	@Failure		500			{object}	ResponseError
//	@Router			/cinemas/{id} [put]
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

	writeJSON(UpdateCinemaResponse{Cinema: c}, http.StatusOK, w)
}

// deleteCinemaHandler godoc
//
//	@Summary		Deletes a cinema
//	@Description	deletes a cinema
//	@Tags			cinemas
//	@Accept			json
//	@Produce		json
//	@Param			name		body		string	false	"name"
//	@Param			location	body		string	false	"location"
//	@Success		200			{object}	ResponseMessage
//	@Failure		404			{object}	ResponseMessage
//	@Failure		409			{object}	ResponseMessage
//	@Failure		500			{object}	ResponseError
//	@Router			/cinemas/{id} [put]
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
	writeJSON(ResponseMessage{Message: "resource deleted successfully"}, http.StatusOK, w)
}

type CreateHallResponse struct {
	Hall *internal.Hall `json:"hall"`
}

// createHallHandler godoc
//
//	@Summary		Creates a hall
//	@Description	creates a hall for a given cinema
//	@Tags			halls
//	@Accept			json
//	@Produce		json
//	@Param			id					path		int		true	"cinema id"
//	@Param			name				body		string	false	"name"
//	@Param			seat_arrangement	body		string	false	"seat arrangement"
//	@Param			seat_price			body		string	false	"seat price"
//	@Success		201					{object}	CreateHallResponse
//	@Failure		400					{object}	ViolationsMessage
//
//	@Failure		409					{object}	ResponseMessage
//	@Failure		500					{object}	ResponseError
//	@Router			/cinemas/{id}/halls [post]
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
	writeJSON(CreateHallResponse{Hall: h}, http.StatusCreated, w)
}

type GetHallsResponse struct {
	Halls []internal.Hall `json:"halls"`
}

// getHallsHandler godoc
//
//	@Summary		Gets a list of halls
//	@Description	gets a list of halls for a given cinema
//	@Tags			halls
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"cinema id"
//	@Success		200	{object}	GetHallsResponse
//	@Failure		400	{object}	ResponseMessage
//	@Failure		500	{object}	ResponseError
//	@Router			/cinemas/{id}/halls [get]
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
	writeJSON(GetHallsResponse{Halls: halls}, http.StatusOK, w)
}

type UpdateHallResponse struct {
	Hall *internal.Hall `json:"hall"`
}

// updateHallHandler godoc
//
//	@Summary		Updates a hall
//	@Description	Updates a hall by id
//	@Tags			halls
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"hall id"
//	@Success		200	{object}	UpdateHallResponse
//	@Failure		400	{object}	ResponseMessage
//	@Failure		400	{object}	ViolationsMessage
//	@Failure		409	{object}	ResponseMessage
//	@Failure		500	{object}	ResponseError
//	@Router			/halls/{id} [put]
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
	h, c, err := app.storage.Halls.GetAndCinema(int32(id))
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
	writeJSON(UpdateHallResponse{Hall: h}, http.StatusOK, w)
}

// deleteHallHandler godoc
//
//	@Summary		Deletes a hall
//	@Description	Deletes a hall by id
//	@Tags			halls
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"hall id"
//	@Success		200	{object}	ResponseMessage
//	@Failure		400	{object}	ResponseMessage
//	@Failure		404	{object}	ViolationsMessage
//	@Failure		409	{object}	ResponseMessage
//	@Failure		500	{object}	ResponseError
//	@Router			/halls/{id} [delete]
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
	h, c, err := app.storage.Halls.GetAndCinema(int32(id))
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
	writeJSON(ResponseMessage{Message: "resource deleted successfully"}, http.StatusOK, w)
}

type CreateSeatReponse struct {
	Seat *internal.Seat `json:"seat"`
}

// createSeatHandler godoc
//
//	@Summary		Creates a seat
//	@Description	Creates a seat for a given hall
//	@Tags			seats
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"hall id"
//	@Success		201	{object}	CreateSeatReponse
//	@Failure		400	{object}	ViolationsMessage
//	@Failure		404	{object}	ResponseMessage
//	@Failure		409	{object}	ResponseMessage
//	@Failure		500	{object}	ResponseError
//	@Router			/halls/{id}/seats [post]
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
	h, c, err := app.storage.Halls.GetAndCinema(int32(id))
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
	writeJSON(CreateSeatReponse{Seat: seat}, http.StatusCreated, w)
}

type GetSeatsResponse struct {
	Seats []internal.Seat `json:"seats"`
}

// getSeatsHandler godoc
//
//	@Summary		Gets a list of seats
//	@Description	gets a list of seats for a given hall
//	@Tags			seats
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"hall id"
//	@Success		201	{object}	CreateSeatReponse
//	@Failure		400	{object}	ResponseMessage
//	@Failure		500	{object}	ResponseError

//	@Router	/halls/{id}/seats [get]
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
	writeJSON(GetSeatsResponse{Seats: seats}, http.StatusOK, w)
}

type UpdateSeatReponse struct {
	Seat *internal.Seat `json:"seat"`
}

// updateSeatHandler godoc
//
//	@Summary		Updates a seat
//	@Description	updates a seat by id
//	@Tags			seats
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"seat id"
//	@Success		201	{object}	CreateSeatReponse
//	@Failure		400	{object}	ResponseMessage
//	@Failure		400	{object}	ViolationsMessage
//	@Failure		404	{object}	ResponseMessage
//	@Failure		409	{object}	ResponseMessage
//	@Failure		500	{object}	ResponseError
//
//	@Router			/seats/{id} [put]
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

	c, _, s, err := app.storage.Seats.GetWithCinemaAndHall(int32(id))
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
	writeJSON(UpdateSeatReponse{Seat: s}, http.StatusOK, w)
}

// deleteSeatHandler godoc
//
//	@Summary		Deletes a seat
//	@Description	deletes a seat by id
//	@Tags			seats
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"seat id"
//	@Success		200	{object}	ResponseMessage
//	@Failure		400	{object}	ResponseMessage
//	@Failure		400	{object}	ViolationsMessage
//	@Failure		404	{object}	ResponseMessage
//	@Failure		409	{object}	ResponseMessage
//	@Failure		500	{object}	ResponseError
//
//	@Router			/seats/{id} [delete]
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

	c, _, s, err := app.storage.Seats.GetWithCinemaAndHall(int32(id))
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
	writeJSON(ResponseMessage{Message: "resouce delete successfully"}, http.StatusOK, w)
}
