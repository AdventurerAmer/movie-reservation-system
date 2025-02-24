package main

import (
	"net/http"

	"github.com/harlequingg/movie-reservation-system/internal"
)

func composeRoutes(app *Application) http.Handler {
	mux := &http.ServeMux{}

	fs := http.FileServer(http.Dir("./public"))
	mux.Handle("GET /static/", http.StripPrefix("/static/", fs))

	mux.HandleFunc("GET /v1/healthcheck", app.healthCheckHandler)

	mux.HandleFunc("POST /v1/users", app.createUserHandler)
	mux.HandleFunc("GET /v1/users/{id}", app.authenticate(app.getUserHandler))
	mux.HandleFunc("PUT /v1/users/{id}", app.authenticate(app.updateUserHandler))
	mux.HandleFunc("DELETE /v1/users/{id}", app.authenticate(app.deleteUserHandler))

	mux.HandleFunc("POST /v1/tokens/activation", app.createUserActivationTokenHandler)
	mux.HandleFunc("PUT /v1/tokens/activation", app.activateUserHandler)
	mux.HandleFunc("POST /v1/tokens/authentication", app.createAuthenticationTokenHandler)
	mux.HandleFunc("POST /v1/tokens/password-reset", app.createPasswordResetTokenHandler)
	mux.HandleFunc("PUT /v1/tokens/password-reset", app.resetPasswordHandler)

	mux.HandleFunc("POST /v1/movies", app.authorize([]internal.Permission{"movies:create"}, app.createMovieHandler))
	mux.HandleFunc("GET /v1/movies/{id}", app.getMovieHandler)
	mux.HandleFunc("GET /v1/movies", app.getMoviesHandler)
	mux.HandleFunc("PUT /v1/movies/{id}", app.authorize([]internal.Permission{"movies:update"}, app.updateMovieHandler))
	mux.HandleFunc("DELETE /v1/movies/{id}", app.authorize([]internal.Permission{"movies:delete"}, app.deleteMovieHandler))

	mux.HandleFunc("POST /v1/cinemas", app.authenticate(app.requireUserActivation(app.createCinemaHandler)))
	mux.HandleFunc("GET /v1/cinemas/{id}", app.getCinemaHandler)
	mux.HandleFunc("GET /v1/cinemas", app.getCinemasHandler)
	mux.HandleFunc("PUT /v1/cinemas/{id}", app.authenticate(app.requireUserActivation(app.updateCinemaHandler)))
	mux.HandleFunc("DELETE /v1/cinemas/{id}", app.authenticate(app.requireUserActivation(app.deleteCinemaHandler)))

	mux.HandleFunc("POST /v1/cinemas/{id}/halls", app.authenticate(app.requireUserActivation(app.createHallHandler)))
	mux.HandleFunc("GET /v1/cinemas/{id}/halls", app.getHallsHandler)
	mux.HandleFunc("PUT /v1/cinemas/halls/{id}", app.authenticate(app.requireUserActivation(app.updateHallHandler)))
	mux.HandleFunc("DELETE /v1/cinemas/halls/{id}", app.authenticate(app.requireUserActivation(app.deleteHallHandler)))

	mux.HandleFunc("POST /v1/cinemas/halls/{id}/seats", app.authenticate(app.requireUserActivation(app.createSeatHandler)))
	mux.HandleFunc("GET /v1/cinemas/halls/{id}/seats", app.getSeatsHandler)
	mux.HandleFunc("PUT /v1/cinemas/halls/seats/{id}", app.authenticate(app.requireUserActivation(app.updateSeatHandler)))
	mux.HandleFunc("DELETE /v1/cinemas/halls/seats/{id}", app.authenticate(app.requireUserActivation(app.deleteSeatHandler)))

	mux.HandleFunc("POST /v1/schedules", app.authenticate(app.requireUserActivation(app.createScheduleHandler)))
	mux.HandleFunc("GET /v1/schedules", app.getSchedulesHandler)
	mux.HandleFunc("PUT /v1/schedules/{id}", app.authenticate(app.requireUserActivation(app.updateScheduleHandler)))
	mux.HandleFunc("DELETE /v1/schedules/{id}", app.authenticate(app.requireUserActivation(app.deleteScheduleHandler)))

	mux.HandleFunc("POST /v1/schedules/{id}/tickets", app.authenticate(app.requireUserActivation(app.createTicketsForScheduleHandler)))
	mux.HandleFunc("GET /v1/schedules/{id}/tickets", app.getTicketsForScheduleHandler)

	mux.HandleFunc("POST /v1/tickets/{id}/lock", app.authenticate(app.requireUserActivation(app.lockTicketHandler)))
	mux.HandleFunc("POST /v1/tickets/{id}/unlock", app.authenticate(app.requireUserActivation(app.unlockTicketHandler)))

	mux.HandleFunc("GET /v1/checkout", app.authenticate(app.requireUserActivation(app.getCheckoutHandler)))
	mux.HandleFunc("POST /v1/checkout", app.authenticate(app.requireUserActivation(app.checkoutHandler)))
	mux.HandleFunc("/v1/webhook", app.handleWebhook)
	mux.HandleFunc("/v1/checkout_sessions/cancel", app.handleCheckoutSessionCancel)

	if app.config.limiter.enabled {
		return app.enableCORS(app.recoverFromPanic(app.rateLimit(mux)))
	}

	return app.enableCORS(app.recoverFromPanic(mux))
}
