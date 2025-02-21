package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
	"github.com/stripe/stripe-go/webhook"
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
		writeServerErr(err, w)
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
		writeServerErr(err, w)
		return
	}

	user, err := app.storage.CreateUser(*req.Name, *req.Email, passwordHash)
	if err != nil {
		writeError(err, http.StatusConflict, w)
		return
	}

	token := generateToken()
	_, err = app.storage.CreateTokenForUser(user.ID, TokenScopeActivation, token, 10*time.Minute)
	if err != nil {
		writeServerErr(err, w)
		return
	}

	data := map[string]any{
		"token": token,
	}
	app.Go(app.SendMail(user.Email, ActivateUserTmpl, data))

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
	u := getUserFromRequestContext(r)
	if u == nil {
		writeServerErr(err, w)
		return
	}
	if u.ID != int64(id) {
		writeForbidden(w)
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
		Name *string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}

	v := NewValidator()
	v.CheckUsername(req.Name)

	if v.HasErrors() {
		writeErrors(v, w)
		return
	}

	u := getUserFromRequestContext(r)
	if u == nil {
		writeServerErr(errors.New("user must be authenticated"), w)
		return
	}

	if u.ID != int64(id) {
		writeForbidden(w)
		return
	}

	if req.Name != nil {
		u.Name = *req.Name
	}

	err = app.storage.UpdateUser(u)
	if err != nil {
		writeServerErr(err, w)
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

	u := getUserFromRequestContext(r)
	if u == nil {
		writeServerErr(err, w)
		return
	}

	if u.ID != int64(id) {
		writeForbidden(w)
		return
	}

	err = app.storage.DeleteUser(u)
	if err != nil {
		writeServerErr(err, w)
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
		writeServerErr(err, w)
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

	err = app.storage.DeleteAllTokensForUser(u.ID, []TokenScope{TokenScopeActivation})
	if err != nil {
		writeServerErr(err, w)
		return
	}

	token := generateToken()
	_, err = app.storage.CreateTokenForUser(u.ID, TokenScopeActivation, token, 10*time.Minute)
	if err != nil {
		log.Println(err)
		writeServerErr(err, w)
		return
	}

	data := map[string]any{
		"token": token,
	}
	app.Go(app.SendMail(u.Email, ActivateUserTmpl, data))

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
	u, err := app.storage.GetUserFromToken(TokenScopeActivation, req.Token)
	if err != nil {
		log.Println(err)
		writeServerErr(err, w)
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
		writeServerErr(err, w)
		return
	}

	res := map[string]any{
		"user": u,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) createAuthenticationTokenHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    *string `json:"email"`
		Password *string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}
	v := NewValidator()
	v.CheckEmail(req.Email)
	v.CheckPassword(req.Password)
	if v.HasErrors() {
		writeErrors(v, w)
		return
	}
	u, err := app.storage.GetUserByEmail(*req.Email)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if u == nil {
		writeError(errors.New("invalid credentials"), http.StatusUnauthorized, w)
		return
	}
	if bcrypt.CompareHashAndPassword(u.PasswordHash, []byte(*req.Password)) != nil {
		writeError(errors.New("invalid credentials"), http.StatusUnauthorized, w)
		return
	}

	err = app.storage.DeleteAllTokensForUser(u.ID, []TokenScope{TokenScopeAuthentication})
	if err != nil {
		writeServerErr(err, w)
		return
	}

	token := generateToken()
	_, err = app.storage.CreateTokenForUser(u.ID, TokenScopeAuthentication, token, 24*time.Hour)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"token": token,
	}
	writeJSON(res, http.StatusCreated, w)
}

func (app *Application) createPasswordResetTokenHandler(w http.ResponseWriter, r *http.Request) {
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
		writeServerErr(err, w)
		return
	}
	if u == nil {
		res := map[string]any{"message": "invalid email"}
		writeJSON(res, http.StatusConflict, w)
		return
	}

	err = app.storage.DeleteAllTokensForUser(u.ID, []TokenScope{TokenScopePasswordReset})
	if err != nil {
		writeServerErr(err, w)
		return
	}

	token := generateToken()
	_, err = app.storage.CreateTokenForUser(u.ID, TokenScopePasswordReset, token, 10*time.Minute)
	if err != nil {
		writeServerErr(err, w)
		return
	}

	data := map[string]any{
		"token": token,
	}
	app.Go(app.SendMail(u.Email, ResetPasswordTempl, data))

	res := map[string]any{
		"message": "password token was send to the provided email",
	}
	writeJSON(res, http.StatusCreated, w)
}

