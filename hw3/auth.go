package main

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"regexp"
	"time"
)

const (
	passwordIterations = 600_000
	sessionTTL         = 24 * time.Hour
)

var (
	errUsernameTaken      = errors.New("username already exists")
	errInvalidCredentials = errors.New("invalid username or password")
	usernamePattern       = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)
)

type user struct {
	ID           string
	Username     string
	Salt         [16]byte
	PasswordHash [32]byte
}

type userStore interface {
	createUser(context.Context, user) error
	userByName(context.Context, string) (user, bool, error)
}

type authService struct {
	users    userStore
	sessions sessionStore
}

func validCredentials(username, password string) bool {
	return usernamePattern.MatchString(username) && len(password) >= 8 && len(password) <= 128
}

func (s *authService) register(ctx context.Context, username, password string) (user, error) {
	u := user{ID: newID(), Username: username}
	rand.Read(u.Salt[:])
	hash, err := hashPassword(password, u.Salt)
	if err != nil {
		return user{}, err
	}
	u.PasswordHash = hash
	if err = s.users.createUser(ctx, u); err != nil {
		return user{}, err
	}
	return u, nil
}

func (s *authService) login(ctx context.Context, username, password string) (string, error) {
	u, ok, err := s.users.userByName(ctx, username)
	if err != nil {
		return "", err
	}
	// Считаем хеш и для неизвестного логина, чтобы не делать быстрый отказ.
	hash, err := hashPassword(password, u.Salt)
	if err != nil {
		return "", err
	}
	match := subtle.ConstantTimeCompare(hash[:], u.PasswordHash[:])
	if !ok || match != 1 {
		return "", errInvalidCredentials
	}

	var raw [32]byte
	rand.Read(raw[:])
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	if err = s.sessions.saveSession(ctx, token, u.ID); err != nil {
		return "", err
	}
	return token, nil
}

func (s *authService) authenticate(ctx context.Context, token string) (string, error) {
	return s.sessions.sessionUser(ctx, token)
}

func hashPassword(password string, salt [16]byte) ([32]byte, error) {
	key, err := pbkdf2.Key(sha256.New, password, salt[:], passwordIterations, 32)
	if err != nil {
		return [32]byte{}, err
	}
	return [32]byte(key), nil
}
