package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/AdventurerAmer/movie-reservation-system/internal"
	"golang.org/x/time/rate"
)

type userRequestContextKey string

const UserRequestContextKey userRequestContextKey = "UserContextKey"

func getUserFromRequestContext(r *http.Request) *internal.User {
	return r.Context().Value(UserRequestContextKey).(*internal.User)
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
		u, err := app.storage.Tokens.GetUser(internal.TokenScopeAuthentication, token)
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

func (app *Application) authorize(permissions []internal.Permission, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := getUserFromRequestContext(r)
		if u == nil {
			writeServerErr(errors.New("user is not authenticated"), w)
			return
		}
		has, err := app.storage.Permissions.Get(u.ID)
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

func (app *Application) rateLimit(next http.Handler) http.HandlerFunc {
	type client struct {
		limiter          *rate.Limiter
		lastRequestWasAt time.Time
	}
	var (
		mu      sync.RWMutex
		clients = make(map[string]client)
	)
	app.StartService(func() {
		ticker := time.NewTicker(time.Minute)
	loop:
		for {
			select {
			case <-ticker.C:
				func() {
					mu.Lock()
					defer mu.Unlock()
					for ip, c := range clients {
						if time.Since(c.lastRequestWasAt) >= time.Minute*3 {
							delete(clients, ip)
						}
					}
				}()
			case _, open := <-app.quit:
				if !open {
					break loop
				}
			}
		}
	})
	return func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			writeServerErr(err, w)
			return
		}

		exceeded := func() bool {
			mu.Lock()
			defer mu.Unlock()
			c, ok := clients[ip]
			if !ok {
				c = client{
					limiter: rate.NewLimiter(rate.Limit(app.config.limiter.maxRequestPerSecond), app.config.limiter.burst),
				}
			}
			c.lastRequestWasAt = time.Now()
			clients[ip] = c
			return !c.limiter.Allow()
		}()

		if exceeded {
			res := map[string]any{
				"message": "rate limit exceeded",
			}
			writeJSON(res, http.StatusTooManyRequests, w)
			return
		}

		next.ServeHTTP(w, r)
	}
}

func (app *Application) enableCORS(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		w.Header().Add("Vary", "Access-Control-Request-Method")

		origin := w.Header().Get("Origin")
		if origin != "" {
			for _, o := range app.config.cors.trustedOrigins {
				if origin == o || o == "*" {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					// preflight request
					if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
						w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, PUT, PATCH, DELETE")
						w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
						w.WriteHeader(http.StatusOK)
						return
					}
					break
				}
			}
		}
		next.ServeHTTP(w, r)
	}
}

func (app *Application) recoverFromPanic(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				w.Header().Set("Connection", "close")
				log.Println("Recovered from panic:", err)
				res := map[string]any{
					"error": "internal server error",
				}
				writeJSON(res, http.StatusInternalServerError, w)
			}
		}()
		next.ServeHTTP(w, r)
	}
}
