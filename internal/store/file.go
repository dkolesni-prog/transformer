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

type Record struct {
	UUID        string `json:"uuid"`
	ShortURL    string `json:"short_url"`
	OriginalURL string `json:"original_url"`
	UserID      string `json:"user_id"`
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

// --------------------------------------------------
// Реализация интерфейса Store
// --------------------------------------------------

func (s *Storage) Save(ctx context.Context, userID string, urlToSave *url.URL, cfg *config.Config) (string, error) {
	const maxRetries = 5
	const randLen = 8

	var shortID string
	var success bool

	for i := 0; i < maxRetries; i++ {
		randVal, err := helpers.RandStringRunes(randLen)
		if err != nil {
			return "", fmt.Errorf("rand string error: %w", err)
		}

		s.mu.Lock()
		_, exists := s.keyShortValuelong[randVal]
		if !exists {
			// Создаём новый Record, включая userID
			rec := Record{
				UUID:        "",
				ShortURL:    randVal,
				OriginalURL: urlToSave.String(),
				UserID:      userID,
			}
			s.keyShortValuelong[randVal] = rec

			if err := s.saveRecord(rec); err != nil {
				s.mu.Unlock()
				return "", fmt.Errorf("saveRecord error: %w", err)
			}
			s.mu.Unlock()

			shortID = randVal
			success = true
			break
		}
		s.mu.Unlock()
	}

	if !success {
		return "", errors.New("could not generate unique URL")
	}
	// Возвращаем полный короткий
	return ensureSlash(cfg.BaseURL) + shortID, nil
}

func (s *Storage) SaveBatch(ctx context.Context, userID string, urls []*url.URL, cfg *config.Config) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	results := make([]string, 0, len(urls))

	for _, u := range urls {
		shortID := strconv.Itoa(len(s.keyShortValuelong))

		rec := Record{
			UUID:        "",
			ShortURL:    shortID,
			OriginalURL: u.String(),
			UserID:      userID,
		}
		s.keyShortValuelong[shortID] = rec

		if err := s.saveRecord(rec); err != nil {
			middleware.Log.Error().Err(err).Msg("Error saving batch record to file")
			return nil, errors.New("error saving batch record to file")
		}

		results = append(results, ensureSlash(cfg.BaseURL)+shortID)
	}

	if len(results) != len(urls) {
		return nil, errors.New("not all URLs have been saved")
	}
	return results, nil
}

func (s *Storage) Load(ctx context.Context, shortID string) (*url.URL, error) {
	s.mu.Lock()
	rec, ok := s.keyShortValuelong[shortID]
	s.mu.Unlock()

	if !ok {
		return nil, errors.New("short ID not found")
	}
	parsed, err := url.Parse(rec.OriginalURL)
	if err != nil {
		return nil, errors.New("invalid stored URL")
	}
	return parsed, nil
}

func (s *Storage) LoadUserURLs(ctx context.Context, userID string, baseURL string) ([]UserURL, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []UserURL
	for shortID, rec := range s.keyShortValuelong {
		if rec.UserID == userID {
			result = append(result, UserURL{
				ShortURL:    ensureSlash(baseURL) + shortID,
				OriginalURL: rec.OriginalURL,
			})
		}
	}
	return result, nil
}

func (s *Storage) Ping(ctx context.Context) error {
	return nil
}

func (s *Storage) Close(ctx context.Context) error {
	return nil
}

func (s *Storage) Bootstrap(ctx context.Context) error {
	return nil
}

// --------------------------------------------------
// Вспомогательные методы
// --------------------------------------------------

// loadFromFile считывает все записи из файла, кладёт в map.
func (s *Storage) loadFromFile() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.Open(s.filePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		var rec Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			middleware.Log.Error().Err(err).Msgf("Error unmarshaling line: %s", line)
			continue
		}
		// Сохраняем в map
		s.keyShortValuelong[rec.ShortURL] = rec
	}
	if scErr := scanner.Err(); scErr != nil {
		return fmt.Errorf("scanner error: %w", scErr)
	}
	return nil
}

func (s *Storage) saveRecord(rec Record) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal record: %w", err)
	}

	file, err := os.OpenFile(s.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("file write data: %w", err)
	}
	if _, err := file.WriteString("\n"); err != nil {
		return fmt.Errorf("file write newline: %w", err)
	}
	return nil
}

// Вспомогательный метод для тесто
func (s *Storage) SetIfAbsent(short, longURL string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.keyShortValuelong[short]; ok {
		return "", false
	}

	rec := Record{
		UUID:        "",
		ShortURL:    short,
		OriginalURL: longURL,
		UserID:      "",
	}
	s.keyShortValuelong[short] = rec

	if err := s.saveRecord(rec); err != nil {
		middleware.Log.Error().Err(err).Msg("Error saving record to file")
	}
	return short, true
}

// Вспомогательный метод для тестов
func (s *Storage) Get(short string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.keyShortValuelong[short]
	if !ok {
		return "", false
	}
	return rec.OriginalURL, true
}
