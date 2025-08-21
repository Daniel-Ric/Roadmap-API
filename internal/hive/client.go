package hive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const DefaultBaseURL = "https://updates.playhive.com/api/v1/submission"

var columnToStatusID = map[string]string{
	"in-progress": "673d43b2b479f2dff6f8b96e",
	"coming-next": "673d43a8b479f2dff6f8b74b",
	"released":    "67489361029fa6e5e4579b21",
}

type ClientOption func(*Client)

func WithCacheTTL(ttl time.Duration) ClientOption {
	return func(c *Client) { c.cacheTTL = ttl }
}

func WithMaxConcurrency(n int) ClientOption {
	return func(c *Client) {
		if n < 1 {
			n = 1
		}
		c.maxConcurrency = n
	}
}

type cacheEntry struct {
	body      []byte
	expiresAt time.Time
}

type Client struct {
	baseURL        string
	httpClient     *http.Client
	cache          sync.Map
	cacheTTL       time.Duration
	maxConcurrency int
}

func NewClient(baseURL string, hc *http.Client, opts ...ClientOption) *Client {
	c := &Client{
		baseURL:        baseURL,
		httpClient:     hc,
		cacheTTL:       0,
		maxConcurrency: 2,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

type Query struct {
	Column        string
	Page          int
	SortBy        string
	InReview      bool
	IncludePinned bool
	Raw           bool
	BypassCache   bool
}

func (q *Query) statusID() (string, error) {
	id, ok := columnToStatusID[strings.ToLower(q.Column)]
	if !ok {
		return "", fmt.Errorf("unknown column: %s", q.Column)
	}
	return id, nil
}

func (c *Client) buildURL(q Query) (string, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.SortBy == "" {
		q.SortBy = "upvotes:desc"
	}
	id, err := q.statusID()
	if err != nil {
		return "", err
	}
	u, _ := url.Parse(c.baseURL)
	params := url.Values{}
	params.Set("s", id)
	params.Set("sortBy", q.SortBy)
	params.Set("inReview", strconv.FormatBool(q.InReview))
	params.Set("includePinned", strconv.FormatBool(q.IncludePinned))
	params.Set("page", strconv.Itoa(q.Page))
	u.RawQuery = params.Encode()
	return u.String(), nil
}

func (c *Client) get(ctx context.Context, fullURL string, bypassCache bool) ([]byte, error) {
	if !bypassCache && c.cacheTTL > 0 {
		if v, ok := c.cache.Load(fullURL); ok {
			entry := v.(cacheEntry)
			if time.Now().Before(entry.expiresAt) {
				return entry.body, nil
			}
			c.cache.Delete(fullURL)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		return nil, fmt.Errorf("upstream status %d: %s", resp.StatusCode, string(b))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if c.cacheTTL > 0 && !bypassCache {
		c.cache.Store(fullURL, cacheEntry{
			body:      body,
			expiresAt: time.Now().Add(c.cacheTTL),
		})
	}
	return body, nil
}

func (c *Client) FetchPage(ctx context.Context, q Query) (hiveResponse, []byte, error) {
	u, err := c.buildURL(q)
	if err != nil {
		return hiveResponse{}, nil, err
	}
	raw, err := c.get(ctx, u, q.BypassCache)
	if err != nil {
		return hiveResponse{}, nil, err
	}
	var hr hiveResponse
	if err := json.Unmarshal(raw, &hr); err != nil {
		return hiveResponse{}, raw, err
	}
	return hr, raw, nil
}

func (c *Client) FetchAllPages(ctx context.Context, base Query) ([]hiveResponse, error) {
	first, _, err := c.FetchPage(ctx, base)
	if err != nil {
		return nil, err
	}
	total := first.TotalPages
	if total == 0 {
		return []hiveResponse{first}, nil
	}
	results := make([]hiveResponse, total)
	results[0] = first

	type job struct{ page int }
	jobs := make(chan job)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			q := base
			q.Page = j.page
			hr, _, err := c.FetchPage(ctx, q)
			mu.Lock()
			if err != nil && firstErr == nil {
				firstErr = err
			} else if err == nil {
				results[j.page-1] = hr
			}
			mu.Unlock()
		}
	}

	workers := c.maxConcurrency
	if workers > total {
		workers = total
	}
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go worker()
	}

	for p := 2; p <= total; p++ {
		jobs <- job{page: p}
	}
	close(jobs)
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	return results, nil
}

func columnMap() map[string]string {
	out := make(map[string]string, len(columnToStatusID))
	for k, v := range columnToStatusID {
		out[k] = v
	}
	return out
}

func (c *Client) Columns() map[string]string { return columnMap() }

func stripHTML(input string) string {
	var b strings.Builder
	inTag := false
	for _, r := range input {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				b.WriteRune(r)
			}
		}
	}
	s := b.String()
	replacer := strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&#39;", "'", "&quot;", `"`)
	return strings.TrimSpace(replacer.Replace(s))
}

func MapResponse(hr hiveResponse) RoadmapPage {
	items := make([]RoadmapItem, 0, len(hr.Results))
	for _, s := range hr.Results {
		status := ""
		if s.PostStatus != nil {
			status = s.PostStatus.Name
		}
		cat := ""
		if s.PostCategory != nil {
			if name, ok := s.PostCategory.Name["en"]; ok {
				cat = name
			}
		}
		eta := ""
		if s.Eta != nil {
			eta = *s.Eta
		}
		items = append(items, RoadmapItem{
			ID:           s.ID,
			Slug:         s.Slug,
			Title:        s.Title,
			Status:       status,
			Category:     cat,
			Upvotes:      s.Upvotes,
			Date:         s.Date,
			LastModified: s.LastModified,
			Pinned:       s.Pinned,
			ETA:          eta,
			ContentHTML:  s.ContentHTML,
			ContentText:  stripHTML(s.ContentHTML),
			Page:         hr.Page,
		})
	}
	return RoadmapPage{
		Meta: PageMeta{
			Page:         hr.Page,
			Limit:        hr.Limit,
			TotalPages:   hr.TotalPages,
			TotalResults: hr.TotalResults,
		},
		Items: items,
	}
}

func ValidateColumn(col string) error {
	_, ok := columnToStatusID[strings.ToLower(col)]
	if !ok {
		return errors.New("column must be one of [in-progress, coming-next, released]")
	}
	return nil
}
