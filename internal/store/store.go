package store

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/clover-eric/ato-cfip/internal/model"
)

type Store struct {
	path string
}

type Cycle struct {
	StartedAt time.Time      `json:"started_at"`
	CreatedAt time.Time      `json:"created_at"`
	All       []model.Result `json:"all_results"`
	Published []model.Result `json:"published"`
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			return nil, err
		}
	}
	return &Store{path: path}, nil
}

func (s *Store) Close() error {
	return nil
}

func (s *Store) SaveCycle(ctx context.Context, started time.Time, all, published []model.Result) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	row := Cycle{
		StartedAt: started,
		CreatedAt: time.Now(),
		All:       all,
		Published: published,
	}
	b, err := json.Marshal(row)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *Store) Prune(ctx context.Context, keepDays int) error {
	if keepDays <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	cutoff := time.Now().AddDate(0, 0, -keepDays)
	kept := make([][]byte, 0)
	if err := func() error {
		f, err := os.Open(s.path)
		if err != nil {
			return err
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Bytes()
			var cycle Cycle
			if err := json.Unmarshal(line, &cycle); err != nil {
				continue
			}
			if cycle.StartedAt.After(cutoff) {
				cp := make([]byte, len(line))
				copy(cp, line)
				kept = append(kept, cp)
			}
		}
		return scanner.Err()
	}(); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	for _, line := range kept {
		if _, err := out.Write(append(line, '\n')); err != nil {
			_ = out.Close()
			return err
		}
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) LastCycle() (*Cycle, error) {
	f, err := os.Open(s.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var last []byte
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		last = append(last[:0], line...)
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return nil, err
	}
	if len(last) == 0 {
		return nil, nil
	}
	var cycle Cycle
	if err := json.Unmarshal(last, &cycle); err != nil {
		return nil, err
	}
	return &cycle, nil
}
