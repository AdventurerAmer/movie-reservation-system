package main

import (
	"net/http"
)

func (app *Application) healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	res := map[string]any{
		"status":     "up",
		"enviroment": app.config.environment,
		"version":    Version,
	}
	writeJSON(res, http.StatusOK, w)
}
