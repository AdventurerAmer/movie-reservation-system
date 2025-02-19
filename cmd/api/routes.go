package main

import "net/http"

func composeRoutes(app *Application) http.Handler {
	mux := &http.ServeMux{}

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

	mux.HandleFunc("POST /v1/movies", app.createMovieHandler)
	mux.HandleFunc("GET /v1/movies/{id}", app.getMovieHandler)
	mux.HandleFunc("GET /v1/movies", app.getMoviesHandler)
	mux.HandleFunc("PUT /v1/movies/{id}", app.updateMovieHandler)
	mux.HandleFunc("DELETE /v1/movies/{id}", app.deleteMovieHandler)

	mux.HandleFunc("POST /v1/cinemas", app.authenticate(app.createCinemaHandler))
	mux.HandleFunc("GET /v1/cinemas/{id}", app.getCinemaHandler)
	mux.HandleFunc("GET /v1/cinemas", app.getCinemasHandler)
	mux.HandleFunc("PUT /v1/cinemas/{id}", app.authenticate(app.updateCinemaHandler))
	mux.HandleFunc("DELETE /v1/cinemas/{id}", app.authenticate(app.deleteCinemaHandler))

	mux.HandleFunc("POST /v1/cinemas/{id}/halls", app.authenticate(app.createHallHandler))
	mux.HandleFunc("GET /v1/cinemas/{id}/halls", app.getHallsHandler)
	mux.HandleFunc("PUT /v1/cinemas/halls/{id}", app.authenticate(app.updateHallHandler))
	mux.HandleFunc("DELETE /v1/cinemas/halls/{id}", app.authenticate(app.deleteHallHandler))

	mux.HandleFunc("POST /v1/cinemas/halls/{id}/seats", app.authenticate(app.createSeatHandler))
	mux.HandleFunc("GET /v1/cinemas/halls/{id}/seats", app.getSeatsHandler)
	mux.HandleFunc("PUT /v1/cinemas/halls/seats/{id}", app.authenticate(app.updateSeatHandler))
	mux.HandleFunc("DELETE /v1/cinemas/halls/seats/{id}", app.authenticate(app.deleteSeatHandler))

	mux.HandleFunc("POST /v1/schedules", app.authenticate(app.createScheduleHandler))
	mux.HandleFunc("GET /v1/schedules", app.getSchedulesHandler)
	mux.HandleFunc("PUT /v1/schedules/{id}", app.authenticate(app.updateScheduleHandler))
	mux.HandleFunc("DELETE /v1/schedules/{id}", app.authenticate(app.deleteScheduleHandler))

	return mux
}
