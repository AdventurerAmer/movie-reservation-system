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

	return mux
}
