package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"image-service/internal/jobs"
	"image/png"
	"io"
	"net/http"
	"os"
	"strings"
)

func (a *api) commit(w http.ResponseWriter, r *http.Request) {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(r.Header.Values("Authorization")) != 1 || len(parts) != 2 ||
		!strings.EqualFold(parts[0], "Bearer") || subtle.ConstantTimeCompare([]byte(parts[1]), []byte(a.internalToken)) != 1 {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "processor credentials required"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var completion jobs.Completion
	if err := decoder.Decode(&completion); err != nil || !jobs.ValidID(completion.TaskID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid completion"})
		return
	}
	if decoder.Decode(new(any)) != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provide one completion"})
		return
	}
	t, err := a.tasks.store.get(r.Context(), completion.TaskID)
	if errors.Is(err, errTaskNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	if err != nil {
		storageUnavailable(w, err)
		return
	}
	// Kafka может доставить сообщение повторно. Готовую задачу не меняем.
	if t.Status != statusInProgress {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	failure := ""
	if completion.Error != "" {
		failure = "image processing failed"
	} else if !validResult(jobs.ResultPath(a.tasks.dataDir, t.ID)) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "result PNG unavailable or invalid"})
		return
	}
	if err = a.tasks.store.finish(r.Context(), t.ID, failure); err != nil {
		storageUnavailable(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validResult(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() > 40<<20 {
		return false
	}
	config, err := png.DecodeConfig(file)
	return err == nil && jobs.ValidSize(config.Width, config.Height)
}
