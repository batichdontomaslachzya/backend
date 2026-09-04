package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"strings"
)

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (a *api) register(w http.ResponseWriter, r *http.Request) {
	c, ok := readCredentials(w, r)
	if !ok {
		return
	}
	u, err := a.auth.register(r.Context(), c.Username, c.Password)
	if errors.Is(err, errUsernameTaken) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	if err != nil {
		storageUnavailable(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"user_id": u.ID, "username": u.Username})
}

func (a *api) login(w http.ResponseWriter, r *http.Request) {
	c, ok := readCredentials(w, r)
	if !ok {
		return
	}
	token, err := a.auth.login(r.Context(), c.Username, c.Password)
	if errors.Is(err, errInvalidCredentials) {
		unauthorized(w)
		return
	}
	if err != nil {
		storageUnavailable(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

func (a *api) requireAuth(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(r.Header.Values("Authorization")) != 1 || len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			unauthorized(w)
			return
		}
		userID, err := a.auth.authenticate(r.Context(), parts[1])
		if errors.Is(err, errSessionNotFound) {
			unauthorized(w)
			return
		}
		if err != nil {
			storageUnavailable(w, err)
			return
		}
		next(w, r, userID)
	}
}

func storageUnavailable(w http.ResponseWriter, err error) {
	log.Printf("storage: %v", err)
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "storage temporarily unavailable"})
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
}

func readCredentials(w http.ResponseWriter, r *http.Request) (credentials, bool) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "Content-Type must be application/json"})
		return credentials{}, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var c credentials
	err = decoder.Decode(&c)
	if err == nil {
		if tailErr := decoder.Decode(new(any)); tailErr != io.EOF {
			err = tailErr
			if err == nil {
				err = errors.New("multiple JSON values")
			}
		}
	}
	if err != nil {
		status := http.StatusBadRequest
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, status, map[string]string{"error": "invalid JSON body (maximum 4096 bytes)"})
		return credentials{}, false
	}
	if !validCredentials(c.Username, c.Password) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "username: 3-32 letters, digits or underscores; password: 8-128 bytes",
		})
		return credentials{}, false
	}
	return c, true
}
