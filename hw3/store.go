package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image-service/internal/jobs"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errTaskNotFound = errors.New("task not found")

type postgresStore struct{ db *pgxpool.Pool }

func (s *postgresStore) save(ctx context.Context, t task) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	filter, err := json.Marshal(t.Filter)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO tasks(id,user_id,status,media_type,filter,input_path,result_path)
 VALUES($1,$2,$3,$4,$5,$6,$7)`, t.ID, t.UserID, t.Status, t.MediaType, filter,
		"input/"+t.ID+".image", "result/"+t.ID+".png")
	return err
}

func (s *postgresStore) get(ctx context.Context, id string) (task, error) {
	if !jobs.ValidID(id) {
		return task{}, errTaskNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var t task
	var filter []byte
	err := s.db.QueryRow(ctx, `SELECT id::text,user_id::text,status,media_type,error,filter
 FROM tasks WHERE id=$1`, id).Scan(&t.ID, &t.UserID, &t.Status, &t.MediaType, &t.Error, &filter)
	if errors.Is(err, pgx.ErrNoRows) {
		return task{}, errTaskNotFound
	}
	if err != nil {
		return task{}, err
	}
	if err = json.Unmarshal(filter, &t.Filter); err != nil {
		return task{}, err
	}
	return t, nil
}

func (s *postgresStore) finish(ctx context.Context, id, failure string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	status := statusReady
	if failure != "" {
		status = statusFailed
	}
	// Условие в UPDATE делает повторный callback безопасным и без mutex.
	_, err := s.db.Exec(ctx, `UPDATE tasks SET status=$2,error=$3,completed_at=now()
 WHERE id=$1 AND status='in_progress'`, id, status, failure)
	return err
}

func (s *postgresStore) createUser(ctx context.Context, u user) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := s.db.Exec(ctx, `INSERT INTO users(id,username,salt,password_hash)
 VALUES($1,$2,$3,$4) ON CONFLICT(username) DO NOTHING`, u.ID, u.Username, u.Salt[:], u.PasswordHash[:])
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return errUsernameTaken
	}
	return nil
}

func (s *postgresStore) userByName(ctx context.Context, name string) (user, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var u user
	var salt, hash []byte
	err := s.db.QueryRow(ctx, `SELECT id::text,username,salt,password_hash FROM users WHERE username=$1`, name).
		Scan(&u.ID, &u.Username, &salt, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return user{}, false, nil
	}
	if err != nil {
		return user{}, false, err
	}
	if len(salt) != 16 || len(hash) != 32 {
		return user{}, false, fmt.Errorf("invalid stored password hash")
	}
	copy(u.Salt[:], salt)
	copy(u.PasswordHash[:], hash)
	return u, true, nil
}

func (s *postgresStore) pending(ctx context.Context) ([]jobs.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := s.db.Query(ctx, `SELECT id::text,filter FROM tasks
 WHERE published_at IS NULL AND status='in_progress' ORDER BY created_at LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []jobs.Message
	for rows.Next() {
		var msg jobs.Message
		var filter []byte
		if err = rows.Scan(&msg.TaskID, &filter); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(filter, &msg.Filter); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

func (s *postgresStore) markPublished(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.db.Exec(ctx, "UPDATE tasks SET published_at=now() WHERE id=$1 AND published_at IS NULL", id)
	return err
}
