package main

import (
	"net/http"
)

type HealthCheckResponse struct {
	Status     string `json:"status"`
	Enviroment string `json:"enviroment"`
	Version    string `json:"version"`
}

// healthCheckHandler godoc
//
//	@Summary		Gets Health Check status
//	@Description	gets a health check status
//	@Tags			checkouts
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	HealthCheckResponse
//	@Router			/healthcheck [get]
func (app *Application) healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	res := HealthCheckResponse{
		Status:     "up",
		Enviroment: app.config.environment,
		Version:    Version,
	}
	writeJSON(res, http.StatusOK, w)
}
