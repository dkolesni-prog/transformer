package store

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/dkolesni-prog/transformer/internal/app"
	"github.com/dkolesni-prog/transformer/internal/app/endpoints"
	"github.com/dkolesni-prog/transformer/internal/app/middleware"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RDB struct {
	pool *pgxpool.Pool
}

func NewRDB(ctx context.Context, dsn string) (*RDB, error) {
	cfg, parseErr := pgxpool.ParseConfig(dsn)
	if parseErr != nil {
		middleware.Log.Printf("parse DSN error: " + parseErr.Error())
		return nil, errors.New("parse DSN error: " + parseErr.Error())
	}

	pool, poolErr := pgxpool.NewWithConfig(ctx, cfg)
	if poolErr != nil {
		middleware.Log.Printf("cannot create pgxpool: " + poolErr.Error())
		return nil, errors.New("cannot create pgxpool: " + poolErr.Error())
	}

	pingErr := pool.Ping(ctx)
	if pingErr != nil {
		pool.Close()
		middleware.Log.Printf("failed ping: " + pingErr.Error())
		return nil, errors.New("failed ping: " + pingErr.Error())
	}

	return &RDB{pool: pool}, nil
}

func (r *RDB) Bootstrap(ctx context.Context) error {
	schema := `
CREATE TABLE IF NOT EXISTS short_urls (
	id SERIAL PRIMARY KEY,
	short_id TEXT UNIQUE NOT NULL,
	original_url TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL DEFAULT NOW(),
	deleted_at TIMESTAMP
);
`
	tx, beginErr := r.pool.Begin(ctx)
	if beginErr != nil {
		middleware.Log.Printf("cannot begin tx: " + beginErr.Error())
		return errors.New("cannot begin tx: " + beginErr.Error())
	}
	defer tx.Rollback(ctx)

	_, execErr := tx.Exec(ctx, schema)
	if execErr != nil {
		middleware.Log.Printf("cannot create table: " + execErr.Error())
		return errors.New("cannot create table: " + execErr.Error())
	}

	commitErr := tx.Commit(ctx)
	if commitErr != nil {
		middleware.Log.Printf("cannot commit tx: " + commitErr.Error())
		return errors.New("cannot commit tx: " + commitErr.Error())
	}

	return nil
}

func (r *RDB) Save(ctx context.Context, urlToSave *url.URL, cfg *app.Config) (string, error) {
	const maxRetries = 5
	const randLen = 8

	for i := 0; i < maxRetries; i++ {
		randomID, genErr := endpoints.RandStringRunes(randLen)
		if genErr != nil {
			middleware.Log.Printf("failed random ID: " + genErr.Error())
			return "", errors.New("failed random ID: " + genErr.Error())
		}

		sqlInsert := `
INSERT INTO short_urls (short_id, original_url) 
VALUES ($1, $2)
`
		_, execErr := r.pool.Exec(ctx, sqlInsert, randomID, urlToSave.String())
		if execErr == nil {
			return ensureSlash(cfg.BaseURL) + randomID, nil
		}

		// If unique violation, try next random
		if isUniqueViolation(execErr) {
			continue
		}

		middleware.Log.Printf("db insert error: " + execErr.Error())
		return "", errors.New("db insert error: " + execErr.Error())
	}

	middleware.Log.Printf("failed to generate a unique short_id after retries")
	return "", errors.New("failed to generate a unique short_id")
}

// SaveBatch inserts multiple URLs in one transaction, each with random short ID, re-trying on conflict.
func (r *RDB) SaveBatch(ctx context.Context, urls []*url.URL, cfg *app.Config) ([]string, error) {
	if len(urls) == 0 {
		return nil, nil
	}

	tx, beginErr := r.pool.Begin(ctx)
	if beginErr != nil {
		middleware.Log.Printf("cannot begin tx: " + beginErr.Error())
		return nil, errors.New("cannot begin tx: " + beginErr.Error())
	}

	defer func() {
		// If we return an error at any point, rollback will be called automatically
		// if we don't commit below.
		tx.Rollback(ctx)
	}()

	results := make([]string, 0, len(urls))
	for _, u := range urls {
		const maxRetries = 5
		const randLen = 8

		var finalURL string
		for i := 0; i < maxRetries; i++ {
			randomID, genErr := endpoints.RandStringRunes(randLen)
			if genErr != nil {
				middleware.Log.Printf("failed random ID in batch: " + genErr.Error())
				return nil, errors.New("failed random ID: " + genErr.Error())
			}

			sqlInsert := `
INSERT INTO short_urls (short_id, original_url)
VALUES ($1, $2)
`
			_, execErr := tx.Exec(ctx, sqlInsert, randomID, u.String())
			if execErr == nil {
				finalURL = ensureSlash(cfg.BaseURL) + randomID
				break
			}
			if isUniqueViolation(execErr) {
				continue
			}
			middleware.Log.Printf("db insert error (batch): " + execErr.Error())
			return nil, errors.New("db insert error (batch): " + execErr.Error())
		}

		if finalURL == "" {
			middleware.Log.Printf("failed to generate a unique short_id in batch")
			return nil, errors.New("failed to generate a unique short_id in batch")
		}

		results = append(results, finalURL)
	}

	commitErr := tx.Commit(ctx)
	if commitErr != nil {
		middleware.Log.Printf("commit error in batch: " + commitErr.Error())
		return nil, errors.New("commit error in batch: " + commitErr.Error())
	}

	return results, nil
}

func (r *RDB) Load(ctx context.Context, shortID string) (*url.URL, error) {
	var rawURL string
	var deletedAt *time.Time

	sqlSelect := `SELECT original_url, deleted_at FROM short_urls WHERE short_id = $1`
	sErr := r.pool.QueryRow(ctx, sqlSelect, shortID).Scan(&rawURL, &deletedAt)
	if sErr != nil {
		if errors.Is(sErr, pgx.ErrNoRows) {
			return nil, nil
		}
		middleware.Log.Printf("db select error: " + sErr.Error())
		return nil, errors.New("db select error: " + sErr.Error())
	}
	if deletedAt != nil {
		middleware.Log.Printf("short URL is marked deleted for: " + shortID)
		return nil, errors.New("short URL is marked deleted")
	}

	parsed, pErr := url.Parse(rawURL)
	if pErr != nil {
		middleware.Log.Printf("invalid URL in DB: " + pErr.Error())
		return nil, errors.New("invalid stored URL: " + pErr.Error())
	}
	return parsed, nil
}

func (r *RDB) Ping(ctx context.Context) error {
	pErr := r.pool.Ping(ctx)
	if pErr != nil {
		middleware.Log.Printf("failed to ping db: " + pErr.Error())
		return errors.New("ping error: " + pErr.Error())
	}
	return nil
}

func (r *RDB) Close(ctx context.Context) error {
	r.pool.Close()
	return nil
}

// isUniqueViolation checks if err is a Postgres unique constraint error.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

func ensureSlash(baseURL string) string {
	if !strings.HasSuffix(baseURL, "/") {
		return baseURL + "/"
	}
	return baseURL
}
