package main

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/AdventurerAmer/movie-reservation-system/internal"
	"golang.org/x/crypto/bcrypt"
)

// createUserHandler godoc
//
//	@Summary		Create a new user
//	@Description	Create a new user by name, email, password
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			name		body		string	true "name of the user"
//	@Param			email		body		string	true "email of the user"
//	@Param			password	body		string	true "password of the user"
//	@Success		201			{object}	internal.User
//	@Failure		400			{object}	Violations
//	@Failure		409			{object}	ResponseMessage
//	@Failure		500			{object}	ResponseError
//	@Router			/users [post]
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

	u, err := app.storage.Users.GetByEmail(*req.Email)
	if err != nil {
		writeServerErr(err, w)
		return
	}

	if u != nil {
		res := map[string]any{
			"message": "email already exists",
		}
		writeJSON(res, http.StatusConflict, w)
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeServerErr(err, w)
		return
	}

	user, err := app.storage.Users.Create(*req.Name, *req.Email, passwordHash)
	if err != nil {
		writeServerErr(err, w)
		return
	}

	token := internal.GenerateToken()
	_, err = app.storage.Tokens.Create(user.ID, internal.TokenScopeActivation, token, 10*time.Minute)
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

	err = app.storage.Users.Update(u)
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

	err = app.storage.Users.Delete(u)
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

	u, err := app.storage.Users.GetByEmail(*req.Email)
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

	err = app.storage.Tokens.DeleteAll(u.ID, []internal.TokenScope{internal.TokenScopeActivation})
	if err != nil {
		writeServerErr(err, w)
		return
	}

	token := internal.GenerateToken()
	_, err = app.storage.Tokens.Create(u.ID, internal.TokenScopeActivation, token, 10*time.Minute)
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
	u, err := app.storage.Tokens.GetUser(internal.TokenScopeActivation, req.Token)
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
	err = app.storage.Users.Update(u)
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
	u, err := app.storage.Users.GetByEmail(*req.Email)
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

	err = app.storage.Tokens.DeleteAll(u.ID, []internal.TokenScope{internal.TokenScopeAuthentication})
	if err != nil {
		writeServerErr(err, w)
		return
	}

	token := internal.GenerateToken()
	_, err = app.storage.Tokens.Create(u.ID, internal.TokenScopeAuthentication, token, 24*time.Hour)
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

	u, err := app.storage.Users.GetByEmail(*req.Email)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if u == nil {
		res := map[string]any{"message": "invalid email"}
		writeJSON(res, http.StatusConflict, w)
		return
	}

	err = app.storage.Tokens.DeleteAll(u.ID, []internal.TokenScope{internal.TokenScopePasswordReset})
	if err != nil {
		writeServerErr(err, w)
		return
	}

	token := internal.GenerateToken()
	_, err = app.storage.Tokens.Create(u.ID, internal.TokenScopePasswordReset, token, 10*time.Minute)
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
	u, err := app.storage.Tokens.GetUser(internal.TokenScopePasswordReset, *req.Token)
	if err != nil {
		writeServerErr(err, w)
		return
	}
	if u == nil {
		writeError(errors.New("invalid token"), http.StatusConflict, w)
		return
	}

	err = app.storage.Tokens.DeleteAll(u.ID, []internal.TokenScope{internal.TokenScopePasswordReset, internal.TokenScopeAuthentication})
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
	err = app.storage.Users.Update(u)
	if err != nil {
		writeServerErr(err, w)
		return
	}

	res := map[string]any{
		"message": "password was reset",
	}
	writeJSON(res, http.StatusOK, w)
}
