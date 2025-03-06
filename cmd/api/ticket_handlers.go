package main

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/AdventurerAmer/movie-reservation-system/internal"
)

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
	res := map[string]any{
		"message":      "created tickets successfully",
		"ticket_count": n,
	}
	writeJSON(res, http.StatusOK, w)
}

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
	ticketSeats, err := app.storage.Tickets.GetAllForSchedule(int64(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"tickets": ticketSeats,
	}
	writeJSON(res, http.StatusOK, w)
}

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
		res := map[string]any{
			"message": "ticket is already locked",
		}
		writeJSON(res, http.StatusConflict, w)
		return
	}
	if t.ScheduleID == int64(internal.TicketStateSold) {
		res := map[string]any{
			"message": "ticket is already sold",
		}
		writeJSON(res, http.StatusConflict, w)
		return
	}
	s, err := app.storage.Schedules.GetByID(t.ScheduleID)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if time.Now().After(s.StartsAt) {
		res := map[string]any{
			"message": "can't lock ticket because movie already started",
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
			"message": fmt.Sprintf("you can't lock a ticket during checkout: %v", checkoutSession.SessionID),
		}
		writeJSON(res, http.StatusConflict, w)
		return
	}

	err = app.storage.Tickets.Lock(t, u)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"ticket": t,
	}
	writeJSON(res, http.StatusOK, w)
}

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
