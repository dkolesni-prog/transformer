// Internal/app/Storage.go.

package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"sync"
)

type Record struct { // так мы будем хранить в файле.
	UUID        string `json:"uuid"`
	ShortURL    string `json:"short_url"`
	OriginalURL string `json:"original_url"`
}

type Storage struct {
	mu                *sync.Mutex
	keyShortValuelong map[string]string
	filePath          string
}

func (s *Storage) loadFromFile() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.Open(s.filePath)
	if errors.Is(err, os.ErrNotExist) {
		// Если файла нет — не ошибка, просто пропускаем
		return nil
	}
	if err != nil {
		Log.Error().Err(err).Msg("error opening file")
		return errors.New("open file: " + err.Error())
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		var rec Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			// Если строка не парсится, можно логировать ошибку и продолжать
			Log.Error().Err(err).Msgf("Error unmarshaling line: %s", line)
			continue
		}
		// Кладём данные в map (short -> original)
		s.keyShortValuelong[rec.ShortURL] = rec.OriginalURL
	}
	scErr := scanner.Err()
	if scErr != nil {
		Log.Error().Err(scErr).Msg("scanner error in loadFromFile")
		return errors.New("scanner error: " + scErr.Error())
	}
	return nil
}

// saveRecord добавляет одну запись в файл (дозапись в конец файла).
func (s *Storage) saveRecord(short, original string) error {
	// Допустим, для "uuid" сделаем либо auto-increment, либо пока пустую строку.
	rec := Record{
		UUID:        "", // Можно придумать, как генерировать уникальные ID.
		ShortURL:    short,
		OriginalURL: original,
	}

	data, err := json.Marshal(rec)
	if err != nil {
		Log.Error().Err(err).Msg("failed to marshal record")
		return errors.New("marshal record: " + err.Error())
	}

	file, err := os.OpenFile(s.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		Log.Error().Err(err).Msg("open file error")
		return errors.New("open file: " + err.Error())
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			Log.Error().Err(cerr).Msg("file close error")
		}
	}()

	if _, err := file.Write(data); err != nil {
		Log.Error().Err(err).Msg("file write data")
		return errors.New("file write data: " + err.Error())
	}
	if _, err := file.WriteString("\n"); err != nil {
		Log.Error().Err(err).Msg("file write newline")
		return errors.New("file write newline: " + err.Error())
	}
	return nil
}

func NewStorage(filepath string) *Storage {
	s := &Storage{
		mu:                &sync.Mutex{},
		keyShortValuelong: make(map[string]string),
		filePath:          filepath,
	}
	if err := s.loadFromFile(); err != nil {
		Log.Error().Err(err).Msg("Error loading data from file")
	}
	return s
}

func (s *Storage) SetIfAbsent(short, long string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.keyShortValuelong[short]; ok {
		return "", false
	}
	s.keyShortValuelong[short] = long

	// Сохраняем запись на диск
	if err := s.saveRecord(short, long); err != nil {
		Log.Error().Err(err).Msg("Error saving record to file")
	}

	return short, true
}

func (s *Storage) Get(short string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	long, ok := s.keyShortValuelong[short]
	return long, ok
}
