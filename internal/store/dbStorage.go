// internal/store/dbStorage.go
package store

import (
	"context"
	"errors"
	"fmt"
	"github.com/dkolesni-prog/transformer/internal/config"
	"github.com/dkolesni-prog/transformer/internal/helpers"
	"net/url"
	"strings"

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
		return nil, fmt.Errorf("parse DSN error: %w", parseErr)
	}

	pool, poolErr := pgxpool.NewWithConfig(ctx, cfg)
	if poolErr != nil {
		return nil, fmt.Errorf("cannot create pgxpool: %w", poolErr)
	}

	if pingErr := pool.Ping(ctx); pingErr != nil {
		pool.Close()
		return nil, fmt.Errorf("failed ping: %w", pingErr)
	}

	return &RDB{pool: pool}, nil
}

// Bootstrap — создаём таблицу, если нет. Добавьте is_deleted bool, если нужно.
func (r *RDB) Bootstrap(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS short_urls (
    id SERIAL PRIMARY KEY,
    short_id TEXT UNIQUE NOT NULL,
    original_url TEXT UNIQUE NOT NULL,
    user_id TEXT NOT NULL,
    is_deleted BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP
);
`
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cannot begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, execErr := tx.Exec(ctx, schema); execErr != nil {
		return fmt.Errorf("cannot create table: %w", execErr)
	}
	if commitErr := tx.Commit(ctx); commitErr != nil {
		return fmt.Errorf("cannot commit tx: %w", commitErr)
	}
	return nil
}

// Save — создаём одну запись.
func (r *RDB) Save(ctx context.Context, userID string, urlToSave *url.URL, cfg *config.Config) (string, error) {
	const maxRetries = 5
	const randLen = 8

	for i := 0; i < maxRetries; i++ {
		randomID, genErr := helpers.RandStringRunes(randLen)
		if genErr != nil {
			return "", fmt.Errorf("failed to generate random ID: %w", genErr)
		}

		sqlInsert := `
INSERT INTO short_urls (short_id, original_url, user_id)
VALUES ($1, $2, $3)
ON CONFLICT (original_url) DO NOTHING
RETURNING short_id;
`
		var shortID string
		err := r.pool.QueryRow(ctx, sqlInsert, randomID, urlToSave.String(), userID).Scan(&shortID)
		if err == nil {
			return ensureSlash(cfg.BaseURL) + shortID, nil
		}
		// Если вернулся pgx.ErrNoRows => значит CONFLICT
		if errors.Is(err, pgx.ErrNoRows) {
			var existingID string
			confSQL := `SELECT short_id FROM short_urls WHERE original_url=$1;`
			if selErr := r.pool.QueryRow(ctx, confSQL, urlToSave.String()).Scan(&existingID); selErr == nil {
				return ensureSlash(cfg.BaseURL) + existingID, errors.New("conflict: URL already exists")
			}
		}
	}
	return "", errors.New("failed to generate a unique short_id after retries")
}

// LoadFull — возвращает URL + isDeleted + ошибку.
func (r *RDB) LoadFull(ctx context.Context, shortID string) (*url.URL, bool, error) {
	const sqlSelect = `
SELECT original_url, is_deleted
FROM short_urls
WHERE short_id = $1;
`
	var rawURL string
	var isDel bool

	err := r.pool.QueryRow(ctx, sqlSelect, shortID).Scan(&rawURL, &isDel)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, errors.New("not found")
	}
	if err != nil {
		return nil, false, fmt.Errorf("LoadFull query: %w", err)
	}

	parsed, pErr := url.Parse(rawURL)
	if pErr != nil {
		return nil, false, fmt.Errorf("bad URL in DB: %w", pErr)
	}
	return parsed, isDel, nil
}

// SaveBatch — сохраняем несколько ссылок.
func (r *RDB) SaveBatch(ctx context.Context, userID string, urls []*url.URL, cfg *config.Config) ([]string, error) {
	const maxRetries = 5
	const randLen = 8

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx) // игнорируем ошибку, если уже коммит
	}()

	var results []string
	for _, u := range urls {
		var shortID string
		success := false
	RETRY_LOOP:
		for i := 0; i < maxRetries; i++ {
			randVal, genErr := helpers.RandStringRunes(randLen)
			if genErr != nil {
				return nil, fmt.Errorf("rand string error: %w", genErr)
			}
			insertSQL := `
INSERT INTO short_urls (short_id, original_url, user_id)
VALUES ($1, $2, $3)
ON CONFLICT (original_url) DO NOTHING
RETURNING short_id;
`
			if err := tx.QueryRow(ctx, insertSQL, randVal, u.String(), userID).Scan(&shortID); err == nil {
				results = append(results, ensureSlash(cfg.BaseURL)+shortID)
				success = true
				break RETRY_LOOP
			}
		}
		if !success {
			return nil, errors.New("could not save one of the URLs")
		}
	}

	if commitErr := tx.Commit(ctx); commitErr != nil {
		return nil, fmt.Errorf("commit transaction: %w", commitErr)
	}
	return results, nil
}

func (r *RDB) LoadUserURLs(ctx context.Context, userID string, baseURL string) ([]UserURL, error) {
	const sqlSelect = `
SELECT short_id, original_url
FROM short_urls
WHERE user_id = $1 AND is_deleted=false;
`
	rows, err := r.pool.Query(ctx, sqlSelect, userID)
	if err != nil {
		return nil, fmt.Errorf("LoadUserURLs: %w", err)
	}
	defer rows.Close()

	var out []UserURL
	for rows.Next() {
		var sid, orig string
		if err := rows.Scan(&sid, &orig); err != nil {
			return nil, fmt.Errorf("rows.Scan: %w", err)
		}
		out = append(out, UserURL{
			ShortURL:    ensureSlash(baseURL) + sid,
			OriginalURL: orig,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows.Err: %w", err)
	}
	return out, nil
}

// DeleteBatch — ставим is_deleted=true для нескольких shortID пользователя userID.
func (r *RDB) DeleteBatch(ctx context.Context, userID string, shortIDs []string) error {
	const sqlUpdate = `
UPDATE short_urls
SET is_deleted=true, deleted_at=now()
WHERE user_id = $1
  AND short_id = ANY($2);
`
	_, err := r.pool.Exec(ctx, sqlUpdate, userID, shortIDs)
	if err != nil {
		return fmt.Errorf("DeleteBatch: %w", err)
	}
	return nil
}

func (r *RDB) Ping(ctx context.Context) error {
	if err := r.pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping error: %w", err)
	}
	return nil
}

func (r *RDB) Close(ctx context.Context) error {
	r.pool.Close()
	return nil
}

// ensureSlash — вспомогательная функция
func ensureSlash(baseURL string) string {
	if !strings.HasSuffix(baseURL, "/") {
		return baseURL + "/"
	}
	return baseURL
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