func (app *Application) resetPasswordHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password *string `json:"password"`
		Token    *string `json:"token"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}
	v := NewValidator()
	v.CheckPassword(req.Password)
	v.Check(req.Token != nil, "token", "must be provided")
	if req.Token != nil {
		v.Check(*req.Token != "", "token", "must be provided")
	}
	if v.HasErrors() {
		writeErrors(v, w)
		return
	}
	u, err := app.storage.GetUserFromToken(TokenScopePasswordReset, *req.Token)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if u == nil {
		writeError(errors.New("invalid token"), http.StatusConflict, w)
		return
	}

	err = app.storage.DeleteAllTokensForUser(u.ID, []TokenScope{TokenScopePasswordReset, TokenScopeAuthentication})
	if err != nil {
		writeServerErr(err, w)
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeServerErr(err, w)
		return
	}

	u.PasswordHash = passwordHash
	err = app.storage.UpdateUser(u)
	if err != nil {
		writeServerErr(err, w)
		return
	}

	res := map[string]any{
		"message": "password was reset",
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) createMovieHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title   string   `json:"title"`
		Runtime int32    `json:"runtime"`
		Year    int32    `json:"year"`
		Genres  []string `json:"genres"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}

	v := NewValidator()
	v.Check(req.Title != "", "title", "must be provided")
	v.Check(req.Runtime > 0, "runtime", "must be greater than zero")
	v.Check(req.Year > 0, "year", "must be greater than zero")
	v.Check(len(req.Genres) != 0, "genres", "must be provided")

	for idx, g := range req.Genres {
		v.Check(g != "", fmt.Sprintf("genre at index: %d", idx), "must be provided")
	}

	if v.HasErrors() {
		writeErrors(v, w)
		return
	}

	m, err := app.storage.CreateMovie(req.Title, req.Runtime, req.Year, req.Genres)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"movie": m,
	}
	writeJSON(res, http.StatusCreated, w)
}

