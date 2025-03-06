package main

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/AdventurerAmer/movie-reservation-system/internal"
	"golang.org/x/crypto/bcrypt"
)

// createUserActivationTokenHandler godoc
//
//	@Summary		Creates an activation token
//	@Description	creates an activation token and sends it to the email
//	@Tags			tokens
//	@Accept			json
//	@Produce		json
//	@Param			email	body		string	true	"email of the user"
//	@Success		201		{object}	ResponseMessage
//	@Failure		400		{object}	ViolationsMessage
//	@Failure		409		{object}	ResponseMessage
//	@Failure		500		{object}	ResponseError
//	@Router			/tokens/activation [post]
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

	writeJSON(ResponseMessage{Message: "activation token was send to the provided email"}, http.StatusCreated, w)
}

type ActivateUserResponse struct {
	User *internal.User `json:"user"`
}

// activateUserHandler godoc
//
//	@Summary		Activates a user
//	@Description	activates a user
//	@Tags			tokens
//	@Accept			json
//	@Produce		json
//	@Param			token	body		string	true	"token"
//	@Success		200		{object}	internal.User
//	@Failure		409		{object}	ResponseMessage
//	@Failure		500		{object}	ResponseError
//	@Router			/tokens/activation [put]
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

	writeJSON(ActivateUserResponse{User: u}, http.StatusOK, w)
}

type CreateAuthenticationTokenResponse struct {
	Token string `json:"token"`
}

// createAuthenticationTokenHandler godoc
//
//	@Summary		Creates an auth token
//	@Description	creates an auth token
//	@Tags			tokens
//	@Accept			json
//	@Produce		json
//	@Param			email		body		string	true	"email"
//	@Param			password	body		string	true	"password"
//
// @Success		201			{object}	CreateAuthenticationTokenResponse
// @Failure		400			{object}	ViolationsMessage
// @Failure		409			{object}	ResponseMessage
// @Failure		500			{object}	ResponseError
// @Router			/tokens/authentication [post]
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
	writeJSON(CreateAuthenticationTokenResponse{Token: token}, http.StatusCreated, w)
}

// createPasswordResetTokenHandler godoc
//
//	@Summary		Creates a password-reset token
//	@Description	creates a password-reset token and sends the token to the given email
//	@Tags			tokens
//	@Accept			json
//	@Produce		json
//	@Param			email	body		string	true	"email"
//	@Success		201		{object}	ResponseMessage
//	@Failure		400		{object}	ViolationsMessage
//	@Failure		409		{object}	ResponseMessage
//	@Failure		500		{object}	ResponseError
//	@Router			/tokens/password-reset [post]
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
	writeJSON(ResponseMessage{Message: "password token was send to the provided email"}, http.StatusCreated, w)
}

// resetPasswordHandler godoc
//
//	@Summary		Creates a password-reset token
//	@Description	creates a password-reset token
//	@Tags			tokens
//	@Accept			json
//	@Produce		json
//	@Param			email	body		string	true	"email"
//
//	@Success		200		{object}	ResponseMessage
//	@Failure		400		{object}	ViolationsMessage
//	@Failure		409		{object}	ResponseMessage
//	@Failure		500		{object}	ResponseError
//	@Router			/tokens/password-reset [put]
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

	writeJSON(ResponseMessage{Message: "password was reset"}, http.StatusOK, w)
}
