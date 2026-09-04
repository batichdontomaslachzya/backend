package jobs

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"regexp"
)

const (
	Topic         = "image-tasks"
	MaxImageBytes = 10 << 20
	MaxPixels     = 8_000_000
	MaxDimension  = 4096
)

var idPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type Filter struct {
	Name       string `json:"name"`
	Parameters struct {
		Sigma float64 `json:"sigma,omitempty"`
	} `json:"parameters,omitempty"`
}

type Message struct {
	TaskID string `json:"task_id"`
	Filter Filter `json:"filter"`
}

type Completion struct {
	TaskID string `json:"task_id"`
	Error  string `json:"error,omitempty"`
}

func ValidID(id string) bool { return idPattern.MatchString(id) }

func (f Filter) Validate() error {
	sigma := f.Parameters.Sigma
	switch f.Name {
	case "negative", "flip_x":
		if sigma != 0 {
			return errors.New("this filter does not accept sigma")
		}
	case "blur", "sharpen":
		if math.IsNaN(sigma) || math.IsInf(sigma, 0) || sigma <= 0 || sigma > 10 {
			return errors.New("sigma must be greater than 0 and at most 10")
		}
	default:
		return errors.New("filter must be negative, flip_x, blur or sharpen")
	}
	return nil
}

func ValidSize(width, height int) bool {
	return width > 0 && height > 0 && width <= MaxDimension && height <= MaxDimension && width*height <= MaxPixels
}

func InputPath(dir, id string) string  { return filepath.Join(dir, "input", id+".image") }
func ResultPath(dir, id string) string { return filepath.Join(dir, "result", id+".png") }

// Сначала пишем временный файл, затем одним rename публикуем полный результат.
func WriteFile(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".image-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
