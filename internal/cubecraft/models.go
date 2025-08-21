package cubecraft

import "time"

type Card struct {
	ID         string            `json:"id"`
	Title      string            `json:"title"`
	URL        string            `json:"url"`
	Properties map[string]string `json:"properties"`
	CreatedAt  int64             `json:"createdAt"`
	UpdatedAt  int64             `json:"lastUpdated"`
	ReleasedAt string            `json:"releasedAt"`
}

type item struct {
	ID          string
	Slug        string
	Title       string
	Status      string
	Category    string
	Network     string
	ProjectLead string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ReleasedAt  time.Time
	URL         string
	ContentHTML string
	ContentText string
}

type statusChange struct {
	At   time.Time
	From string
	To   string
	Item item
}
