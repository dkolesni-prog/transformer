// internal/store/dbStorage.go
package store

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/dkolesni-prog/transformer/internal/app/middleware"
	"github.com/dkolesni-prog/transformer/internal/config"
	"github.com/dkolesni-prog/transformer/internal/helpers"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RDB is our database wrapper.
type RDB struct {
	pool *pgxpool.Pool
}

// NewRDB initializes a new RDB instance.
func NewRDB(ctx context.Context, dsn string) (*RDB, error) {
	cfg, parseErr := pgxpool.ParseConfig(dsn)
	if parseErr != nil {
		middleware.Log.Error().Err(parseErr).Msg("Could not parse DSN")
		return nil, errors.New("parse DSN error: " + parseErr.Error())
	}

	pool, poolErr := pgxpool.NewWithConfig(ctx, cfg)
	if poolErr != nil {
		middleware.Log.Error().Err(poolErr).Msg("Could not create pgxpool")
		return nil, errors.New("cannot create pgxpool: " + poolErr.Error())
	}

	if pingErr := pool.Ping(ctx); pingErr != nil {
		middleware.Log.Error().Err(pingErr).Msg("Could not ping database")
		// Close doesn't return an error, so we just call it
		pool.Close()

		return nil, errors.New("failed ping: " + pingErr.Error())
	}

	return &RDB{pool: pool}, nil
}