func (app *Application) getMovieHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	m, err := app.storage.GetMovieByID(int64(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if m == nil {
		writeNotFound(w)
		return
	}
	res := map[string]any{
		"movie": m,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) getMoviesHandler(w http.ResponseWriter, r *http.Request) {
	v := NewValidator()

	title := getQueryStringOr(r, "title", "")
	genres := getQueryCSVOr(r, "genres", []string{})
	page := getQueryIntOr(r, "page", 1, v)
	pageSize := getQueryIntOr(r, "page_size", 20, v)
	sort := getQueryStringOr(r, "sort", "id")

	v.Check(page > 0 && page <= 10_000_000, "page", "must be between 1 and 10_000_000")
	v.Check(pageSize > 0 && pageSize <= 100, "page_size", "must be between 1 and 100")

	sortList := []string{"id", "-id", "title", "-title", "year", "-year", "runtime", "-runtime"}
	v.Check(slices.Contains(sortList, sort), fmt.Sprintf("sort-%s", sort), "not supported")

	if v.HasErrors() {
		writeErrors(v, w)
		return
	}

	movies, meta, err := app.storage.GetMovies(title, genres, page, pageSize, sort)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"movies": movies,
		"meta":   meta,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) updateMovieHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	var req struct {
		Title   *string   `json:"title"`
		Runtime *int32    `json:"runtime"`
		Year    *int32    `json:"year"`
		Genres  *[]string `json:"genres"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}

	v := NewValidator()
	if req.Title != nil {
		v.Check(*req.Title != "", "title", "must be provided")
	}
	if req.Runtime != nil {
		v.Check(*req.Runtime > 0, "runtime", "must be greater than zero")
	}
	if req.Year != nil {
		v.Check(*req.Year > 0, "year", "must be greater than zero")
	}
	if req.Genres != nil {
		v.Check(len(*req.Genres) != 0, "genres", "must be provided")
		for idx, g := range *req.Genres {
			v.Check(g != "", fmt.Sprintf("genre at index: %d", idx), "must be provided")
		}
	}
	if v.HasErrors() {
		writeErrors(v, w)
		return
	}
	m, err := app.storage.GetMovieByID(int64(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if m == nil {
		writeNotFound(w)
		return
	}
	if req.Title != nil {
		m.Title = *req.Title
	}
	if req.Runtime != nil {
		m.Runtime = *req.Runtime
	}
	if req.Year != nil {
		m.Year = *req.Year
	}
	if req.Genres != nil {
		m.Genres = *req.Genres
	}
	err = app.storage.UpdateMovie(m)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"movie": m,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) deleteMovieHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	m, err := app.storage.GetMovieByID(int64(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if m == nil {
		writeNotFound(w)
		return
	}
	err = app.storage.DeleteMovie(m)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"message": "resource deleted successfully",
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) createCinemaHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Location string `json:"location"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}

	v := NewValidator()
	v.Check(req.Name != "", "name", "must be provided")
	v.Check(req.Location != "", "location", "must be provided")

	if v.HasErrors() {
		writeErrors(v, w)
		return
	}

	u := getUserFromRequestContext(r)
	if u == nil {
		writeServerErr(errors.New("user not authenticated"), w)
		return
	}

	c, err := app.storage.CreateCinema(u.ID, req.Name, req.Location)
	if err != nil {
		writeServerErr(err, w)
		return
	}

	res := map[string]any{
		"cinema": c,
	}
	writeJSON(res, http.StatusCreated, w)
}

func (app *Application) getCinemaHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	c, err := app.storage.GetCinemaByID(int32(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if c == nil {
		writeNotFound(w)
		return
	}
	res := map[string]any{
		"cinema": c,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) getCinemasHandler(w http.ResponseWriter, r *http.Request) {
	v := NewValidator()

	name := getQueryStringOr(r, "name", "")
	location := getQueryStringOr(r, "location", "")
	page := getQueryIntOr(r, "page", 1, v)
	pageSize := getQueryIntOr(r, "page_size", 20, v)
	sort := getQueryStringOr(r, "sort", "id")

	v.Check(page > 0 && page <= 10_000_000, "page", "must be between 1 and 10_000_000")
	v.Check(pageSize > 0 && pageSize <= 100, "page_size", "must be between 1 and 100")

	sortList := []string{"id", "-id", "name", "-name", "location", "-location"}
	v.Check(slices.Contains(sortList, sort), fmt.Sprintf("sort-%s", sort), "not supported")

	if v.HasErrors() {
		writeErrors(v, w)
		return
	}

	cinemas, meta, err := app.storage.GetCinemas(name, location, page, pageSize, sort)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"cinemas": cinemas,
		"meta":    meta,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) updateCinemaHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	var req struct {
		Name     *string `json:"name"`
		Location *string `json:"location"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}

	v := NewValidator()
	if req.Name != nil {
		v.Check(*req.Name != "", "name", "must be provided")
	}
	if req.Location != nil {
		v.Check(*req.Location != "location", "location", "must be provided")
	}
	v.Check(req.Name != nil || req.Location != nil, "name or location", "must be provided")

	if v.HasErrors() {
		writeErrors(v, w)
		return
	}

	u := getUserFromRequestContext(r)
	if u == nil {
		writeServerErr(errors.New("user is not authenticated"), w)
		return
	}

	c, err := app.storage.GetCinemaByID(int32(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if c == nil {
		writeNotFound(w)
		return
	}

	if c.OwnerID != u.ID {
		writeForbidden(w)
		return
	}

	if req.Name != nil {
		c.Name = *req.Name
	}

	if req.Location != nil {
		c.Location = *req.Location
	}

	err = app.storage.UpdateCinema(c)
	if err != nil {
		writeServerErr(err, w)
		return
	}

	res := map[string]any{
		"cinema": c,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) deleteCinemaHandler(w http.ResponseWriter, r *http.Request) {
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
	c, err := app.storage.GetCinemaByID(int32(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if c == nil {
		writeNotFound(w)
		return
	}
	if c.OwnerID != u.ID {
		writeForbidden(w)
		return
	}
	err = app.storage.DeleteCinema(c)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"message": "resource deleted successfully",
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) createHallHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}

	var req struct {
		Name               string          `json:"name"`
		SeatingArrangement string          `json:"seat_arrangement"`
		SeatPrice          decimal.Decimal `json:"seat_price"`
	}

	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}

	v := NewValidator()
	v.Check(req.Name != "", "name", "must be provided")
	v.Check(req.SeatingArrangement != "", "seat_arrangement", "must be provided")
	v.Check(req.SeatPrice.GreaterThan(decimal.Zero), "seat_price", "must be greater than zero")

	if v.HasErrors() {
		writeErrors(v, w)
		return
	}

	u := getUserFromRequestContext(r)
	if u == nil {
		writeServerErr(errors.New("user is not authenticated"), w)
		return
	}
	c, err := app.storage.GetCinemaByID(int32(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if c == nil {
		writeNotFound(w)
		return
	}
	if c.OwnerID != u.ID {
		writeForbidden(w)
		return
	}
	h, err := app.storage.CreateHall(req.Name, c.ID, req.SeatingArrangement, req.SeatPrice)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"hall": h,
	}
	writeJSON(res, http.StatusCreated, w)
}

func (app *Application) getHallHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	h, err := app.storage.GetHallByID(int32(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if h == nil {
		writeNotFound(w)
		return
	}
	res := map[string]any{
		"hall": h,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) getHallsHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	halls, err := app.storage.GetHallsForCinema(int32(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"halls": halls,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) updateHallHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	var req struct {
		Name            *string          `json:"name"`
		SeatArrangement *string          `json:"seat_arrangement"`
		SeatPrice       *decimal.Decimal `json:"seat_price"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}
	v := NewValidator()
	if req.Name != nil {
		v.Check(*req.Name != "", "name", "must be provided")
	}
	if req.SeatArrangement != nil {
		v.Check(*req.SeatArrangement != "", "seat_arrangement", "must be provided")
	}
	if req.SeatPrice != nil {
		v.Check(req.SeatPrice.GreaterThan(decimal.Zero), "seat_price", "must be provided")
	}
	if v.HasErrors() {
		writeErrors(v, w)
		return
	}
	u := getUserFromRequestContext(r)
	if u == nil {
		writeServerErr(errors.New("user is not authenticated"), w)
		return
	}
	h, c, err := app.storage.GetHallCinema(int32(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if h == nil {
		writeNotFound(w)
		return
	}
	if c.OwnerID != u.ID {
		writeForbidden(w)
		return
	}
	if req.Name != nil {
		h.Name = *req.Name
	}
	if req.SeatArrangement != nil {
		h.SeatArrangement = *req.SeatArrangement
	}
	if req.SeatPrice != nil {
		h.SeatPrice = *req.SeatPrice
	}
	err = app.storage.UpdateHall(h)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"hall": h,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) deleteHallHandler(w http.ResponseWriter, r *http.Request) {
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
	h, c, err := app.storage.GetHallCinema(int32(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if h == nil {
		writeNotFound(w)
		return
	}
	if c.OwnerID != u.ID {
		writeForbidden(w)
		return
	}
	err = app.storage.DeleteHall(h)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"message": "resource deleted successfully",
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) createSeatHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	var req struct {
		Coordinates string `json:"coordinates"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}
	v := NewValidator()
	v.Check(req.Coordinates != "", "coordinates", "must be provided")
	if v.HasErrors() {
		writeErrors(v, w)
		return
	}
	u := getUserFromRequestContext(r)
	if u == nil {
		writeServerErr(errors.New("user is not authenticated"), w)
		return
	}
	h, c, err := app.storage.GetHallCinema(int32(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if h == nil {
		writeNotFound(w)
		return
	}
	if c.OwnerID != c.OwnerID {
		writeForbidden(w)
		return
	}
	seat, err := app.storage.CreateSeat(int32(id), req.Coordinates)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"seat": seat,
	}
	writeJSON(res, http.StatusCreated, w)
}

func (app *Application) getSeatHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	s, err := app.storage.GetSeatByID(int32(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if s == nil {
		writeNotFound(w)
		return
	}
	res := map[string]any{
		"seat": s,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) getSeatsHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	seats, err := app.storage.GetSeatsForHall(int32(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"seats": seats,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) updateSeatHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	var req struct {
		Coordinates string `json:"coordinates"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}
	v := NewValidator()
	v.Check(req.Coordinates != "", "coordinates", "must be provided")
	if v.HasErrors() {
		writeErrors(v, w)
		return
	}
	u := getUserFromRequestContext(r)
	if u == nil {
		writeServerErr(errors.New("user is not authenticated"), w)
		return
	}

	c, _, s, err := app.storage.GetCinemaHallSeat(int32(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if c == nil {
		writeNotFound(w)
		return
	}
	if c.OwnerID != u.ID {
		writeForbidden(w)
		return
	}

	s.Coordinates = req.Coordinates
	err = app.storage.UpdateSeat(s)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"seat": s,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) deleteSeatHandler(w http.ResponseWriter, r *http.Request) {
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

	c, _, s, err := app.storage.GetCinemaHallSeat(int32(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if c == nil {
		writeNotFound(w)
		return
	}
	if c.OwnerID != u.ID {
		writeForbidden(w)
		return
	}

	err = app.storage.DeleteSeat(s)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"message": "resouce delete successfully",
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) createScheduleHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MovieID  *int64           `json:"movie_id"`
		HallID   *int32           `json:"hall_id"`
		Price    *decimal.Decimal `json:"price"`
		StartsAt *time.Time       `json:"starts_at"`
		EndsAt   *time.Time       `json:"ends_at"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}
	v := NewValidator()
	v.Check(req.MovieID != nil, "movie_id", "must be provided")
	v.Check(req.HallID != nil, "hall_id", "must be provided")
	v.Check(req.Price != nil, "price", "must be provided")
	v.Check(req.StartsAt != nil, "starts_at", "must be provided")
	v.Check(req.EndsAt != nil, "ends_at", "must be provided")

	if req.MovieID != nil {
		v.Check(*req.MovieID > 0, "movie_id", "must be greater then zero")
	}
	if req.HallID != nil {
		v.Check(*req.HallID > 0, "hall_id", "must be greater then zero")
	}
	if req.Price != nil {
		v.Check(req.Price.GreaterThanOrEqual(decimal.Zero), "price", "must be greater than or equal to zero")
	}
	if req.StartsAt != nil {
		v.Check(req.StartsAt.After(time.Now()), "starts_at", "invalid time")
	}
	if req.EndsAt != nil {
		v.Check(req.EndsAt.After(time.Now()), "ends_at", "invalid time")
	}
	if req.StartsAt != nil && req.EndsAt != nil {
		v.Check(req.EndsAt.After(*req.StartsAt), "ends_at", "must come after starts_at")
	}

	if v.HasErrors() {
		writeErrors(v, w)
		return
	}

	u := getUserFromRequestContext(r)
	if u == nil {
		writeServerErr(errors.New("user is not authenticated"), w)
		return
	}

	m, err := app.storage.GetMovieByID(*req.MovieID)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if m == nil {
		writeError(fmt.Errorf("couldn't find movie with id %d", *req.MovieID), http.StatusNotFound, w)
		return
	}

	_, c, err := app.storage.GetHallCinema(*req.HallID)
	if err != nil {
		writeServerErr(err, w)
		return
	}

	if c == nil {
		writeError(fmt.Errorf("couldn't find hall with id %d", *req.HallID), http.StatusNotFound, w)
		return
	}

	if c.OwnerID != u.ID {
		writeForbidden(w)
		return
	}

	s, err := app.storage.GetSchedule(*req.MovieID, *req.HallID, *req.StartsAt, *req.EndsAt, 0)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if s != nil {
		res := map[string]any{
			"message":  "there is already a schedule that intersets with this schedule",
			"schedule": s,
		}
		writeJSON(res, http.StatusConflict, w)
		return
	}
	s, err = app.storage.CreateSchedule(*req.MovieID, *req.HallID, *req.Price, *req.StartsAt, *req.EndsAt)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"schedule": s,
	}
	writeJSON(res, http.StatusCreated, w)
}

func (app *Application) getSchedulesHandler(w http.ResponseWriter, r *http.Request) {
	v := NewValidator()
	movie_id := getQueryIntOr(r, "movie_id", 0, v)
	hall_id := getQueryIntOr(r, "hall_id", 0, v)
	page := getQueryIntOr(r, "page", 1, v)
	pageSize := getQueryIntOr(r, "page_size", 20, v)
	sort := getQueryStringOr(r, "sort", "starts_at")

	v.Check(movie_id > 0, "movie_id", "must be greater than zero")
	v.Check(hall_id > 0, "hall_id", "must be greater than zero")
	v.Check(page >= 1 && page <= 10_000_000, "page", "must be between 1 and 10_000_000")
	v.Check(pageSize >= 1 && pageSize <= 100, "page", "must be between 1 and 100")
	sortList := []string{"id", "-id", "price", "-price", "starts_at", "-starts_at", "ends_at", "-ends_at"}
	v.Check(slices.Contains(sortList, sort), "sort", "not supported")

	if v.HasErrors() {
		writeErrors(v, w)
		return
	}

	s, m, err := app.storage.GetSchedules(int64(movie_id), int32(hall_id), sort, page, pageSize)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"schedules": s,
		"meta":      m,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) updateScheduleHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	var req struct {
		Price    *decimal.Decimal `json:"price"`
		StartsAt *time.Time       `json:"starts_at"`
		EndsAt   *time.Time       `json:"ends_at"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}
	v := NewValidator()
	if req.Price != nil {
		v.Check(req.Price.GreaterThanOrEqual(decimal.Zero), "price", "must be greater than or equal to zero")
	}
	if req.StartsAt != nil {
		v.Check(req.StartsAt.After(time.Now()), "starts_at", "invalid time")
	}
	if req.EndsAt != nil {
		v.Check(req.EndsAt.After(time.Now()), "ends_at", "invalid time")
	}
	if req.StartsAt != nil && req.EndsAt != nil {
		v.Check(req.EndsAt.After(*req.EndsAt), "ends_at", "must come after starts_at")
	}
	if v.HasErrors() {
		writeErrors(v, w)
		return
	}

	s, err := app.storage.GetScheduleByID(int64(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}

	if s == nil {
		writeNotFound(w)
		return
	}

	if req.Price != nil {
		s.Price = *req.Price
	}

	if req.StartsAt != nil {
		s.StartsAt = *req.StartsAt
	}

	if req.EndsAt != nil {
		s.EndsAt = *req.EndsAt
	}

	if req.StartsAt != nil || req.EndsAt != nil {
		conflictingSchedule, err := app.storage.GetSchedule(s.MovieID, s.HallID, s.StartsAt, s.EndsAt, s.ID)
		if err != nil {
			writeServerErr(err, w)
			return
		}
		if conflictingSchedule != nil {
			res := map[string]any{
				"message":  "there is already a schedule that intersets with this schedule",
				"schedule": conflictingSchedule,
			}
			writeJSON(res, http.StatusConflict, w)
			return
		}
	}

	err = app.storage.UpdateSchedule(s)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"schedule": s,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) deleteScheduleHandler(w http.ResponseWriter, r *http.Request) {
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
	s, err := app.storage.GetScheduleByID(int64(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if s == nil {
		writeNotFound(w)
		return
	}
	err = app.storage.DeleteSchedule(s)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"message": "resource deleted successfully",
	}
	writeJSON(res, http.StatusOK, w)
}

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
	s, err := app.storage.GetScheduleByID(int64(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if s == nil {
		writeNotFound(w)
		return
	}
	n, err := app.storage.CreateTicketsForSchedule(s)
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
	ticketSeats, err := app.storage.GetTicketSeatsForSchedule(int64(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"tickets": ticketSeats,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) updateTicketHandler(w http.ResponseWriter, r *http.Request) {
	id, err := getIDFromPathValue(r)
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	var req struct {
		StateID *TicketState `json:"state_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}
	v := NewValidator()
	v.Check(id > 0, "id", "must be provided")
	v.Check(req.StateID != nil, "state_id", "must be provided")
	validStates := []TicketState{TicketStateUnsold, TicketStateLocked, TicketStateSold}
	if req.StateID != nil {
		v.Check(slices.Contains(validStates, *req.StateID), "state_id", "unsupported")
	}
	if v.HasErrors() {
		writeErrors(v, w)
		return
	}
	u := getUserFromRequestContext(r)
	if u == nil {
		writeServerErr(errors.New("user is not authenticated"), w)
		return
	}
	t, err := app.storage.GetTicketByID(int64(id))
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	if t == nil {
		writeNotFound(w)
		return
	}

	if t.StateID == TicketStateSold {
		res := map[string]any{
			"message": "invalid ticket state transform",
		}
		writeJSON(res, http.StatusConflict, w)
		return
	}

	if *req.StateID == TicketStateLocked && t.StateID != TicketStateUnsold {
		res := map[string]any{
			"message": "invalid ticket state transform",
		}
		writeJSON(res, http.StatusConflict, w)
		return
	}

	if *req.StateID == TicketStateSold && t.StateID != TicketStateLocked {
		res := map[string]any{
			"message": "invalid ticket state transform",
		}
		writeJSON(res, http.StatusConflict, w)
		return
	}

	c, _, _, err := app.storage.GetCinemaHallSeat(t.SeatID)
	if err != nil {
		writeServerErr(err, w)
		return
	}

	if c.OwnerID != u.ID {
		writeForbidden(w)
		return
	}

	t.StateID = *req.StateID
	err = app.storage.UpdateTicket(t)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"ticket": t,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) deleteTicketHandler(w http.ResponseWriter, r *http.Request) {
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

	t, err := app.storage.GetTicketByID(int64(id))
	if err != nil {
		writeBadRequest(err, w)
		return
	}
	if t == nil {
		writeNotFound(w)
		return
	}
	if t.StateID == TicketStateSold || t.StateID == TicketStateLocked {
		res := map[string]any{
			"message": "ticket is sold or locked",
		}
		writeJSON(res, http.StatusConflict, w)
		return
	}

	c, _, _, err := app.storage.GetCinemaHallSeat(t.SeatID)
	if err != nil {
		writeServerErr(err, w)
		return
	}

	if c.OwnerID != u.ID {
		writeForbidden(w)
		return
	}

	err = app.storage.DeleteTicket(t)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"message": "resources deleted successfully",
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
	t, err := app.storage.GetTicketByID(int64(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if t == nil {
		writeNotFound(w)
		return
	}
	if t.StateID == TicketStateLocked {
		res := map[string]any{
			"message": "ticket is already locked",
		}
		writeJSON(res, http.StatusConflict, w)
		return
	}
	if t.ScheduleID == int64(TicketStateSold) {
		res := map[string]any{
			"message": "ticket is already sold",
		}
		writeJSON(res, http.StatusConflict, w)
		return
	}
	s, err := app.storage.GetScheduleByID(t.ScheduleID)
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
	checkoutSession, err := app.storage.GetCheckoutSessionByUserID(u)
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

	err = app.storage.LockTicket(t, u)
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
	t, err := app.storage.GetTicketByID(int64(id))
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if t == nil {
		writeNotFound(w)
		return
	}
	if t.StateID != TicketStateLocked {
		res := map[string]any{
			"message": "ticket must be locked to unlock it",
		}
		writeJSON(res, http.StatusConflict, w)
		return
	}

	checkoutSession, err := app.storage.GetCheckoutSessionByUserID(u)
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

	err = app.storage.UnlockTicketByUser(t, u)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"ticket": t,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) getCheckoutHandler(w http.ResponseWriter, r *http.Request) {
	u := getUserFromRequestContext(r)
	if u == nil {
		writeServerErr(errors.New("user is not authenticated"), w)
		return
	}
	ticketsCheckout, total, err := app.storage.GetTicketsCheckoutForUser(u)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	res := map[string]any{
		"tickets_checkout": ticketsCheckout,
		"total":            total,
	}
	writeJSON(res, http.StatusOK, w)
}

func (app *Application) checkoutHandler(w http.ResponseWriter, r *http.Request) {
	u := getUserFromRequestContext(r)
	if u == nil {
		writeServerErr(errors.New("user is not authenticated"), w)
		return
	}
	checkoutSession, err := app.storage.GetCheckoutSessionByUserID(u)
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
	ticketsCheckout, _, err := app.storage.GetTicketsCheckoutForUser(u)
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
	checkoutSession, err = app.storage.CreateCheckoutSession(u, s.ID)
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
			log.Println("Fulfill Payment")
			ses, err := app.storage.GetCheckoutSessionBySessionID(cs.ID)
			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if ses != nil {
				err = app.storage.FulfillCheckoutSessionForUser(cs.ID, ses.UserID)
				if err != nil {
					log.Println(err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				log.Println("Fulfill Session: ", cs.ID)
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
		log.Println("EventTypeCheckoutSessionExpired")
		ses, err := app.storage.GetCheckoutSessionBySessionID(cs.ID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if ses != nil {
			err = app.storage.DeleteCheckoutSessionBySessionID(ses.SessionID)
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
	cs, err := app.storage.GetCheckoutSessionBySessionID(sessionID)
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
			err = app.storage.DeleteCheckoutSessionBySessionID(cs.SessionID)
			if err != nil {
				log.Println(err)
			} else {
				log.Println("Deleted Session:", cs.SessionID)
			}
		}
	}
}
