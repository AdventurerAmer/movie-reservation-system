package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
	"github.com/stripe/stripe-go/webhook"
)

func (app *Application) getCheckoutHandler(w http.ResponseWriter, r *http.Request) {
	u := getUserFromRequestContext(r)
	if u == nil {
		writeServerErr(errors.New("user is not authenticated"), w)
		return
	}
	items, total, err := app.storage.Checkouts.GetItems(u.ID)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"items": items,
		"total": total,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) checkoutHandler(w http.ResponseWriter, r *http.Request) {
	u := getUserFromRequestContext(r)
	if u == nil {
		writeServerErr(errors.New("user is not authenticated"), w)
		return
	}
	checkoutSession, err := app.storage.Checkouts.GetByUserID(u.ID)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if checkoutSession != nil {
		res := map[string]any{
			"message": fmt.Sprintf("you already have a session with id: %v", checkoutSession.SessionID),
		}
		writeJSON(res, http.StatusConflict, w)
		return
	}
	ticketsCheckout, _, err := app.storage.Checkouts.GetItems(u.ID)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if len(ticketsCheckout) == 0 {
		res := map[string]any{
			"message": "you didn't lock any tickets",
		}
		writeJSON(res, http.StatusUnprocessableEntity, w)
		return
	}
	lineItems := make([]*stripe.CheckoutSessionLineItemParams, len(ticketsCheckout))
	for i := 0; i < len(ticketsCheckout); i++ {
		c := ticketsCheckout[i]
		price, exact := c.Ticket.Price.Mul(decimal.NewFromInt(100)).Float64()
		if !exact {
			writeBadRequest(fmt.Errorf("price %v is not exact", price), w)
			return
		}
		ticketStr := fmt.Sprintf("Movie: %s\nCinema: %s\nHall: %s\nSeat: %s\nTicket: %d\n %v-%v", c.Movie.Title, c.Cinema.Name, c.Hall.Name, c.Seat.Coordinates, c.Ticket.ID, c.Schedule.StartsAt, c.Schedule.EndsAt)
		lineItems[i] = &stripe.CheckoutSessionLineItemParams{
			PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
				Currency: stripe.String("usd"),
				ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
					Name: stripe.String(ticketStr),
				},
				UnitAmountDecimal: stripe.Float64(price),
			},
			Quantity: stripe.Int64(1),
		}
	}

	url := "http://localhost:8080/static/"
	params := &stripe.CheckoutSessionParams{
		LineItems:  lineItems,
		Mode:       stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL: stripe.String(url + "success.html"),
		CancelURL:  stripe.String("http://localhost:8080/v1/checkout_sessions/cancel?session_id={CHECKOUT_SESSION_ID}"),
		ExpiresAt:  stripe.Int64(time.Now().Add(30 * time.Minute).Unix()),
	}
	s, err := session.New(params)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	checkoutSession, err = app.storage.Checkouts.Create(u.ID, s.ID)
	if err != nil {
		if _, err := session.Expire(s.ID, nil); err != nil {
			writeServerErr(err, w)
			return
		}
		writeServerErr(err, w)
		return
	}

	res := map[string]any{
		"url":              s.URL,
		"checkout_session": checkoutSession,
	}
	writeJSON(res, http.StatusCreated, w)
}

func (app *Application) handleWebhook(w http.ResponseWriter, r *http.Request) {
	const MaxBodyBytes = int64(65536)
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading request body: %v\n", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	event, err := webhook.ConstructEvent(body, r.Header.Get("Stripe-Signature"), app.config.stripe.webhookSecret)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error verifying webhook signature: %v\n", err)
		w.WriteHeader(http.StatusBadRequest) // Return a 400 error on a bad signature
		return
	}
	switch event.Type {
	case string(stripe.EventTypeCheckoutSessionCompleted), string(stripe.EventTypeCheckoutSessionAsyncPaymentSucceeded):
		var data stripe.CheckoutSession
		err := json.Unmarshal(event.Data.Raw, &data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing webhook JSON: %v\n", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		params := &stripe.CheckoutSessionParams{}
		params.AddExpand("line_items")
		cs, err := session.Get(data.ID, params)
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		log.Println("EventTypeCheckoutSessionCompleted|EventTypeCheckoutSessionAsyncPaymentSucceeded")

		if cs.PaymentStatus != stripe.CheckoutSessionPaymentStatusUnpaid {
			ses, err := app.storage.Checkouts.GetBySessionID(cs.ID)
			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if ses != nil {
				err = app.storage.Checkouts.Fulfill(cs.ID, ses.UserID)
				if err != nil {
					log.Println(err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
			}
		}

	case string(stripe.EventTypeCheckoutSessionExpired):
		var cs stripe.CheckoutSession
		err := json.Unmarshal(event.Data.Raw, &cs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing webhook JSON: %v\n", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ses, err := app.storage.Checkouts.GetBySessionID(cs.ID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if ses != nil {
			err = app.storage.Checkouts.DeleteBySessionID(ses.SessionID)
			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			} else {
				log.Println("Deleted Checkout Session:", ses.SessionID)
			}
		}
	}
}

func (app *Application) handleCheckoutSessionCancel(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	cs, err := app.storage.Checkouts.GetBySessionID(sessionID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if cs == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	s, err := session.Get(cs.SessionID, nil)
	if err != nil {
		log.Println(err)
	}
	if s.Status == stripe.CheckoutSessionStatusOpen {
		_, err := session.Expire(cs.SessionID, nil)
		if err != nil {
			log.Println(err)
		} else {
			log.Printf("Expired Session: %v\n", cs.SessionID)
			err = app.storage.Checkouts.DeleteBySessionID(cs.SessionID)
			if err != nil {
				log.Println(err)
			} else {
				log.Println("Deleted Session:", cs.SessionID)
			}
		}
	}
}
