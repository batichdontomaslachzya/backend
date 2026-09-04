package main

import (
	"context"
	"errors"
	"image-service/internal/jobs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		client := &http.Client{Timeout: 2 * time.Second}
		response, err := client.Get("http://127.0.0.1:8000/healthz")
		if err != nil {
			os.Exit(1)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8000"
	}

	broker, dataDir, internalToken := os.Getenv("KAFKA_BROKER"), os.Getenv("DATA_DIR"), os.Getenv("INTERNAL_TOKEN")
	if broker == "" {
		broker = "localhost:9092"
	}
	if dataDir == "" {
		dataDir = "data"
	}
	if len(internalToken) < 32 {
		log.Fatal("INTERNAL_TOKEN must contain at least 32 characters")
	}
	for _, subdir := range []string{"input", "result"} {
		if err := os.MkdirAll(filepath.Join(dataDir, subdir), 0700); err != nil {
			log.Fatal(err)
		}
	}
	databaseURL, redisAddr := os.Getenv("DATABASE_URL"), os.Getenv("REDIS_ADDR")
	if databaseURL == "" || redisAddr == "" {
		log.Fatal("DATABASE_URL and REDIS_ADDR are required")
	}
	startupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		log.Fatal("invalid DATABASE_URL")
	}
	config.MaxConns = 10
	config.ConnConfig.ConnectTimeout = 5 * time.Second
	db, err := pgxpool.NewWithConfig(startupCtx, config)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer db.Close()
	if err = db.Ping(startupCtx); err != nil {
		log.Fatalf("postgres: %v", err)
	}
	if err = migrate(startupCtx, db); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	cache := redis.NewClient(&redis.Options{
		Addr: redisAddr, Password: os.Getenv("REDIS_PASSWORD"), Protocol: 2,
		DialTimeout: 2 * time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second,
		ContextTimeoutEnabled: true,
	})
	defer cache.Close()
	if err = cache.Ping(startupCtx).Err(); err != nil {
		log.Fatalf("redis: %v", err)
	}
	err = jobs.EnsureTopic(startupCtx, broker)
	cancel()
	if err != nil {
		log.Fatalf("create Kafka topic: %v", err)
	}
	writer := jobs.NewWriter(broker)
	defer writer.Close()
	store := &postgresStore{db: db}
	tasks := &taskService{store: store, writer: writer, dataDir: dataDir, wake: make(chan struct{}, 1)}
	auth := &authService{users: store, sessions: &redisSessions{client: cache}}
	dispatched := make(chan struct{})
	go func() { defer close(dispatched); tasks.dispatch(ctx) }()
	server := &http.Server{
		Addr:              addr,
		Handler:           newHandler(tasks, auth, internalToken),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
		close(done)
	}()

	log.Printf("listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-done
	<-dispatched
}
