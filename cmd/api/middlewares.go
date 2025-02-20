package main

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
)

type userRequestContextKey string

const UserRequestContextKey userRequestContextKey = "UserContextKey"

func getUserFromRequestContext(r *http.Request) *User {
	return r.Context().Value(UserRequestContextKey).(*User)
}

func (app *Application) authenticate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Authorization")
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeError(errors.New("invalid Authorization header"), http.StatusUnauthorized, w)
			return
		}
		parts := strings.Fields(authHeader)
		if len(parts) != 2 || parts[0] != "Bearer" {
			writeError(errors.New("invalid Authorization header"), http.StatusUnauthorized, w)
			return
		}
		token := parts[1]
		u, err := app.storage.GetUserFromToken(TokenScopeAuthentication, token)
		if err != nil {
			writeServerErr(err, w)
			return
		}
		if u == nil {
			writeError(errors.New("invalid token"), http.StatusUnauthorized, w)
			return
		}

		ctx := context.WithValue(r.Context(), UserRequestContextKey, u)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	}
}

func (app *Application) authorize(permissions []Permission, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := getUserFromRequestContext(r)
		if u == nil {
			writeServerErr(errors.New("user is not authenticated"), w)
			return
		}
		has, err := app.storage.GetPermissions(u.ID)
		if err != nil {
			writeServerErr(err, w)
			return
		}
		for _, p := range permissions {
			if !slices.Contains(has, p) {
				writeForbidden(w)
				return
			}
		}
		next.ServeHTTP(w, r)
	}
}

func (app *Application) requireUserActivation(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := getUserFromRequestContext(r)
		if u == nil {
			writeServerErr(errors.New("user is not authenticated"), w)
			return
		}
		if !u.IsActivated {
			writeForbidden(w)
			return
		}
		next.ServeHTTP(w, r)
	}
}
