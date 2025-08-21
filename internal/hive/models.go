package hive

type hiveResponse struct {
	Results      []hiveSubmission `json:"results"`
	Page         int              `json:"page"`
	Limit        int              `json:"limit"`
	TotalPages   int              `json:"totalPages"`
	TotalResults int              `json:"totalResults"`
}

type hiveSubmission struct {
	ID           string         `json:"id"`
	Slug         string         `json:"slug"`
	Title        string         `json:"title"`
	ContentHTML  string         `json:"content" jsonschema:"description=HTML content"`
	Upvotes      int            `json:"upvotes"`
	Date         string         `json:"date"`
	LastModified string         `json:"lastModified"`
	Pinned       bool           `json:"pinned"`
	Eta          *string        `json:"eta,omitempty"`
	PostStatus   *postStatus    `json:"postStatus,omitempty"`
	PostCategory *postCategory  `json:"postCategory,omitempty"`
	Translations map[string]any `json:"contentTranslations,omitempty"`
	User         map[string]any `json:"user,omitempty"`
}

type postStatus struct {
	Name string `json:"name"`
}

type postCategory struct {
	Name map[string]string `json:"name"`
}

type RoadmapItem struct {
	ID           string `json:"id"`
	Slug         string `json:"slug"`
	Title        string `json:"title"`
	Status       string `json:"status"`
	Category     string `json:"category"`
	Upvotes      int    `json:"upvotes"`
	Date         string `json:"date"`
	LastModified string `json:"lastModified"`
	Pinned       bool   `json:"pinned"`
	ETA          string `json:"eta,omitempty"`
	ContentHTML  string `json:"contentHtml"`
	ContentText  string `json:"contentText"`
	Page         int    `json:"page"`
	Network      string `json:"network,omitempty"`
	ProjectLead  string `json:"projectLead,omitempty"`
	URL          string `json:"url,omitempty"`
}

type PageMeta struct {
	Page         int `json:"page"`
	Limit        int `json:"limit"`
	TotalPages   int `json:"totalPages"`
	TotalResults int `json:"totalResults"`
}

type RoadmapPage struct {
	Meta  PageMeta      `json:"meta"`
	Items []RoadmapItem `json:"items"`
}

type RoadmapAggregate struct {
	Column string        `json:"column"`
	Pages  []RoadmapPage `json:"pages"`
}

type StatusChange struct {
	At   int64       `json:"at"`
	From string      `json:"from"`
	To   string      `json:"to"`
	Item RoadmapItem `json:"item"`
}
