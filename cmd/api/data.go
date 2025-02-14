package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"time"
)

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

func (s TokenScope) String() string {
	switch s {
	case 0:
		return "Activation"
	case 1:
		return "Authentication"
	case 2:
		return "PasswordReset"
	}
	return fmt.Sprintf("TokenScope %d", s)
}

const (
	TokenScopeActivation TokenScope = iota
	TokenScopeAuthentication
	TokenScopePasswordReset
)

type Token struct {
	ID        int64      `json:"-"`
	UserID    int64      `json:"user_id"`
	Scope     TokenScope `json:"scope"`
	Hash      []byte     `json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
}

func createToken() ([]byte, string) {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	token := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
	hash := sha256.Sum256([]byte(token))
	return hash[:], token
}
