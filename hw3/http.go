package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"image-service/internal/jobs"
	"net/http"
)

//go:embed docs/openapi.yaml
var openAPI []byte

type api struct {
	tasks         *taskService
	auth          *authService
	internalToken string
}

func newHandler(tasks *taskService, auth *authService, internalToken string) http.Handler {
	a := &api{tasks: tasks, auth: auth, internalToken: internalToken}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /register", a.register)
	mux.HandleFunc("POST /login", a.login)
	mux.HandleFunc("POST /task", a.requireAuth(a.createTask))
	mux.HandleFunc("GET /status/{task_id}", a.requireAuth(a.taskStatus))
	mux.HandleFunc("GET /result/{task_id}", a.requireAuth(a.taskResult))
	mux.HandleFunc("GET /image/{task_id}", a.requireAuth(a.originalImage))
	mux.HandleFunc("POST /commit", a.commit)
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.Write(openAPI)
	})
	return observeHTTP(mux)
}

func (a *api) createTask(w http.ResponseWriter, r *http.Request, userID string) {
	filter, data, mediaType, err := readUpload(w, r)
	if err != nil {
		status := http.StatusBadRequest
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) || errors.Is(err, errImageTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	t, err := a.tasks.create(r.Context(), userID, mediaType, filter, data)
	if err != nil {
		storageUnavailable(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"task_id": t.ID})
}

func (a *api) taskStatus(w http.ResponseWriter, r *http.Request, userID string) {
	t, ok := a.loadTask(w, r, userID)
	if !ok {
		return
	}
	response := map[string]string{"status": t.Status}
	if t.Error != "" {
		response["error"] = t.Error
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *api) taskResult(w http.ResponseWriter, r *http.Request, userID string) {
	t, ok := a.loadTask(w, r, userID)
	if !ok {
		return
	}
	if t.Status == statusFailed {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": t.Error})
		return
	}
	if t.Status != statusReady {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": t.Status})
		return
	}
	serveImage(w, r, jobs.ResultPath(a.tasks.dataDir, t.ID), "image/png")
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (a *api) loadTask(w http.ResponseWriter, r *http.Request, userID string) (task, bool) {
	t, err := a.tasks.get(r.Context(), r.PathValue("task_id"), userID)
	if errors.Is(err, errTaskNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return task{}, false
	}
	if err != nil {
		storageUnavailable(w, err)
		return task{}, false
	}
	return t, true
}
