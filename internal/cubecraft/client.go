package cubecraft

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	notionAPIURL      = "https://cubecraft.notion.site/api/v3/queryCollection?src=initial_load"
	notionSiteBaseURL = "https://cubecraft.notion.site/e86c96a3ee78465d8e5c24c22489c094"
	collectionViewID  = "79bd3042-c1cf-42aa-9d4e-d81a3043c505"
	clientTimeout     = 30 * time.Second
	defaultPageSize   = 10
)

var notionQueryPayload = []byte(`{
  "source": {
    "type": "collection",
    "id": "d14e867c-526a-4627-ad4f-1f56fdee77d6",
    "spaceId": "2a7d9973-2a91-430b-9d0f-520163f17777"
  },
  "collectionView": {
    "id": "79bd3042-c1cf-42aa-9d4e-d81a3043c505",
    "spaceId": "2a7d9973-2a91-430b-9d0f-520163f17777"
  },
  "loader": {
    "reducers": {
      "board_columns": {
        "type": "groups",
        "version": "v2",
        "returnPinnedGroups": true,
        "groupBy": { "sort": { "type": "manual" }, "type": "select", "property": "3E6J" },
        "groupSortPreference": [
          { "value": { "type": "select", "value": "Information" }, "hidden": false, "property": "3E6J" },
          { "value": { "type": "select", "value": "In Progress" }, "hidden": false, "property": "3E6J" },
          { "value": { "type": "select", "value": "Testing" }, "hidden": false, "property": "3E6J" },
          { "value": { "type": "select", "value": "Released" }, "hidden": false, "property": "3E6J" },
          { "value": { "type": "select", "value": "Scrapped" }, "hidden": true,  "property": "3E6J" },
          { "value": { "type": "select", "value": "BLOCKED" },  "hidden": true,  "property": "3E6J" },
          { "value": { "type": "select" }, "hidden": true, "property": "3E6J" }
        ],
        "limit": 10,
        "aggregation": { "type": "independent", "groupAggregation": { "aggregator": "count" } },
        "blockResults": { "type": "independent", "defaultLimit": 50, "loadContentCover": false, "groupOverrides": {} }
      }
    },
    "sort": [ { "property": "?igY", "direction": "descending" } ],
    "searchQuery": "",
    "userTimeZone": "Europe/Berlin"
  }
}`)

type ClientOption func(*Client)

func WithCacheTTL(ttl time.Duration) ClientOption {
	return func(c *Client) { c.cacheTTL = ttl }
}

type cacheEntry struct {
	data      []Card
	expiresAt time.Time
}

type Client struct {
	httpClient *http.Client
	cacheTTL   time.Duration
	mu         sync.RWMutex
	cache      *cacheEntry
}

