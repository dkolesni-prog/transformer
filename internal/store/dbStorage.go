// Internal/store/dbStorage.go.

package store

import (
	"context"
	"errors"
	"fmt"

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
	defer func() {
		err := tx.Rollback(ctx)
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			middleware.Log.Error().Err(err).Msg("cannot rollback")
		}
	}()

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
			return "", fmt.Errorf("failed to generate random ID: %w", genErr)
		}

		sqlInsert := `
INSERT INTO short_urls (short_id, original_url) 
VALUES ($1, $2)
ON CONFLICT (original_url) DO UPDATE SET short_id = EXCLUDED.short_id
RETURNING short_id;
`
		var shortID string
		err := r.pool.QueryRow(ctx, sqlInsert, randomID, urlToSave.String()).Scan(&shortID)
		if err == nil {
			return ensureSlash(cfg.BaseURL) + shortID, nil
		}
		middleware.Log.Error().Err(err).Msg("Database insert error, retrying")
	}

	middleware.Log.Warn().Msg("Failed to generate a unique short_id after retries")
	return "", errors.New("failed to generate a unique short_id")
}

func (r *RDB) SaveBatch(ctx context.Context, urls []*url.URL, cfg *config.Config) ([]string, error) {
	const maxRetries = 5
	const randLen = 8

	tx, beginErr := r.pool.Begin(ctx)
	if beginErr != nil {
		middleware.Log.Error().Err(beginErr).Msg("Failed to begin transaction")
		return nil, fmt.Errorf("failed to begin transaction: %w", beginErr)
	}

	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			middleware.Log.Error().Err(err).Msg("Failed to rollback transaction")
		}
	}()

	var results []string
	for _, urlToSave := range urls {
		var shortID string
		success := false

		for range maxRetries {
			randomID, genErr := helpers.RandStringRunes(randLen)
			if genErr != nil {
				middleware.Log.Error().Err(genErr).Msg("Failed to generate random ID in batch")
				return nil, fmt.Errorf("failed to generate random ID in batch: %w", genErr)
			}

			sqlInsert := `
INSERT INTO short_urls (short_id, original_url)
VALUES ($1, $2)
ON CONFLICT (original_url) DO UPDATE SET short_id = EXCLUDED.short_id
RETURNING short_id;
`
			err := tx.QueryRow(ctx, sqlInsert, randomID, urlToSave.String()).Scan(&shortID)
			if err == nil {
				success = true
				results = append(results, ensureSlash(cfg.BaseURL)+shortID)
				break
			}

			if isUniqueViolation(err) {
				continue
			}

			middleware.Log.Error().Err(err).Msg("Failed to insert URL in batch")
			return nil, fmt.Errorf("failed to insert URL in batch: %w", err)
		}

		if !success {
			middleware.Log.Error().Str("url", urlToSave.String()).Msg("Failed to save URL after retries")
			return nil, errors.New("failed to save URL after retries")
		}
	}

	if commitErr := tx.Commit(ctx); commitErr != nil {
		middleware.Log.Error().Err(commitErr).Msg("Failed to commit transaction")
		return nil, fmt.Errorf("failed to commit transaction: %w", commitErr)
	}

	return results, nil
}

func (r *RDB) Load(ctx context.Context, shortID string) (*url.URL, error) {
	middleware.Log.Info().Str("shortID", shortID).Msg("Starting Load operation")
	var rawURL string
	var deletedAt *time.Time

	var ErrNoRowsFound = errors.New("no rows found for the provided short_id")

	sqlSelect := `SELECT original_url, deleted_at FROM short_urls WHERE short_id = $1`
	middleware.Log.Debug().Str("query", sqlSelect).Str("shortID", shortID).Msg("Executing query")
	sErr := r.pool.QueryRow(ctx, sqlSelect, shortID).Scan(&rawURL, &deletedAt)
	if sErr != nil {
		if errors.Is(sErr, pgx.ErrNoRows) {
			middleware.Log.Info().Str("shortID", shortID).Msg("No rows found for short ID")
			return nil, ErrNoRowsFound
		}
		middleware.Log.Error().Err(sErr).Str("shortID", shortID).Msg("Database query error")
		return nil, errors.New("database query error")
	}

	middleware.Log.Debug().
		Str("shortID", shortID).
		Str("rawURL", rawURL).
		Interface("deletedAt", deletedAt).
		Msg("Query succeeded, checking deleted status")

	if deletedAt != nil {
		middleware.Log.Warn().Str("shortID", shortID).Msg("Short URL is marked deleted")
		return nil, errors.New("short URL is marked deleted")
	}

	parsed, pErr := url.Parse(rawURL)
	if pErr != nil {
		middleware.Log.Error().Err(pErr).Str("rawURL", rawURL).Msg("Invalid URL in database")
		return nil, errors.New("invalid stored URL")
	}
	middleware.Log.Info().
		Str("shortID", shortID).
		Str("parsedURL", parsed.String()).
		Msg("Successfully loaded short URL")
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
