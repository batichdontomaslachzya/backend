package main

import (
	"bytes"
	"errors"
	"image"
	"image-service/internal/jobs"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"

	"github.com/disintegration/imaging"
)

func processImage(dir string, job jobs.Message) error {
	if !jobs.ValidID(job.TaskID) {
		return errors.New("invalid task ID")
	}
	if err := job.Filter.Validate(); err != nil {
		return err
	}
	file, err := os.Open(jobs.InputPath(dir, job.TaskID))
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, jobs.MaxImageBytes+1))
	if err != nil {
		return err
	}
	if len(data) > jobs.MaxImageBytes {
		return errors.New("input too large")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return err
	}
	if (format != "png" && format != "jpeg") || !jobs.ValidSize(config.Width, config.Height) {
		return errors.New("unsupported image format or dimensions")
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	result := applyFilter(src, job.Filter)
	var output bytes.Buffer
	if err = png.Encode(&output, result); err != nil {
		return err
	}
	return jobs.WriteFile(jobs.ResultPath(dir, job.TaskID), output.Bytes())
}

func applyFilter(src image.Image, filter jobs.Filter) *image.NRGBA {
	switch filter.Name {
	case "blur":
		return imaging.Blur(src, filter.Parameters.Sigma)
	case "sharpen":
		return imaging.Sharpen(src, filter.Parameters.Sigma)
	case "negative":
		dst := imaging.Clone(src)
		for i := 0; i < len(dst.Pix); i += 4 {
			dst.Pix[i] = 255 - dst.Pix[i]
			dst.Pix[i+1] = 255 - dst.Pix[i+1]
			dst.Pix[i+2] = 255 - dst.Pix[i+2]
			// Alpha не инвертируем.
		}
		return dst
	default: // flip_x: отражение относительно горизонтальной оси, верх <-> низ.
		input := imaging.Clone(src)
		dst := image.NewNRGBA(input.Bounds())
		for y := 0; y < dst.Bounds().Dy(); y++ {
			srcStart := (dst.Bounds().Dy() - 1 - y) * input.Stride
			copy(dst.Pix[y*dst.Stride:(y+1)*dst.Stride], input.Pix[srcStart:srcStart+input.Stride])
		}
		return dst
	}
}
