// Internal/store/dbStorage.go.

package store

import (
	"context"
	"errors"

	"net/url"
	"strings"
	"time"

	"github.com/dkolesni-prog/transformer/internal/config"

	"github.com/dkolesni-prog/transformer/internal/app/middleware"
	"github.com/dkolesni-prog/transformer/internal/helpers"

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
		middleware.Log.Error().Err(parseErr).Msg("Parse DSN error")
		return nil, errors.New("parse DSN error: " + parseErr.Error())
	}

	pool, poolErr := pgxpool.NewWithConfig(ctx, cfg)
	if poolErr != nil {
		middleware.Log.Error().Err(poolErr).Msg("cannot create pgxpoo")
		return nil, errors.New("cannot create pgxpool: " + poolErr.Error())
	}

	pingErr := pool.Ping(ctx)
	if pingErr != nil {
		pool.Close()
		middleware.Log.Error().Err(pingErr).Msg("failed ping")
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
		middleware.Log.Error().Err(beginErr).Msg("failed ping")
		return errors.New("cannot begin tx: " + beginErr.Error())
	}
	defer func(tx pgx.Tx, ctx context.Context) {
		err := tx.Rollback(ctx)
		if err != nil {
			middleware.Log.Error().Err(err).Msg("cannot rollback")
		}
	}(tx, ctx)

	_, execErr := tx.Exec(ctx, schema)
	if execErr != nil {
		middleware.Log.Error().Err(execErr).Msg("failed ping")
		return errors.New("cannot create table: " + execErr.Error())
	}

	commitErr := tx.Commit(ctx)
	if commitErr != nil {
		middleware.Log.Error().Err(commitErr).Msg("cannot commit tx: ")
		return errors.New("cannot commit tx: " + commitErr.Error())
	}

	return nil
}

func (r *RDB) Save(ctx context.Context, urlToSave *url.URL, cfg *config.Config) (string, error) {
	const maxRetries = 5
	const randLen = 8

	for range maxRetries {
		randomID, genErr := helpers.RandStringRunes(randLen)
		if genErr != nil {
			middleware.Log.Error().Err(genErr).Msg("Failed to generate random ID")
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

		middleware.Log.Error().Err(execErr).Msg("Database insert error")

		return "", errors.New("db insert error: " + execErr.Error())
	}

	middleware.Log.Printf("failed to generate a unique short_id after retries")
	return "", errors.New("failed to generate a unique short_id")
}

// SaveBatch inserts multiple URLs in one transaction, each with random short ID, re-trying on conflict.
func (r *RDB) SaveBatch(ctx context.Context, urls []*url.URL, cfg *config.Config) ([]string, error) {
	if len(urls) == 0 {
		return nil, nil
	}

	tx, beginErr := r.pool.Begin(ctx)
	if beginErr != nil {
		middleware.Log.Error().Err(beginErr).Msg("Cannot begin transaction")
		return nil, errors.New("cannot begin transaction")
	}

	defer func(tx pgx.Tx, ctx context.Context) {
		err := tx.Rollback(ctx)
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			middleware.Log.Error().Err(err).Msg("Cannot rollback transaction")
		}
	}(tx, ctx)

	results := make([]string, 0, len(urls))
	for _, u := range urls {
		const maxRetries = 5
		const randLen = 8

		var finalURL string
		for range maxRetries {
			randomID, genErr := helpers.RandStringRunes(randLen)
			if genErr != nil {
				middleware.Log.Error().Err(genErr).Msg("Failed to generate random ID in batch")
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
			middleware.Log.Error().Err(execErr).Msg("DB insert error in batch")
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
		middleware.Log.Error().Err(commitErr).Msg("Commit error in batch")
		return nil, errors.New("commit error in batch")
	}

	return results, nil
}

func (r *RDB) Load(ctx context.Context, shortID string) (*url.URL, error) {
	var rawURL string
	var deletedAt *time.Time

	var ErrNoRowsFound = errors.New("no rows found for the provided short_id")

	sqlSelect := `SELECT original_url, deleted_at FROM short_urls WHERE short_id = $1`
	sErr := r.pool.QueryRow(ctx, sqlSelect, shortID).Scan(&rawURL, &deletedAt)
	if sErr != nil {
		if errors.Is(sErr, pgx.ErrNoRows) {
			middleware.Log.Info().Str("shortID", shortID).Msg("No rows found for short ID")
			return nil, ErrNoRowsFound
		}
		middleware.Log.Error().Err(sErr).Str("shortID", shortID).Msg("Database query error")
		return nil, errors.New("database query error")
	}

	if deletedAt != nil {
		middleware.Log.Warn().Str("shortID", shortID).Msg("Short URL is marked deleted")
		return nil, errors.New("short URL is marked deleted")
	}

	parsed, pErr := url.Parse(rawURL)
	if pErr != nil {
		middleware.Log.Error().Err(pErr).Str("rawURL", rawURL).Msg("Invalid URL in database")
		return nil, errors.New("invalid stored URL")
	}

	return parsed, nil
}

func (r *RDB) Ping(ctx context.Context) error {
	pErr := r.pool.Ping(ctx)
	if pErr != nil {
		middleware.Log.Error().Err(pErr).Msg("Failed to ping database")
		return errors.New("ping error")
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
