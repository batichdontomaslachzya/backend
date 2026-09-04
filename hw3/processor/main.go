package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image-service/internal/jobs"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/segmentio/kafka-go"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	stopMetrics, err := startMetrics()
	if err != nil {
		return err
	}
	defer stopMetrics()
	broker, dir := os.Getenv("KAFKA_BROKER"), os.Getenv("DATA_DIR")
	apiURL, token := os.Getenv("API_URL"), os.Getenv("INTERNAL_TOKEN")
	if broker == "" {
		broker = "localhost:9092"
	}
	if dir == "" {
		dir = "data"
	}
	if apiURL == "" {
		apiURL = "http://localhost:8000"
	}
	if len(token) < 32 {
		return errors.New("INTERNAL_TOKEN must contain at least 32 characters")
	}
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{broker}, Topic: jobs.Topic, GroupID: "image-processor",
		MinBytes: 1, MaxBytes: 1 << 20, MaxWait: time.Second,
		CommitInterval: 0, StartOffset: kafka.FirstOffset,
	})
	defer reader.Close()
	client := &http.Client{Timeout: 10 * time.Second}
	log.Print("processor waiting for image tasks")
	for ctx.Err() == nil {
		message, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("fetch: %v", err)
			pause(ctx)
			continue
		}
		var job jobs.Message
		if err = json.Unmarshal(message.Value, &job); err != nil || !jobs.ValidID(job.TaskID) {
			log.Printf("skip malformed message at offset %d", message.Offset)
		} else {
			completion := jobs.Completion{TaskID: job.TaskID}
			started := time.Now()
			err = processImage(dir, job)
			recordProcessing(job.Filter.Name, started, err)
			if err != nil {
				log.Printf("task %s: %v", job.TaskID, err)
				completion.Error = "image processing failed"
			}
			if err = notify(ctx, client, strings.TrimRight(apiURL, "/")+"/commit", token, completion); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err // Не подтверждаем сообщение при неверной конфигурации callback.
			}
		}
		// Offset подтверждается только после ответа API, а не сразу при чтении.
		for ctx.Err() == nil {
			if err = reader.CommitMessages(ctx, message); err == nil {
				break
			}
			log.Printf("commit offset: %v", err)
			pause(ctx)
		}
	}
	return nil
}

func notify(ctx context.Context, client *http.Client, url, token string, completion jobs.Completion) error {
	body, err := json.Marshal(completion)
	if err != nil {
		return err
	}
	for ctx.Err() == nil {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		response, err := client.Do(req)
		if err == nil {
			io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
			response.Body.Close()
			if response.StatusCode == http.StatusNoContent {
				return nil
			}
			if response.StatusCode == http.StatusNotFound {
				log.Printf("task %s is unknown to API; skip", completion.TaskID)
				return nil
			}
			if response.StatusCode >= 400 && response.StatusCode < 500 {
				return fmt.Errorf("callback rejected task %s: HTTP %d", completion.TaskID, response.StatusCode)
			}
			log.Printf("callback temporarily unavailable: HTTP %d", response.StatusCode)
		} else if ctx.Err() == nil {
			log.Printf("callback: %v", err)
		}
		pause(ctx)
	}
	return ctx.Err()
}

func pause(ctx context.Context) {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
