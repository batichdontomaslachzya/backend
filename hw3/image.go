package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image-service/internal/jobs"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
)

var errImageTooLarge = errors.New("image exceeds 10 MiB")

func readUpload(w http.ResponseWriter, r *http.Request) (jobs.Filter, []byte, string, error) {
	var filter jobs.Filter
	r.Body = http.MaxBytesReader(w, r.Body, jobs.MaxImageBytes+(1<<20))
	err := r.ParseMultipartForm(1 << 20)
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if err != nil {
		return filter, nil, "", err
	}
	form := r.MultipartForm
	if len(form.File) != 1 || len(form.File["image"]) != 1 || len(form.Value) != 1 || len(form.Value["filter"]) != 1 {
		return filter, nil, "", errors.New("provide one image file and one filter JSON field")
	}
	decoder := json.NewDecoder(bytes.NewBufferString(form.Value["filter"][0]))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&filter); err != nil {
		return filter, nil, "", errors.New("invalid filter JSON")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return filter, nil, "", errors.New("provide one filter JSON object")
	}
	if err = filter.Validate(); err != nil {
		return filter, nil, "", err
	}
	header := form.File["image"][0]
	if header.Size > jobs.MaxImageBytes {
		return filter, nil, "", errImageTooLarge
	}
	file, err := header.Open()
	if err != nil {
		return filter, nil, "", errors.New("could not read upload")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, jobs.MaxImageBytes+1))
	if err != nil {
		return filter, nil, "", errors.New("could not read upload")
	}
	if len(data) > jobs.MaxImageBytes {
		return filter, nil, "", errImageTooLarge
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg") {
		return filter, nil, "", errors.New("image must be PNG or JPEG")
	}
	if !jobs.ValidSize(config.Width, config.Height) {
		return filter, nil, "", errors.New("image must fit 4096x4096 and 8 million pixels")
	}
	return filter, data, "image/" + format, nil
}

func (a *api) originalImage(w http.ResponseWriter, r *http.Request, userID string) {
	t, ok := a.loadTask(w, r, userID)
	if !ok {
		return
	}
	serveImage(w, r, jobs.InputPath(a.tasks.dataDir, t.ID), t.MediaType)
}

func serveImage(w http.ResponseWriter, r *http.Request, path, mediaType string) {
	file, err := os.Open(path)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "image file unavailable"})
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "image file unavailable"})
		return
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}
