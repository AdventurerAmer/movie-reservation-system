package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/crypto/bcrypt"
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
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) createUserHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     *string `json:"name"`
		Email    *string `json:"email"`
		Password *string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}
	v := NewValidator()

	v.CheckUsername(req.Name)
	v.CheckEmail(req.Email)
	v.CheckPassword(req.Password)

	if v.HasErrors() {
		writeErrors(v, w)
		return
	}

	u, err := app.storage.GetUserByEmail(*req.Email)
	if err != nil {
		writeServerErr(w)
		return
	}

	if u != nil {
		res := map[string]any{
			"message": "user with the provided email is already registered",
		}
		writeJSON(res, http.StatusConflict, w)
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeServerErr(w)
		return
	}

	user, err := app.storage.CreateUser(*req.Name, *req.Email, passwordHash)
	if err != nil {
		writeError(err, http.StatusConflict, w)
		return
	}

	hash, token := createToken()

	_, err = app.storage.CreateTokenForUser(user.ID, TokenScopeActivation, hash[:], 10*time.Minute)
	if err != nil {
		writeServerErr(w)
		return
	}

	app.Go(func() {
		tmpl, err := template.ParseFS(Templates, "templates/*.gotmpl")
		if err != nil {
			panic(err)
		}
		data := map[string]any{
			"token": token,
		}
		app.mailer.Send(u.Email, tmpl, data)
	})

	res := map[string]any{
		"user":    user,
		"message": "activation token was send to the provided email",
	}
	writeJSON(res, http.StatusCreated, w)
}

func (app *Application) getUserHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	u, err := app.storage.GetUserByID(int64(id))
	if err != nil {
		writeServerErr(w)
		return
	}
	res := map[string]any{
		"user": u,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) updateUserHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	var req struct {
		Name  *string `json:"name"`
		Email *string `json:"email"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}

	v := NewValidator()
	v.Check(req.Name != nil || req.Email != nil, "name or email", "must be provided")
	if req.Name != nil {
		v.CheckUsername(req.Name)
	}
	if req.Email != nil {
		v.CheckEmail(req.Email)
	}

	if v.HasErrors() {
		writeErrors(v, w)
		return
	}

	u, err := app.storage.GetUserByID(int64(id))
	if err != nil {
		writeServerErr(w)
		return
	}

	if req.Name != nil {
		u.Name = *req.Name
	}

	if req.Email != nil {
		u.Email = *req.Email
	}

	err = app.storage.UpdateUser(u)
	if err != nil {
		writeServerErr(w)
		return
	}

	res := map[string]any{
		"user": u,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	u, err := app.storage.GetUserByID(int64(id))
	if err != nil {
		writeServerErr(w)
		return
	}
	if u == nil {
		writeServerErr(w)
		return
	}
	err = app.storage.DeleteUser(u)
	if err != nil {
		writeServerErr(w)
		return
	}
	res := map[string]any{
		"message": "user delete successfully",
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) createUserActivationTokenHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email *string `json:"email"`
	}

	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}

	v := NewValidator()
	v.CheckEmail(req.Email)

	if v.HasErrors() {
		writeErrors(v, w)
		return
	}

	u, err := app.storage.GetUserByEmail(*req.Email)
	if err != nil {
		log.Println(err)
		writeServerErr(w)
		return
	}
	if u == nil {
		res := map[string]any{"message": "invalid email"}
		writeJSON(res, http.StatusConflict, w)
		return
	}

	if u.IsActivated {
		res := map[string]any{"message": "user is already activated"}
		writeJSON(res, http.StatusConflict, w)
		return
	}

	hash, token := createToken()

	_, err = app.storage.CreateTokenForUser(u.ID, TokenScopeActivation, hash[:], 10*time.Minute)
	if err != nil {
		log.Println(err)
		writeServerErr(w)
		return
	}

	app.Go(func() {
		tmpl, err := template.ParseFS(Templates, "templates/*.gotmpl")
		if err != nil {
			panic(err)
		}
		data := map[string]any{
			"token": token,
		}
		app.mailer.Send(u.Email, tmpl, data)
	})

	res := map[string]any{
		"message": "activation token was send to the provided email",
	}
	writeJSON(res, http.StatusCreated, w)
}

func (app *Application) activateUserHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}
	hash := sha256.Sum256([]byte(req.Token))
	u, err := app.storage.GetUserFromToken(TokenScopeActivation, hash[:])
	if err != nil {
		log.Println(err)
		writeServerErr(w)
		return
	}
	if u == nil {
		writeError(errors.New("invalid token"), http.StatusConflict, w)
		return
	}

	if u.IsActivated {
		writeError(errors.New("invalid token"), http.StatusConflict, w)
		return
	}

	u.IsActivated = true
	err = app.storage.UpdateUser(u)
	if err != nil {
		log.Println(err)
		writeServerErr(w)
		return
	}

	res := map[string]any{
		"user": u,
	}
	writeJSON(res, http.StatusOK, w)
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

func writeBadRequest(err error, w http.ResponseWriter) {
	writeError(err, http.StatusBadRequest, w)
}

func writeServerErr(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	w.Write(InternalServerErrorBuf.Bytes())
}
