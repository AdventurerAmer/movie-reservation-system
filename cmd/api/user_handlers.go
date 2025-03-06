package main

import (
	"errors"
	"net/http"
	"time"

	"github.com/AdventurerAmer/movie-reservation-system/internal"
	"golang.org/x/crypto/bcrypt"
)

type CreatedUserResponse struct {
	User    *internal.User `json:"user"`
	Message string         `json:"message"`
}

// createUserHandler godoc
//
//	@Summary		Creates a new user
//	@Description	creates a new user by name, email, password
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			name		body		string	true	"name of the user"
//	@Param			email		body		string	true	"email of the user"
//	@Param			password	body		string	true	"password of the user"
//	@Success		201			{object}	CreatedUserResponse
//	@Failure		400			{object}	ViolationsMessage
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
	res := CreatedUserResponse{User: user, Message: "activation token was send to the provided email"}
	writeJSON(res, http.StatusCreated, w)
}

type GetUserResponse struct {
	User *internal.User `json:"user"`
}

// getUserHandler godoc
//
//	@Summary		Get User Info
//	@Description	gets The user Info by ID
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"id of the user"
//	@Success		200	{object}	GetUserResponse
//	@Failure		400	{object}	ResponseError
//	@Failure		403	{object}	ResponseError
//	@Router			/users/{id} [get]
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
	writeJSON(GetUserResponse{User: u}, http.StatusOK, w)
}

type UpdateUserResponse struct {
	User *internal.User `json:"user"`
}

// updateUserHandler godoc
//
//	@Summary		Updates User Info
//	@Description	updates the user Info by ID
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			id		path		int		true	"id of the user"
//	@Param			name	body		string	false	"new name of the user"
//	@Success		200		{object}	internal.User
//	@Failure		400		{object}	ResponseError
//	@Failure		403		{object}	ResponseError
//	@Failure		500		{object}	ResponseError
//	@Router			/users/{id} [put]
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

	writeJSON(UpdateUserResponse{User: u}, http.StatusOK, w)
}

// deleteUserHandler godoc
//
//	@Summary		Delete User
//	@Description	deletes the user
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int	true	"id of the user"
//	@Success		200	{object}	ResponseMessage
//	@Failure		403	{object}	ResponseError
//	@Failure		500	{object}	ResponseError
//	@Router			/users/{id} [delete]
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
	writeJSON(ResponseMessage{Message: "user delete successfully"}, http.StatusOK, w)
}
