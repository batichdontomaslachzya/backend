package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"image-service/internal/jobs"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

const (
	statusInProgress = "in_progress"
	statusReady      = "ready"
	statusFailed     = "failed"
)

type task struct {
	ID        string
	UserID    string
	Status    string
	MediaType string
	Error     string
	Filter    jobs.Filter
}

type taskStore interface {
	save(context.Context, task) error
	get(context.Context, string) (task, error)
	finish(context.Context, string, string) error
	pending(context.Context) ([]jobs.Message, error)
	markPublished(context.Context, string) error
}

type taskService struct {
	store   taskStore
	writer  *kafka.Writer
	dataDir string
	wake    chan struct{}
}

func (s *taskService) create(ctx context.Context, userID, mediaType string, filter jobs.Filter, image []byte) (task, error) {
	t := task{ID: newID(), UserID: userID, Status: statusInProgress, MediaType: mediaType, Filter: filter}
	if err := jobs.WriteFile(jobs.InputPath(s.dataDir, t.ID), image); err != nil {
		return task{}, err
	}
	// INSERT одновременно сохраняет задачу и намерение опубликовать её в Kafka.
	if err := s.store.save(ctx, t); err != nil {
		return task{}, err
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return t, nil
}

func (s *taskService) get(ctx context.Context, id, userID string) (task, error) {
	t, err := s.store.get(ctx, id)
	if err != nil {
		return task{}, err
	}
	if t.UserID != userID {
		return task{}, errTaskNotFound
	}
	return t, nil
}

// Один диспетчер на API. Повторы после сбоя допустимы и безопасны для callback.
func (s *taskService) dispatch(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		messages, err := s.store.pending(ctx)
		if err != nil && ctx.Err() == nil {
			log.Printf("outbox read: %v", err)
		}
		for _, msg := range messages {
			data, err := json.Marshal(msg)
			if err != nil {
				log.Printf("outbox encode: %v", err)
				break
			}
			publishCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err = s.writer.WriteMessages(publishCtx, kafka.Message{Key: []byte(msg.TaskID), Value: data})
			cancel()
			if err != nil {
				if ctx.Err() == nil {
					log.Printf("outbox publish: %v", err)
				}
				break
			}
			if err = s.store.markPublished(ctx, msg.TaskID); err != nil {
				if ctx.Err() == nil {
					log.Printf("outbox mark: %v", err)
				}
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.wake:
		}
	}
}

func newID() string {
	var id [16]byte
	rand.Read(id[:])
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
}
