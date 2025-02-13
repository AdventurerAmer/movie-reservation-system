package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
)

var InternalServerErrorBuf bytes.Buffer

func init() {
	res := map[string]any{
		"message": "internal server error",
	}
	err := json.NewEncoder(&InternalServerErrorBuf).Encode(res)
	if err != nil {
		panic(err)
	}
}

func (app *Application) healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	res := map[string]any{
		"status":     "up",
		"enviroment": app.config.environment,
		"version":    Version,
	}
	writeJSON(res, w)
}

func writeJSON(src any, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	var b bytes.Buffer
	err := json.NewEncoder(&b).Encode(src)
	if err != nil {
		log.Printf("failed to encode %v: %v\n", src, err)
		w.Write(InternalServerErrorBuf.Bytes())
		return
	}
	w.Write(b.Bytes())
}
