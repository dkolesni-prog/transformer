package app

import (
	"testing"
)

func TestShortenURLDirect(t *testing.T) {
	storage := NewStorage("test_data.json")
	shortURL, err := createShortURL("https://example.com", storage, "http://localhost:8080/")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if shortURL == "" {
		t.Fatal("shortURL is empty")
	}
}
