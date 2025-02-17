package main

import (
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/base32"
	"fmt"
	"html/template"
	"time"

	"github.com/shopspring/decimal"
)

//go:embed templates
var Templates embed.FS
var ActivateUserTmpl *template.Template
var ResetPasswordTempl *template.Template

func init() {
	var err error
	ActivateUserTmpl, err = template.ParseFS(Templates, "templates/activate_user.gotmpl")
	if err != nil {
		panic(err)
	}
	ResetPasswordTempl, err = template.ParseFS(Templates, "templates/reset_password.gotmpl")
	if err != nil {
		panic(err)
	}
}

type User struct {
	ID           int64     `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	PasswordHash []byte    `json:"-"`
	IsActivated  bool      `json:"is_activated"`
	Version      int32     `json:"-"`
}

type TokenScope int16

const (
	TokenScopeActivation TokenScope = iota
	TokenScopeAuthentication
	TokenScopePasswordReset
)

func (s TokenScope) String() string {
	switch s {
	case TokenScopeActivation:
		return "Activation"
	case TokenScopeAuthentication:
		return "Authentication"
	case TokenScopePasswordReset:
		return "PasswordReset"
	}
	return fmt.Sprintf("TokenScope %d", s)
}

type Token struct {
	ID        int64      `json:"-"`
	UserID    int64      `json:"user_id"`
	Scope     TokenScope `json:"scope"`
	Hash      []byte     `json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
}

func generateToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	token := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
	return token
}

func hashToken(token string) []byte {
	hash := sha256.Sum256([]byte(token))
	return hash[:]
}

type Permission string

type MetaData struct {
	CurrentPage  int `json:"current_page,omitempty"`
	PageSize     int `json:"page_size,omitempty"`
	FirstPage    int `json:"first_page,omitempty"`
	LastPage     int `json:"last_page,omitempty"`
	TotalRecords int `json:"total_records,omitempty"`
}

type Movie struct {
	ID        int64    `json:"id"`
	CreatedAt string   `json:"created_at"`
	Title     string   `json:"title"`
	Runtime   int32    `json:"runtime"`
	Year      int32    `json:"year"`
	Genres    []string `json:"genres"`
	Version   int32    `json:"version"`
}

type Cinema struct {
	ID       int32  `json:"id"`
	Name     string `json:"name"`
	Location string `json:"location"`
	OwnerID  int64  `json:"ower_id"`
	Version  int32  `json:"version"`
}

type Hall struct {
	ID                 int32           `json:"id"`
	Name               string          `json:"name"`
	CinemaID           int32           `json:"cinema_id"`
	SeatingArrangement string          `json:"seating_arrangement"`
	SeatPrice          decimal.Decimal `json:"seat_price"`
	Version            int32           `json:"version"`
}

type Seat struct {
	ID       int32  `json:"id"`
	Location string `json:"location"`
	HallID   int32  `json:"hall_id"`
	Version  int32  `json:"version"`
}
