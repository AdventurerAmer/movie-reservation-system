package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
)

// Respons Message
type ResponseMessage struct {
	Message string `json:"message"` // Message
}

// ResponseError
type ResponseError struct {
	Error string `json:"error"` // Error
}

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

func getPathValuePositiveInt(r *http.Request, p string) (int, error) {
	v, err := strconv.Atoi(r.PathValue(p))
	if err != nil {
		return 0, fmt.Errorf(`invalid path parameter %q must be a positive integer`, p)
	}
	if v <= 0 {
		return 0, fmt.Errorf(`invalid path parameter %q must be a positive integer`, p)
	}
	return v, nil
}

func getIDFromPathValue(r *http.Request) (int, error) {
	id, err := getPathValuePositiveInt(r, "id")
	if err != nil {
		return 0, err
	}
	return id, nil
}

func getQueryStringOr(r *http.Request, key string, defaultValue string) string {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultValue
	}
	return s
}

func getQueryCSVOr(r *http.Request, key string, defaultValue []string) []string {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultValue
	}
	return strings.Split(s, ",")
}

func getQueryIntOr(r *http.Request, key string, defaultValue int, v *Validator) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultValue
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		v.Check(false, key, "must be a valid integer")
	}
	return i
}

func readJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(dst)
	if err != nil {
		var synatxErr *json.SyntaxError
		var unmarshalTypeErr *json.UnmarshalTypeError
		var invalidUnmarshalErr *json.InvalidUnmarshalError
		switch {
		case errors.Is(err, io.ErrUnexpectedEOF):
			return fmt.Errorf("body contains malformed JSON")
		case errors.Is(err, io.EOF):
			return fmt.Errorf("body must not empty")
		case errors.As(err, &synatxErr):
			return fmt.Errorf("body contains malformed JSON at character %d", synatxErr.Offset)
		case errors.As(err, &unmarshalTypeErr):
			if unmarshalTypeErr.Field != "" {
				return fmt.Errorf("body contains incorrect JSON type for field %q", unmarshalTypeErr.Field)
			}
			return fmt.Errorf("body contains malformed JSON at character %d", unmarshalTypeErr.Offset)
		case errors.As(err, &invalidUnmarshalErr):
			panic(err)
		default:
			return err
		}
	}
	return nil
}

func writeJSON(src any, status int, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	var b bytes.Buffer
	err := json.NewEncoder(&b).Encode(src)
	if err != nil {
		log.Printf("failed to encode %v: %v\n", src, err)
		w.Write(InternalServerErrorBuf.Bytes())
		return
	}
	w.Write(b.Bytes())
}

func writeError(err error, status int, w http.ResponseWriter) {
	res := map[string]any{"error": err.Error()}
	writeJSON(res, status, w)
}

func writeErrors(v *Validator, w http.ResponseWriter) {
	res := map[string]any{"errors": v.violations}
	writeJSON(res, http.StatusBadRequest, w)
}

func writeServerErr(err error, w http.ResponseWriter) {
	log.Printf("%v\n%v\n", err, string(debug.Stack()))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	w.Write(InternalServerErrorBuf.Bytes())
}

func writeBadRequest(err error, w http.ResponseWriter) {
	writeError(err, http.StatusBadRequest, w)
}

func writeNotFound(w http.ResponseWriter) {
	res := map[string]any{
		"message": "resource not found",
	}
	writeJSON(res, http.StatusNotFound, w)
}

func writeForbidden(w http.ResponseWriter) {
	writeError(errors.New("permission denied"), http.StatusForbidden, w)
}
