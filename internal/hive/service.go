package hive

import (
	"context"
	"sync"
	"time"
)

type Service interface {
	GetPage(ctx context.Context, q Query) (RoadmapPage, []byte, error)
	GetAll(ctx context.Context, q Query) ([]RoadmapPage, error)
	GetColumns() map[string]string
	Updates() []changeEntry
}

type changeEntry struct {
	At   time.Time
	From string
	To   string
	Item RoadmapItem
}

type service struct {
	client     *Client
	mu         sync.Mutex
	prevStatus map[string]string
	updates    []changeEntry
}

func NewService(c *Client) Service {
	return &service{
		client:     c,
		prevStatus: make(map[string]string),
		updates:    make([]changeEntry, 0, 128),
	}
}

func (s *service) GetPage(ctx context.Context, q Query) (RoadmapPage, []byte, error) {
	hr, raw, err := s.client.FetchPage(ctx, q)
	if err != nil {
		return RoadmapPage{}, nil, err
	}
	page := MapResponse(hr)
	s.recordChanges(page.Items)
	return page, raw, nil
}

func (s *service) GetAll(ctx context.Context, q Query) ([]RoadmapPage, error) {
	all, err := s.client.FetchAllPages(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]RoadmapPage, 0, len(all))
	collected := make([]RoadmapItem, 0, 256)
	for _, hr := range all {
		m := MapResponse(hr)
		out = append(out, m)
		collected = append(collected, m.Items...)
	}
	s.recordChanges(collected)
	return out, nil
}

func (s *service) GetColumns() map[string]string {
	return s.client.Columns()
}

func (s *service) recordChanges(items []RoadmapItem) {
	now := time.Now()
	keepAfter := now.Add(-24 * time.Hour)
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := s.updates[:0]
	for _, u := range s.updates {
		if u.At.After(keepAfter) {
			filtered = append(filtered, u)
		}
	}
	s.updates = filtered
	for _, it := range items {
		prev, ok := s.prevStatus[it.ID]
		if !ok {
			s.prevStatus[it.ID] = it.Status
			continue
		}
		if prev != it.Status {
			s.updates = append(s.updates, changeEntry{
				At:   now,
				From: prev,
				To:   it.Status,
				Item: it,
			})
			s.prevStatus[it.ID] = it.Status
		}
	}
}

func (s *service) Updates() []changeEntry {
	keepAfter := time.Now().Add(-24 * time.Hour)
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]changeEntry, 0, len(s.updates))
	for _, u := range s.updates {
		if u.At.After(keepAfter) {
			out = append(out, u)
		}
	}
	return out
}
