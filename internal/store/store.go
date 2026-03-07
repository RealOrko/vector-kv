package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
	pgvector "github.com/pgvector/pgvector-go"
)

type Store struct {
	db *sql.DB
}

type Result struct {
	Content  string  `json:"content"`
	Distance float64 `json:"distance"`
}

func New(databaseURL string) (*Store, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Insert(ctx context.Context, key, content string, embedding []float32) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO entries (key, content, embedding) VALUES ($1, $2, $3)",
		key, content, pgvector.NewVector(embedding))
	return err
}

func (s *Store) Query(ctx context.Context, key string, embedding []float32, limit int) ([]Result, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT content, embedding <=> $1 AS distance
		 FROM entries WHERE key = $2
		 ORDER BY embedding <=> $1 LIMIT $3`,
		pgvector.NewVector(embedding), key, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Result
	for rows.Next() {
		var r Result
		if err := rows.Scan(&r.Content, &r.Distance); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM entries WHERE key = $1", key)
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}
