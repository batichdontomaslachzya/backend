package jobs

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"
)

func EnsureTopic(ctx context.Context, broker string) error {
	conn, err := kafka.DialContext(ctx, "tcp", broker)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	controller, err := conn.Controller()
	if err != nil {
		return err
	}
	admin, err := kafka.DialContext(ctx, "tcp", net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		return err
	}
	defer admin.Close()
	admin.SetDeadline(time.Now().Add(10 * time.Second))
	err = admin.CreateTopics(kafka.TopicConfig{Topic: Topic, NumPartitions: 1, ReplicationFactor: 1})
	if errors.Is(err, kafka.TopicAlreadyExists) {
		return nil
	}
	return err
}

func NewWriter(broker string) *kafka.Writer {
	return &kafka.Writer{
		Addr: kafka.TCP(broker), Topic: Topic, RequiredAcks: kafka.RequireAll,
		BatchSize: 1, MaxAttempts: 3, WriteTimeout: 5 * time.Second,
		ReadTimeout: 5 * time.Second,
	}
}
