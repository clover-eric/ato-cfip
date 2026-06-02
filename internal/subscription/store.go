package subscription

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Entry struct {
	Token      string    `json:"token"`
	Name       string    `json:"name"`
	Source     string    `json:"source"`
	SourceType string    `json:"source_type"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Store struct {
	path string
	mu   sync.Mutex
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func DetectSourceType(source string) string {
	lines := strings.Fields(strings.TrimSpace(source))
	if len(lines) == 1 {
		lower := strings.ToLower(lines[0])
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
			return "url"
		}
	}
	return "raw"
}

func NewToken() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *Store) List() ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *Store) Get(token string) (Entry, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadLocked()
	if err != nil {
		return Entry{}, false, err
	}
	for _, entry := range entries {
		if entry.Token == token {
			return entry, true, nil
		}
	}
	return Entry{}, false, nil
}

func (s *Store) Save(entry Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadLocked()
	if err != nil {
		return err
	}
	replaced := false
	for i := range entries {
		if entries[i].Token == entry.Token {
			entries[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		entries = append(entries, entry)
	}
	return s.saveLocked(entries)
}

func (s *Store) Delete(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadLocked()
	if err != nil {
		return err
	}
	next := entries[:0]
	for _, entry := range entries {
		if entry.Token != token {
			next = append(next, entry)
		}
	}
	return s.saveLocked(next)
}

func (s *Store) loadLocked() ([]Entry, error) {
	if s.path == "" {
		return nil, errors.New("subscription store path is empty")
	}
	b, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return []Entry{}, nil
	}
	var entries []Entry
	if err := json.Unmarshal(b, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *Store) saveLocked(entries []Entry) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
