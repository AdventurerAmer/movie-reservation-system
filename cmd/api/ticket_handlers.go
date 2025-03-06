package main

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/AdventurerAmer/movie-reservation-system/internal"
)

// createTicketsForScheduleHandler godoc
//
//	@Summary		Creates the tickets
//	@Description	creates the tickets for a given schedule
//	@Tags			tickets
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"schedule id"
//	@Success		201	{object}	ResponseMessage
//	@Failure		400	{object}	ResponseError
//	@Failure		400	{object}	ViolationsMessage
//	@Failure		404	{object}	ResponseMessage
//	@Failure		500	{object}	ResponseError
//	@Router			/schedules/{id}/tickets [post]
func (app *Application) createTicketsForScheduleHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	v := NewValidator()
	v.Check(id > 0, "id", "must be provided")
	if v.HasErrors() {
		writeErrors(v, w)
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
	n, err := app.storage.Tickets.CreateAll(s)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	writeJSON(ResponseMessage{Message: fmt.Sprintf("created %d tickets successfully", n)}, http.StatusOK, w)
}

type GetTicketsForSchedule struct {
	Tickets []internal.Ticket `json:"tickets"`
}

// getTicketsForScheduleHandler godoc
//
//	@Summary		Gets a list of tickets
//	@Description	gets a list of tickets for a given schedule
//	@Tags			tickets
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"schedule id"
//	@Success		200	{object}	ResponseMessage
//	@Failure		400	{object}	ResponseError
//	@Failure		400	{object}	ViolationsMessage
//	@Failure		500	{object}	ResponseError
//	@Router			/schedules/{id}/tickets [get]
func (app *Application) getTicketsForScheduleHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	v := NewValidator()
	v.Check(id > 0, "id", "must be provided")
	if v.HasErrors() {
		writeErrors(v, w)
		return
	}
	tickets, err := app.storage.Tickets.GetAllForSchedule(int64(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	writeJSON(GetTicketsForSchedule{Tickets: tickets}, http.StatusOK, w)
}

type LockTicketResponse struct {
	Ticket *internal.Ticket `json:"ticket"`
}

// lockTicketHandler godoc
//
//	@Summary		Locks a ticket
//	@Description	locks a ticket to a given user for some time
//	@Tags			tickets
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"ticket id"
//	@Success		200	{object}	LockTicketResponse
//	@Failure		400	{object}	ResponseError
//	@Failure		404	{object}	ResponseMessage
//	@Failure		409	{object}	ResponseMessage
//	@Failure		500	{object}	ResponseError
//	@Router			/tickets/{id}/lock [post]
func (app *Application) lockTicketHandler(w http.ResponseWriter, r *http.Request) {
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
	t, err := app.storage.Tickets.GetByID(int64(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if t == nil {
		writeNotFound(w)
		return
	}
	if t.StateID == internal.TicketStateLocked {
		writeJSON(ResponseMessage{Message: "ticket is already locked"}, http.StatusConflict, w)
		return
	}
	if t.ScheduleID == int64(internal.TicketStateSold) {
		writeJSON(ResponseMessage{Message: "ticket is already sold"}, http.StatusConflict, w)
		return
	}
	s, err := app.storage.Schedules.GetByID(t.ScheduleID)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if time.Now().After(s.StartsAt) {
		writeJSON(ResponseMessage{Message: "can't lock ticket because movie already started"}, http.StatusConflict, w)
		return
	}
	checkoutSession, err := app.storage.Checkouts.GetByUserID(u.ID)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if checkoutSession != nil {
		writeJSON(ResponseMessage{Message: fmt.Sprintf("you can't lock a ticket during checkout: %v", checkoutSession.SessionID)}, http.StatusConflict, w)
		return
	}

	err = app.storage.Tickets.Lock(t, u)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	writeJSON(LockTicketResponse{Ticket: t}, http.StatusOK, w)
}

// unlockTicketHandler godoc
//
//	@Summary		Unlocks a ticket
//	@Description	unlocks a ticket
//	@Tags			tickets
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"ticket id"
//	@Success		200	{object}	LockTicketResponse
//	@Failure		400	{object}	ResponseError
//	@Failure		404	{object}	ResponseMessage
//	@Failure		409	{object}	ResponseMessage
//	@Failure		500	{object}	ResponseError
//	@Router			/tickets/{id}/unlock [post]
func (app *Application) unlockTicketHandler(w http.ResponseWriter, r *http.Request) {
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
	t, err := app.storage.Tickets.GetByID(int64(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if t == nil {
		writeNotFound(w)
		return
	}
	if t.StateID != internal.TicketStateLocked {
		res := map[string]any{
			"message": "ticket must be locked to unlock it",
		}
		writeJSON(res, http.StatusConflict, w)
		return
	}

	checkoutSession, err := app.storage.Checkouts.GetByUserID(u.ID)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if checkoutSession != nil {
		res := map[string]any{
			"message": fmt.Sprintf("you can't unlock a ticket during checkout: %v", checkoutSession.SessionID),
		}
		writeJSON(res, http.StatusConflict, w)
		return
	}

	err = app.storage.Tickets.Unlock(t, u)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"ticket": t,
	}
	writeJSON(res, http.StatusOK, w)
}
