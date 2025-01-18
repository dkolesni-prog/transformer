// Internal/store/file.go.
package store

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"sync"

	"github.com/dkolesni-prog/transformer/internal/app/middleware"
	"github.com/dkolesni-prog/transformer/internal/config"
	"github.com/dkolesni-prog/transformer/internal/helpers"
)

type Record struct {
	UUID        string `json:"uuid"`
	ShortURL    string `json:"short_url"`
	OriginalURL string `json:"original_url"`
}

type Storage struct {
	mu                *sync.Mutex
	keyShortValuelong map[string]string
	filePath          string
}

func NewStorage(cfg *config.Config) *Storage {
	s := &Storage{
		mu:                &sync.Mutex{},
		keyShortValuelong: make(map[string]string),
		filePath:          cfg.FileStoragePath,
	}
	if err := s.loadFromFile(); err != nil {
		middleware.Log.Error().Err(err).Msg("Error loading data from file")
	}
	return s
}

// ----------------------------------------------.
// Satisfying store.Store interface.
// ----------------------------------------------.

func (s *Storage) Save(ctx context.Context, urlToSave *url.URL, cfg *config.Config) (string, error) {
	const maxRetries = 5
	const randValLength = 8
	var shortURL string
	var success bool

	for i := range make([]int, maxRetries) {
		randVal, err := helpers.RandStringRunes(randValLength)
		shortURL, success = s.SetIfAbsent(randVal, urlToSave.String())
		if success {
			break
		}
		if i == maxRetries-1 || err != nil {
			return "", errors.New("could not generate unique URL")
		}
	}

	fullShortURL := helpers.EnsureTrailingSlash(cfg.BaseURL) + shortURL
	return fullShortURL, nil
}

func (s *Storage) SaveBatch(ctx context.Context, urls []*url.URL, cfg *config.Config) ([]string, error) {
	if len(urls) == 0 {
		return nil, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	results := make([]string, 0, len(urls))
	for _, u := range urls {
		const randValLength = 8
		var short string
		var success bool

		for i := 0; i < 5; i++ { // Retry mechanism
			randVal, err := helpers.RandStringRunes(randValLength)
			if err != nil {
				middleware.Log.Error().Err(err).Msg("Failed to generate random string")
				return nil, errors.New("could not generate random string")
			}

			short, success = s.keyShortValuelong[randVal]
			if !success {
				s.keyShortValuelong[randVal] = u.String()
				short = randVal
				break
			}
		}

		if !success {
			middleware.Log.Error().Msgf("Failed to save URL: %s after retries", u.String())
			return nil, errors.New("failed to save URL after retries")
		}

		if err := s.saveRecord(short, u.String()); err != nil {
			middleware.Log.Error().Err(err).Msg("Failed to save batch record")
			return nil, err
		}

		fullShortURL := helpers.EnsureTrailingSlash(cfg.BaseURL) + short
		results = append(results, fullShortURL)
	}

	return results, nil
}

func (s *Storage) Load(ctx context.Context, shortID string) (*url.URL, error) {
	longVal, ok := s.Get(shortID)
	if !ok {
		return nil, errors.New("short ID not found")
	}
	parsed, err := url.Parse(longVal)
	if err != nil {
		return nil, errors.New("invalid stored URL")
	}
	return parsed, nil
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

// ----------------------------------------------.
// Helpers.
// ----------------------------------------------.

func (s *Storage) loadFromFile() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.Open(s.filePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		middleware.Log.Error().Err(err).Msg("error opening file")
		return errors.New("open file: " + err.Error())
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			middleware.Log.Error().Err(err).Msg("error closing file")
		}
	}(file)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		var rec Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			middleware.Log.Error().Err(err).Msgf("Error unmarshaling line: %s", line)
			continue
		}
		s.keyShortValuelong[rec.ShortURL] = rec.OriginalURL
	}

	if scErr := scanner.Err(); scErr != nil {
		middleware.Log.Error().Err(scErr).Msg("scanner error in loadFromFile")
		return errors.New("scanner error: " + scErr.Error())
	}
	return nil
}

// saveRecord just appends a JSON record to the file.
func (s *Storage) saveRecord(short, original string) error {
	rec := Record{
		UUID:        "",
		ShortURL:    short,
		OriginalURL: original,
	}

	data, err := json.Marshal(rec)
	if err != nil {
		middleware.Log.Error().Err(err).Msg("failed to marshal record")
		return errors.New("marshal record: " + err.Error())
	}

	file, err := os.OpenFile(s.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		middleware.Log.Error().Err(err).Msg("open file error")
		return errors.New("open file: " + err.Error())
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			middleware.Log.Error().Err(err).Msg("close file error")
		}
	}(file)

	if _, err := file.Write(data); err != nil {
		middleware.Log.Error().Err(err).Msg("file write data")
		return errors.New("file write data: " + err.Error())
	}
	if _, err := file.WriteString("\n"); err != nil {
		middleware.Log.Error().Err(err).Msg("file write newline")
		return errors.New("file write newline: " + err.Error())
	}
	return nil
}

func (s *Storage) SetIfAbsent(short, long string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.keyShortValuelong[short]; ok {
		return "", false
	}
	s.keyShortValuelong[short] = long

	if err := s.saveRecord(short, long); err != nil {
		middleware.Log.Error().Err(err).Msg("Error saving record to file")
	}
	return short, true
}

func (s *Storage) Get(short string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	long, ok := s.keyShortValuelong[short]
	return long, ok
}
