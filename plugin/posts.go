package plugin

import (
	"strings"
	"time"

	"github.com/goblogplatform/goblog/blog"
)

// Post is a published post as a plugin sees it through the posts_list host
// function. Its JSON field names are part of the wasm contract.
type Post struct {
	ID        uint      `json:"id"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`     // absolute under site_url; site-relative when site_url is unset
	Content   string    `json:"content"` // the post's Markdown source
	Tags      []string  `json:"tags"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MaxPostsList caps how many posts one posts_list call returns.
const MaxPostsList = 50

// Posts returns the newest published posts, newest first: at most limit,
// clamped to 1..MaxPostsList. Drafts are never included.
func (r *Registry) Posts(limit int) ([]Post, error) {
	db, err := (&dbStore{r: r}).db()
	if err != nil {
		return nil, err
	}
	limit = max(1, min(limit, MaxPostsList))
	var site blog.Setting
	db.Where(map[string]any{"key": "site_url"}).Limit(1).Find(&site)
	base := strings.TrimRight(strings.TrimSpace(site.Value), "/")
	var posts []blog.Post
	if err := db.Preload("Tags").Preload("PostType").Where("draft = ?", false).Order("created_at desc").Limit(limit).Find(&posts).Error; err != nil {
		return nil, err
	}
	out := make([]Post, 0, len(posts))
	for _, p := range posts {
		tags := make([]string, 0, len(p.Tags))
		for _, t := range p.Tags {
			tags = append(tags, t.Name)
		}
		out = append(out, Post{ID: p.ID, Title: p.Title, URL: base + p.Permalink(), Content: p.Content, Tags: tags, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt})
	}
	return out, nil
}
