package main

import (
	"context"
	"sync"
)

// MemoryStore keeps links in a map. Data is lost when the program stops.
type MemoryStore struct {
	mu    sync.Mutex
	links map[string]Link
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{links: make(map[string]Link)}
}

func (s *MemoryStore) Save(ctx context.Context, link Link) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.links[link.Code]; exists {
		return ErrCodeTaken
	}
	s.links[link.Code] = link
	return nil
}

func (s *MemoryStore) Get(ctx context.Context, code string) (Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, ok := s.links[code]
	if !ok {
		return Link{}, ErrNotFound
	}
	return link, nil
}

func (s *MemoryStore) Visit(ctx context.Context, code string) (Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, ok := s.links[code]
	if !ok {
		return Link{}, ErrNotFound
	}
	link.Clicks++
	s.links[code] = link
	return link, nil
}

func (s *MemoryStore) Delete(ctx context.Context, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.links[code]; !ok {
		return ErrNotFound
	}
	delete(s.links, code)
	return nil
}