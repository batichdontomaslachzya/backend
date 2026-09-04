package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

var errSessionNotFound = errors.New("session not found")

type sessionStore interface {
	saveSession(context.Context, string, string) error
	sessionUser(context.Context, string) (string, error)
}

type redisSessions struct{ client *redis.Client }

func sessionKey(token string) string {
	hash := sha256.Sum256([]byte(token))
	return "session:" + hex.EncodeToString(hash[:])
}

func (s *redisSessions) saveSession(ctx context.Context, token, userID string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return s.client.Set(ctx, sessionKey(token), userID, sessionTTL).Err()
}

func (s *redisSessions) sessionUser(ctx context.Context, token string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	userID, err := s.client.Get(ctx, sessionKey(token)).Result()
	if errors.Is(err, redis.Nil) {
		return "", errSessionNotFound
	}
	return userID, err
}