func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: clientTimeout},
		cacheTTL:   0,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *Client) Fetch(ctx context.Context) ([]Card, error) {
	if c.cacheTTL > 0 {
		c.mu.RLock()
		if c.cache != nil && time.Now().Before(c.cache.expiresAt) {
			data := c.cache.data
			c.mu.RUnlock()
			return data, nil
		}
		c.mu.RUnlock()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, notionAPIURL, bytes.NewReader(notionQueryPayload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", os.Getenv("NOTION_COOKIE"))
	req.Header.Set("x-notion-space-id", "2a7d9973-2a91-430b-9d0f-520163f17777")
	req.Header.Set("x-notion-active-user-header", "")
	req.Header.Set("notion-client-version", "23.13.0.5155")
	req.Header.Set("notion-audit-log-platform", "web")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}

	var full struct {
		RecordMap struct {
			Block map[string]json.RawMessage `json:"block"`
		} `json:"recordMap"`
	}
	if err := json.Unmarshal(b, &full); err != nil {
		return nil, err
	}

	type rawBlock struct {
		Value struct {
			ParentTable    string                     `json:"parent_table"`
			Properties     map[string]json.RawMessage `json:"properties"`
			CreatedTime    int64                      `json:"created_time"`
			LastEditedTime int64                      `json:"last_edited_time"`
		} `json:"value"`
	}

	parseProps := func(rb rawBlock) (title string, props map[string]string, created int64, updated int64, releasedAt string) {
		created = rb.Value.CreatedTime
		updated = rb.Value.LastEditedTime
		props = make(map[string]string)

		for k, rawv := range rb.Value.Properties {
			var arr [][]any
			if err := json.Unmarshal(rawv, &arr); err != nil || len(arr) == 0 {
				continue
			}
			cell := arr[0]

			if len(cell) > 1 {
				if dateCells, ok := cell[1].([]any); ok && len(dateCells) > 0 {
					if pair, ok2 := dateCells[0].([]any); ok2 && len(pair) > 1 {
						if dm, ok3 := pair[1].(map[string]any); ok3 {
							if ds, ok4 := dm["start_date"].(string); ok4 {
								props[k] = ds
								if k == "?igY" {
									releasedAt = ds
								}
								continue
							}
						}
					}
				}
			}

			if text, ok := cell[0].(string); ok {
				props[k] = text
				if k == "title" {
					title = text
				}
			}
		}
		return
	}

	cards := make([]Card, 0, 256)
	for id, raw := range full.RecordMap.Block {
		var rb rawBlock
		if err := json.Unmarshal(raw, &rb); err != nil {
			continue
		}
		if rb.Value.ParentTable != "collection" {
			continue
		}
		title, props, createdAt, updatedAt, releasedAt := parseProps(rb)
		cleanViewID := strings.ReplaceAll(collectionViewID, "-", "")
		cleanPageID := strings.ReplaceAll(id, "-", "")
		url := fmt.Sprintf("%s?v=%s&p=%s&pm=s", notionSiteBaseURL, cleanViewID, cleanPageID)
		cards = append(cards, Card{
			ID:         id,
			Title:      title,
			URL:        url,
			Properties: props,
			CreatedAt:  createdAt,
			UpdatedAt:  updatedAt,
			ReleasedAt: releasedAt,
		})
	}

	if c.cacheTTL > 0 {
		c.mu.Lock()
		c.cache = &cacheEntry{data: cards, expiresAt: time.Now().Add(c.cacheTTL)}
		c.mu.Unlock()
	}
	return cards, nil
}

func (c *Client) Probe(ctx context.Context) (int, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, notionAPIURL, bytes.NewReader(notionQueryPayload))
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", os.Getenv("NOTION_COOKIE"))
	req.Header.Set("x-notion-space-id", "2a7d9973-2a91-430b-9d0f-520163f17777")
	req.Header.Set("x-notion-active-user-header", "")
	req.Header.Set("notion-client-version", "23.13.0.4147")
	req.Header.Set("notion-audit-log-platform", "web")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	status := resp.StatusCode
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return status, 0, err
	}

	var full struct {
		RecordMap struct {
			Block map[string]json.RawMessage `json:"block"`
		} `json:"recordMap"`
	}
	if err := json.Unmarshal(b, &full); err != nil {
		return status, 0, err
	}

	type rawBlock struct {
		Value struct {
			ParentTable string `json:"parent_table"`
		} `json:"value"`
	}
	count := 0
	for _, raw := range full.RecordMap.Block {
		var rb rawBlock
		if err := json.Unmarshal(raw, &rb); err != nil {
			continue
		}
		if rb.Value.ParentTable == "collection" {
			count++
		}
	}
	if status >= 400 {
		return status, count, fmt.Errorf("notion status %d", status)
	}
	return status, count, nil
}

var keyMap = map[string]string{
	"3E6J":             "status",
	"@@W>":             "network",
	"K:rY":             "category",
	"\\wxR":            "projectLead",
	"^NHI":             "releasePost",
	"created_time":     "createdAt",
	"last_edited_time": "lastUpdated",
	"?igY":             "releasedAt",
}

func renameMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		if nk, ok := keyMap[k]; ok {
			out[nk] = v
		} else {
			out[k] = v
		}
	}
	return out
}
