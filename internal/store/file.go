// internal/store/file.go
package store

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"sync"

	"github.com/dkolesni-prog/transformer/internal/app/middleware"
	"github.com/dkolesni-prog/transformer/internal/config"
	"github.com/dkolesni-prog/transformer/internal/helpers"
)

// Record ...
type Record struct {
	UUID        string `json:"uuid"`
	ShortURL    string `json:"short_url"`
	OriginalURL string `json:"original_url"`
	UserID      string `json:"user_id"`
	// Для простоты: признак удаления
	IsDeleted bool `json:"is_deleted"`
}

type Storage struct {
	mu                *sync.Mutex
	keyShortValuelong map[string]Record
	filePath          string
}

func NewStorage(cfg *config.Config) *Storage {
	s := &Storage{
		mu:                &sync.Mutex{},
		keyShortValuelong: make(map[string]Record),
		filePath:          cfg.FileStoragePath,
	}
	if err := s.loadFromFile(); err != nil {
		middleware.Log.Error().Err(err).Msg("Error loading data from file")
	}
	return s
}

func (s *Storage) Bootstrap(ctx context.Context) error {
	// Для file-хранилища не требуется
	return nil
}

func (s *Storage) Save(ctx context.Context, userID string, urlToSave *url.URL, cfg *config.Config) (string, error) {
	const maxRetries = 5
	const randLen = 8

	for i := 0; i < maxRetries; i++ {
		randVal, err := helpers.RandStringRunes(randLen)
		if err != nil {
			return "", fmt.Errorf("rand string error: %w", err)
		}
		s.mu.Lock()
		_, exists := s.keyShortValuelong[randVal]
		if !exists {
			rec := Record{
				ShortURL:    randVal,
				OriginalURL: urlToSave.String(),
				UserID:      userID,
			}
			s.keyShortValuelong[randVal] = rec
			if err := s.saveRecord(rec); err != nil {
				s.mu.Unlock()
				return "", fmt.Errorf("saveRecord: %w", err)
			}
			s.mu.Unlock()
			return ensureSlash(cfg.BaseURL) + randVal, nil
		}
		s.mu.Unlock()
	}
	return "", errors.New("could not generate unique URL")
}

func (s *Storage) SaveBatch(ctx context.Context, userID string, urls []*url.URL, cfg *config.Config) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var results []string
	for _, u := range urls {
		key := strconv.Itoa(len(s.keyShortValuelong))
		rec := Record{
			ShortURL:    key,
			OriginalURL: u.String(),
			UserID:      userID,
		}
		s.keyShortValuelong[key] = rec
		if err := s.saveRecord(rec); err != nil {
			return nil, fmt.Errorf("save batch record: %w", err)
		}
		results = append(results, ensureSlash(cfg.BaseURL)+key)
	}
	return results, nil
}

// LoadFull вернёт (URL, isDeleted, error).
func (s *Storage) LoadFull(ctx context.Context, shortID string) (*url.URL, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.keyShortValuelong[shortID]
	if !ok {
		return nil, false, errors.New("not found")
	}
	parsed, err := url.Parse(rec.OriginalURL)
	if err != nil {
		return nil, false, errors.New("invalid stored URL")
	}
	return parsed, rec.IsDeleted, nil
}

func (s *Storage) LoadUserURLs(ctx context.Context, userID string, baseURL string) ([]UserURL, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []UserURL
	for shortID, rec := range s.keyShortValuelong {
		if rec.UserID == userID && !rec.IsDeleted {
			result = append(result, UserURL{
				ShortURL:    ensureSlash(baseURL) + shortID,
				OriginalURL: rec.OriginalURL,
			})
		}
	}
	return result, nil
}

func (s *Storage) DeleteBatch(ctx context.Context, userID string, shortIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, sid := range shortIDs {
		rec, ok := s.keyShortValuelong[sid]
		if !ok {
			continue
		}
		if rec.UserID == userID {
			rec.IsDeleted = true
			recSavErr := s.saveRecord(rec)
			if recSavErr != nil {
				middleware.Log.Error().Err(recSavErr).Msg("Error saving record after delete")
			}
			s.keyShortValuelong[sid] = rec
		}
	}
	return nil
}

func (s *Storage) Ping(ctx context.Context) error {
	return nil
}

func (s *Storage) Close(ctx context.Context) error {
	return nil
}

// loadFromFile
func (s *Storage) loadFromFile() error {
	f, err := os.Open(s.filePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		var rec Record
		if unmarshalErr := json.Unmarshal([]byte(line), &rec); unmarshalErr != nil {
			middleware.Log.Error().Err(unmarshalErr).Msg("Error unmarshaling line")
			continue
		}
		s.keyShortValuelong[rec.ShortURL] = rec
	}
	if scErr := sc.Err(); scErr != nil {
		return fmt.Errorf("scanner: %w", scErr)
	}
	return nil
}

// saveRecord
func (s *Storage) saveRecord(rec Record) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	f, err := os.OpenFile(s.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	if _, wErr := f.Write(data); wErr != nil {
		return fmt.Errorf("write data: %w", wErr)
	}
	if _, w2Err := f.WriteString("\n"); w2Err != nil {
		return fmt.Errorf("write newline: %w", w2Err)
	}
	return nil
}

// Вспомогательный метод для тестов (SetIfAbsent).
func (s *Storage) SetIfAbsent(short, longURL string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.keyShortValuelong[short]; ok {
		return "", false
	}
	rec := Record{
		ShortURL:    short,
		OriginalURL: longURL,
		UserID:      "", // тест не задаёт
	}
	s.keyShortValuelong[short] = rec

	if err := s.saveRecord(rec); err != nil {
		middleware.Log.Error().Err(err).Msg("Error saving record to file in SetIfAbsent")
	}
	return short, true
}

// ensureSlash