// Bootstrap creates the table if it doesn't exist.
func (r *RDB) Bootstrap(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS short_urls (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    short_id VARCHAR(16) UNIQUE NOT NULL,
    original_url VARCHAR(2048) UNIQUE NOT NULL,
    user_id VARCHAR(64) NOT NULL,
    is_deleted BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP
);
`
	tx, beginErr := r.pool.Begin(ctx)
	if beginErr != nil {
		middleware.Log.Error().Err(beginErr).Msg("Could not begin transaction in Bootstrap")
		return errors.New("cannot begin tx: " + beginErr.Error())
	}
	// Rollback will be a no-op if Commit succeeds.
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, execErr := tx.Exec(ctx, schema); execErr != nil {
		middleware.Log.Error().Err(execErr).Msg("Could not create table in Bootstrap")
		return errors.New("cannot create table: " + execErr.Error())
	}
	if commitErr := tx.Commit(ctx); commitErr != nil {
		middleware.Log.Error().Err(commitErr).Msg("Could not commit transaction in Bootstrap")
		return errors.New("cannot commit tx: " + commitErr.Error())
	}
	return nil
}

// Save inserts a single URL. Tries maxRetries times to generate a random short_id.
func (r *RDB) Save(ctx context.Context, userID string, urlToSave *url.URL, cfg *config.Config) (string, error) {
	const maxRetries = 5
	const randLen = 8

	for range make([]struct{}, maxRetries) {
		randomID, genErr := helpers.RandStringRunes(randLen)
		if genErr != nil {
			middleware.Log.Error().Err(genErr).Msg("Could not generate random short_id")
			return "", errors.New("failed to generate random ID: " + genErr.Error())
		}

		sqlInsert := `
INSERT INTO short_urls (short_id, original_url, user_id)
VALUES ($1, $2, $3)
ON CONFLICT (original_url) DO NOTHING
RETURNING short_id;
`
		var shortID string
		scanErr := r.pool.QueryRow(ctx, sqlInsert, randomID, urlToSave.String(), userID).Scan(&shortID)
		if scanErr == nil {
			return ensureSlash(cfg.BaseURL) + shortID, nil
		}

		if errors.Is(scanErr, pgx.ErrNoRows) {
			var existingID string
			confSQL := `SELECT short_id FROM short_urls WHERE original_url=$1;`
			if selErr := r.pool.QueryRow(ctx, confSQL, urlToSave.String()).Scan(&existingID); selErr == nil {
				return ensureSlash(cfg.BaseURL) + existingID, errors.New("conflict: URL already exists")
			}
		}
	}
	return "", errors.New("failed to generate a unique short_id after retries")
}

// LoadFull retrieves the original URL and is_deleted flag by short_id.
func (r *RDB) LoadFull(ctx context.Context, shortID string) (*url.URL, bool, error) {
	const sqlSelect = `
SELECT original_url, is_deleted
FROM short_urls
WHERE short_id = $1;
`
	var rawURL string
	var isDeleted bool

	scanErr := r.pool.QueryRow(ctx, sqlSelect, shortID).Scan(&rawURL, &isDeleted)
	if errors.Is(scanErr, pgx.ErrNoRows) {
		return nil, false, errors.New("not found")
	}
	if scanErr != nil {
		middleware.Log.Error().Err(scanErr).Msg("LoadFull query failed")
		return nil, false, errors.New("LoadFull query: " + scanErr.Error())
	}

	parsed, parseErr := url.Parse(rawURL)
	if parseErr != nil {
		middleware.Log.Error().Err(parseErr).Msg("Bad URL in DB record")
		return nil, false, errors.New("bad URL in DB: " + parseErr.Error())
	}
	return parsed, isDeleted, nil
}

// SaveBatch inserts a list of URLs using pgx.Batch to minimize round trips.
func (r *RDB) SaveBatch(ctx context.Context, userID string, urls []*url.URL, cfg *config.Config) ([]string, error) {
	const maxRetries = 5
	const randLen = 8

	batch := &pgx.Batch{}
	genMap := make(map[string]string)

	// Prepare batch of INSERT statements.
	for _, u := range urls {
		success := false
		for range make([]struct{}, maxRetries) {
			randVal, genErr := helpers.RandStringRunes(randLen)
			if genErr != nil {
				middleware.Log.Error().Err(genErr).Msg("Could not generate random short_id in SaveBatch")
				return nil, errors.New("rand string error: " + genErr.Error())
			}

			genMap[u.String()] = randVal
			batch.Queue(`
INSERT INTO short_urls (short_id, original_url, user_id)
VALUES ($1, $2, $3)
ON CONFLICT (original_url) DO NOTHING
RETURNING short_id;
`, randVal, u.String(), userID)

			success = true
			break
		}
		if !success {
			return nil, errors.New("could not generate a short_id for URL: " + u.String())
		}
	}

	br := r.pool.SendBatch(ctx, batch)
	defer func() {
		if closeErr := br.Close(); closeErr != nil {
			middleware.Log.Error().Err(closeErr).Msg("Could not close batch results in SaveBatch")
		}
	}()

	var results []string
	for _, u := range urls {
		var returnedID string
		scanErr := br.QueryRow().Scan(&returnedID)
		if errors.Is(scanErr, pgx.ErrNoRows) {
			// ON CONFLICT DO NOTHING triggered => find existing short_id
			confSQL := `SELECT short_id FROM short_urls WHERE original_url = $1;`
			var existingID string
			if selErr := r.pool.QueryRow(ctx, confSQL, u.String()).Scan(&existingID); selErr == nil {
				returnedID = existingID
			} else {
				middleware.Log.Error().Err(selErr).Msg("Failed to retrieve existing short_id in SaveBatch")
				return nil, errors.New("failed to retrieve existing short_id: " + selErr.Error())
			}
		} else if scanErr != nil {
			middleware.Log.Error().Err(scanErr).Msg("Batch execution failed in SaveBatch")
			return nil, errors.New("batch execution failed: " + scanErr.Error())
		}
		results = append(results, ensureSlash(cfg.BaseURL)+returnedID)
	}

	return results, nil
}

// LoadUserURLs retrieves all non-deleted URLs for a given user.
func (r *RDB) LoadUserURLs(ctx context.Context, userID string, baseURL string) ([]UserURL, error) {
	const sqlSelect = `
SELECT short_id, original_url
FROM short_urls
WHERE user_id = $1
  AND is_deleted = false;
`
	rows, queryErr := r.pool.Query(ctx, sqlSelect, userID)
	if queryErr != nil {
		middleware.Log.Error().Err(queryErr).Msg("LoadUserURLs query failed")
		return nil, errors.New("LoadUserURLs: " + queryErr.Error())
	}
	defer rows.Close()

	var out []UserURL
	for rows.Next() {
		var sid, orig string
		scanErr := rows.Scan(&sid, &orig)
		if scanErr != nil {
			middleware.Log.Error().Err(scanErr).Msg("Rows scan failed in LoadUserURLs")
			return nil, errors.New("rows.Scan: " + scanErr.Error())
		}
		out = append(out, UserURL{
			ShortURL:    ensureSlash(baseURL) + sid,
			OriginalURL: orig,
		})
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		middleware.Log.Error().Err(rowsErr).Msg("Rows iteration error in LoadUserURLs")
		return nil, errors.New("rows.Err: " + rowsErr.Error())
	}
	return out, nil
}

// DeleteBatch sets is_deleted = true for multiple shortIDs belonging to a single userID.
func (r *RDB) DeleteBatch(ctx context.Context, userID string, shortIDs []string) error {
	const sqlUpdate = `
UPDATE short_urls
SET is_deleted = true,
    deleted_at = now()
WHERE user_id = $1
  AND short_id = ANY($2);
`
	if _, execErr := r.pool.Exec(ctx, sqlUpdate, userID, shortIDs); execErr != nil {
		middleware.Log.Error().Err(execErr).Msg("DeleteBatch update failed")
		return errors.New("DeleteBatch: " + execErr.Error())
	}
	return nil
}

func (r *RDB) Ping(ctx context.Context) error {
	pingErr := r.pool.Ping(ctx)
	if pingErr != nil {
		middleware.Log.Error().Err(pingErr).Msg("Ping to database failed")
		return errors.New("ping error: " + pingErr.Error())
	}
	return nil
}

func (r *RDB) Close(ctx context.Context) error {
	r.pool.Close()
	return nil
}

func ensureSlash(baseURL string) string {
	if !strings.HasSuffix(baseURL, "/") {
		return baseURL + "/"
	}
	return baseURL
}
