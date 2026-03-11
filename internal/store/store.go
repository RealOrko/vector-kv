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
	Chunk    int     `json:"chunk"`
	Metadata *string `json:"metadata"`
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

func (s *Store) Insert(ctx context.Context, key, content, metadata string, chunk int, embedding []float32) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO entries (key, content, embedding, metadata, chunk) VALUES ($1, $2, $3, $4, $5)",
		key, content, pgvector.NewVector(embedding), nilIfEmpty(metadata), chunk)
	return err
}

func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func (s *Store) Query(ctx context.Context, key string, embedding []float32, metadata string, limit int) ([]Result, error) {
	var rows *sql.Rows
	var err error
	if metadata == "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT content, embedding <=> $1 AS distance, chunk, metadata
			 FROM entries WHERE key = $2
			 ORDER BY chunk, embedding <=> $1 LIMIT $3`,
			pgvector.NewVector(embedding), key, limit)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT content, embedding <=> $1 AS distance, chunk, metadata
			 FROM entries WHERE key = $2 AND metadata ILIKE '%' || $4 || '%'
			 ORDER BY chunk, embedding <=> $1 LIMIT $3`,
			pgvector.NewVector(embedding), key, limit, metadata)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Result
	for rows.Next() {
		var r Result
		if err := rows.Scan(&r.Content, &r.Distance, &r.Chunk, &r.Metadata); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *Store) QueryByMetadata(ctx context.Context, key, metadata string, limit int) ([]Result, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT content, 0 AS distance, chunk, metadata
		 FROM entries WHERE key = $1 AND metadata ILIKE '%' || $2 || '%'
		 ORDER BY metadata, chunk LIMIT $3`,
		key, metadata, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Result
	for rows.Next() {
		var r Result
		if err := rows.Scan(&r.Content, &r.Distance, &r.Chunk, &r.Metadata); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *Store) Keys(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT DISTINCT key FROM entries ORDER BY key")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM entries WHERE key = $1", key)
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}
