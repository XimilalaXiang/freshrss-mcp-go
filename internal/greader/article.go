package greader

import (
	"encoding/json"
	"fmt"
	"time"
)

// Article is a normalized item for MCP JSON output (string id preserved).
type Article struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	URL       string   `json:"url,omitempty"`
	Content   string   `json:"content,omitempty"`
	Summary   string   `json:"summary,omitempty"`
	Author    string   `json:"author,omitempty"`
	Published string   `json:"published,omitempty"`
	FeedTitle string   `json:"feed_title,omitempty"`
	FeedID    string   `json:"feed_id,omitempty"`
	IsRead    bool     `json:"is_read"`
	IsStarred bool     `json:"is_starred"`
	Labels    []string `json:"labels,omitempty"`
}

// ParseArticle maps one stream item from the Reader JSON API.
func ParseArticle(item map[string]any) (Article, error) {
	id, _ := item["id"].(string)
	if id == "" {
		return Article{}, fmt.Errorf("missing id")
	}
	title, _ := item["title"].(string)

	var published string
	if p, ok := item["published"].(float64); ok {
		published = time.Unix(int64(p), 0).UTC().Format(time.RFC3339)
	}

	var author string
	if a, ok := item["author"].(string); ok {
		author = a
	}

	content := ""
	if c, ok := item["content"].(map[string]any); ok {
		if s, ok := c["content"].(string); ok {
			content = s
		}
	}
	summary := ""
	if s, ok := item["summary"].(map[string]any); ok {
		if s2, ok := s["content"].(string); ok {
			summary = s2
		}
	}

	url := ""
	if alts, ok := item["alternate"].([]any); ok {
		for _, a := range alts {
			if m, ok := a.(map[string]any); ok {
				if m["type"] == "text/html" {
					if h, ok := m["href"].(string); ok {
						url = h
						break
					}
				}
			}
		}
	}

	var cats []string
	if c, ok := item["categories"].([]any); ok {
		for _, x := range c {
			if s, ok := x.(string); ok {
				cats = append(cats, s)
			}
		}
	}

	isRead := false
	isStarred := false
	var labels []string
	for _, c := range cats {
		switch c {
		case "user/-/state/com.google/read":
			isRead = true
		case "user/-/state/com.google/starred":
			isStarred = true
		default:
			if len(c) > 16 && c[:16] == "user/-/label/" {
				labels = append(labels, c)
			}
		}
	}

	feedTitle := ""
	feedID := ""
	if o, ok := item["origin"].(map[string]any); ok {
		if t, ok := o["title"].(string); ok {
			feedTitle = t
		}
		if sid, ok := o["streamId"].(string); ok {
			feedID = sid
		}
	}

	return Article{
		ID:        id,
		Title:     title,
		URL:       url,
		Content:   content,
		Summary:   summary,
		Author:    author,
		Published: published,
		FeedTitle: feedTitle,
		FeedID:    feedID,
		IsRead:    isRead,
		IsStarred: isStarred,
		Labels:    labels,
	}, nil
}

// ArticlesToJSON encodes a slice for tool output.
func ArticlesToJSON(articles []Article) (string, error) {
	b, err := json.MarshalIndent(articles, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
