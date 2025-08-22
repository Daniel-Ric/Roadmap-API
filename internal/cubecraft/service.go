package cubecraft

import (
	"context"
	"roadmapapi/internal/hive"
	"sort"
	"strings"
	"sync"
	"time"
)

var columnToStatus = map[string][]string{
	"in-progress": {"In Progress"},
	"coming-next": {"Testing"},
	"released":    {"Released"},
}

func Columns() map[string]string {
	return map[string]string{
		"in-progress": "notion:in-progress",
		"coming-next": "notion:coming-next",
		"released":    "notion:released",
	}
}

type Service interface {
	Page(ctx context.Context, column string, page, limit int, sortBy string) (hive.RoadmapPage, error)
	All(ctx context.Context, column string, limit int, sortBy string) ([]hive.RoadmapPage, error)
	Columns() map[string]string
	Updates() []statusChange
}

type service struct {
	client     *Client
	mu         sync.Mutex
	prevStatus map[string]string
	updates    []statusChange
}

func NewService(c *Client) Service {
	return &service{
		client:     c,
		prevStatus: make(map[string]string),
		updates:    make([]statusChange, 0, 128),
	}
}

func (s *service) Columns() map[string]string { return Columns() }

func (s *service) Page(ctx context.Context, column string, page, limit int, sortBy string) (hive.RoadmapPage, error) {
	allPages, err := s.All(ctx, column, limit, sortBy)
	if err != nil {
		return hive.RoadmapPage{}, err
	}
	if page <= 0 {
		page = 1
	}
	if page > len(allPages) {
		return hive.RoadmapPage{
			Meta: hive.PageMeta{
				Page:         page,
				Limit:        limit,
				TotalPages:   len(allPages),
				TotalResults: 0,
			},
			Items: nil,
		}, nil
	}
	return allPages[page-1], nil
}

func (s *service) All(ctx context.Context, column string, limit int, sortBy string) ([]hive.RoadmapPage, error) {
	if limit <= 0 {
		limit = 10
	}
	cards, err := s.client.Fetch(ctx)
	if err != nil {
		return nil, err
	}

	targetStatuses, ok := columnToStatus[strings.ToLower(column)]
	if !ok {
		targetStatuses = nil
	}

	items := make([]item, 0, len(cards))
	for _, c := range cards {
		props := renameMap(c.Properties)
		status := props["status"]
		if !contains(targetStatuses, status) {
			continue
		}
		createdAt := time.Unix(c.CreatedAt/1000, 0)
		updatedAt := time.Unix(c.UpdatedAt/1000, 0)
		var releasedAt time.Time
		if props["releasedAt"] != "" {
			if t, err := time.Parse("2006-01-02", props["releasedAt"]); err == nil {
				releasedAt = t
			}
		}
		items = append(items, item{
			ID:          c.ID,
			Slug:        c.ID,
			Title:       c.Title,
			Status:      mapStatusLabel(status),
			Category:    props["category"],
			Network:     props["network"],
			ProjectLead: props["projectLead"],
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
			ReleasedAt:  releasedAt,
			URL:         c.URL,
			ContentHTML: "",
			ContentText: "",
		})
	}

	sortBy = normalizeSort(sortBy, column)
	sort.SliceStable(items, func(i, j int) bool {
		switch sortBy {
		case "releasedat:asc":
			return items[i].ReleasedAt.Before(items[j].ReleasedAt)
		case "releasedat:desc":
			return items[i].ReleasedAt.After(items[j].ReleasedAt)
		case "lastupdated:asc":
			return items[i].UpdatedAt.Before(items[j].UpdatedAt)
		case "lastupdated:desc":
			return items[i].UpdatedAt.After(items[j].UpdatedAt)
		case "createdat:asc":
			return items[i].CreatedAt.Before(items[j].CreatedAt)
		case "createdat:desc":
			return items[i].CreatedAt.After(items[j].CreatedAt)
		case "title:desc":
			return strings.ToLower(items[i].Title) > strings.ToLower(items[j].Title)
		default:
			return strings.ToLower(items[i].Title) < strings.ToLower(items[j].Title)
		}
	})

	total := len(items)
	if total == 0 {
		return []hive.RoadmapPage{
			{Meta: hive.PageMeta{Page: 1, Limit: limit, TotalPages: 1, TotalResults: 0}},
		}, nil
	}

	s.recordStatusChanges(items)

	pages := make([]hive.RoadmapPage, 0, (total+limit-1)/limit)
	for p, offset := 1, 0; offset < total; p, offset = p+1, offset+limit {
		end := offset + limit
		if end > total {
			end = total
		}
		pageItems := items[offset:end]
		dto := make([]hive.RoadmapItem, 0, len(pageItems))
		for _, it := range pageItems {
			dto = append(dto, hive.RoadmapItem{
				ID:           it.ID,
				Slug:         it.Slug,
				Title:        it.Title,
				Status:       it.Status,
				Category:     it.Category,
				Upvotes:      0,
				Date:         it.CreatedAt.Format(time.RFC3339),
				LastModified: it.UpdatedAt.Format(time.RFC3339),
				ETA:          isoOrEmpty(it.ReleasedAt),
				Network:      it.Network,
				ProjectLead:  it.ProjectLead,
				URL:          it.URL,
				ContentHTML:  it.ContentHTML,
				ContentText:  it.ContentText,
				Page:         p,
			})
		}
		pages = append(pages, hive.RoadmapPage{
			Meta: hive.PageMeta{
				Page:         p,
				Limit:        limit,
				TotalPages:   (total + limit - 1) / limit,
				TotalResults: total,
			},
			Items: dto,
		})
	}
	return pages, nil
}

func (s *service) recordStatusChanges(items []item) {
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
			s.updates = append(s.updates, statusChange{
				At:   now,
				From: prev,
				To:   it.Status,
				Item: it,
			})
			s.prevStatus[it.ID] = it.Status
		}
	}
}

func (s *service) Updates() []statusChange {
	keepAfter := time.Now().Add(-24 * time.Hour)
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]statusChange, 0, len(s.updates))
	for _, u := range s.updates {
		if u.At.After(keepAfter) {
			out = append(out, u)
		}
	}
	return out
}

func contains(arr []string, v string) bool {
	if len(arr) == 0 {
		return true
	}
	for _, s := range arr {
		if s == v {
			return true
		}
	}
	return false
}

func isoOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func normalizeSort(in, column string) string {
	in = strings.ToLower(strings.TrimSpace(in))
	switch in {
	case "releasedat:asc", "releasedat:desc",
		"lastupdated:asc", "lastupdated:desc",
		"createdat:asc", "createdat:desc",
		"title:asc", "title:desc":
		return in
	}
	switch strings.ToLower(column) {
	case "released":
		return "releasedat:desc"
	case "in-progress", "coming-next":
		return "lastupdated:desc"
	default:
		return "title:asc"
	}
}

func mapStatusLabel(notionStatus string) string {
	switch notionStatus {
	case "In Progress":
		return "In Progress"
	case "Released":
		return "Released"
	case "Testing":
		return "Coming Next..."
	case "Information":
		return "Information"
	default:
		return notionStatus
	}
}
