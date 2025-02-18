package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"time"

	"github.com/shopspring/decimal"
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

	title := QueryStringOr(r, "title", "")
	genres := QueryCSVOr(r, "genres", []string{})
	page := QueryIntOr(r, "page", 1, v)
	pageSize := QueryIntOr(r, "page_size", 20, v)
	sort := QueryStringOr(r, "sort", "id")

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

	name := QueryStringOr(r, "name", "")
	location := QueryStringOr(r, "location", "")
	page := QueryIntOr(r, "page", 1, v)
	pageSize := QueryIntOr(r, "page_size", 20, v)
	sort := QueryStringOr(r, "sort", "id")

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
		Name               *string          `json:"name"`
		SeatingArrangement *string          `json:"seating_arrangement"`
		SeatPrice          *decimal.Decimal `json:"seat_price"`
	}
	if err := readJSON(r, &req); err != nil {
		writeBadRequest(err, w)
		return
	}
	v := NewValidator()
	if req.Name != nil {
		v.Check(*req.Name != "", "name", "must be provided")
	}
	if req.SeatingArrangement != nil {
		v.Check(*req.SeatingArrangement != "", "seating_arrangement", "must be provided")
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
	if req.SeatingArrangement != nil {
		h.SeatingArrangement = *req.SeatingArrangement
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
