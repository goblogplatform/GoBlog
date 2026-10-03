package blog

import "time"

// PostType defines different kinds of content (e.g. Posts, Notes, Links)
type PostType struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `sql:"index" json:"deleted_at,omitempty"`
	Name        string     `json:"name"`
	Slug        string     `gorm:"uniqueIndex:,length:191" json:"slug"` // length: see Page.Slug
	Description string     `sql:"type:text;" json:"description"`
}

// Permalink returns the URL path for this post type's listing page
func (pt PostType) Permalink() string {
	return "/" + pt.Slug
}
