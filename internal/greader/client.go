// Package greader implements FreshRSS Google Reader HTTP API (greader.php).
package greader

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to FreshRSS greader.php.
type Client struct {
	baseURL    string
	apiURL     string
	httpClient *http.Client
	email      string
	password   string
	authToken  string
	editToken  string
}

// New creates a client. baseURL is e.g. https://rss.example.com (no trailing path).
func New(baseURL, email, password, apiPath string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	if apiPath == "" {
		apiPath = "/api/greader.php"
	}
	apiPath = strings.TrimRight(apiPath, "/")
	return &Client{
		baseURL: baseURL,
		apiURL:  baseURL + apiPath,
		httpClient: &http.Client{
			Timeout: 45 * time.Second,
		},
		email:    email,
		password: password,
	}
}

// Authenticate uses ClientLogin and stores the Auth token (Google Reader style).
func (c *Client) Authenticate() error {
	form := url.Values{}
	form.Set("Email", c.email)
	form.Set("Passwd", c.password)
	req, err := http.NewRequest(http.MethodPost, c.apiURL+"/accounts/ClientLogin", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("authentication request failed (check FRESHRSS_URL): %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("authentication failed (HTTP %d): check FRESHRSS_EMAIL and FRESHRSS_API_PASSWORD", resp.StatusCode)
	}

	var auth string
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Auth=") {
			auth = strings.TrimPrefix(line, "Auth=")
			break
		}
	}
	if auth == "" {
		return fmt.Errorf("authentication failed: server did not return auth token (check API password in FreshRSS settings)")
	}
	c.authToken = auth
	return nil
}

func (c *Client) authHeader() http.Header {
	h := make(http.Header)
	h.Set("Authorization", "GoogleLogin auth="+c.authToken)
	return h
}

// EditToken fetches the token required for write operations (cached).
func (c *Client) EditToken() (string, error) {
	if c.editToken != "" {
		return c.editToken, nil
	}
	req, err := http.NewRequest(http.MethodGet, c.apiURL+"/reader/api/0/token", nil)
	if err != nil {
		return "", err
	}
	req.Header = c.authHeader()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("failed to get edit token (HTTP %d): write operations unavailable", resp.StatusCode)
	}
	c.editToken = strings.TrimSpace(string(b))
	return c.editToken, nil
}

func (c *Client) reauth() error {
	c.authToken = ""
	c.editToken = ""
	return c.Authenticate()
}

func (c *Client) getJSON(pathSuffix string, query url.Values, target any) error {
	b, err := c.getJSONRaw(pathSuffix, query)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, target)
}

func (c *Client) getJSONRaw(pathSuffix string, query url.Values) ([]byte, error) {
	if c.authToken == "" {
		return nil, fmt.Errorf("not authenticated: call Authenticate() first or check credentials")
	}
	full := c.apiURL + pathSuffix
	u, err := url.Parse(full)
	if err != nil {
		return nil, err
	}
	if query != nil {
		u.RawQuery = query.Encode()
	}

	doOnce := func() (*http.Response, error) {
		req, err := http.NewRequest(http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header = c.authHeader()
		return c.httpClient.Do(req)
	}

	resp, err := doOnce()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		resp.Body.Close()
		if rerr := c.reauth(); rerr != nil {
			return nil, fmt.Errorf("re-auth failed: %w", rerr)
		}
		resp, err = doOnce()
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GET %s: HTTP %d: %s", pathSuffix, resp.StatusCode, string(b))
	}
	return b, nil
}

// SubscriptionListJSON is the raw /subscription/list response.
type SubscriptionListJSON struct {
	Subscriptions []struct {
		ID         string `json:"id"`
		Title      string `json:"title"`
		URL        string `json:"url"`
		HtmlURL    string `json:"htmlUrl"`
		IconURL    string `json:"iconUrl"`
		Categories []any  `json:"categories"`
	} `json:"subscriptions"`
}

// ListSubscriptions returns subscribed feeds.
func (c *Client) ListSubscriptions() (*SubscriptionListJSON, error) {
	var out SubscriptionListJSON
	q := url.Values{}
	q.Set("output", "json")
	if err := c.getJSON("/reader/api/0/subscription/list", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TagListJSON is /tag/list.
type TagListJSON struct {
	Tags []struct {
		ID   string `json:"id"`
		Sort string `json:"sortid"`
	} `json:"tags"`
}

// ListTags returns folders/labels.
func (c *Client) ListTags() (*TagListJSON, error) {
	var out TagListJSON
	q := url.Values{}
	q.Set("output", "json")
	if err := c.getJSON("/reader/api/0/tag/list", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UnreadCountJSON is /unread-count.
type UnreadCountJSON struct {
	UnreadCounts []struct {
		ID    string `json:"id"`
		Count int    `json:"count"`
	} `json:"unreadcounts"`
}

// UnreadCounts returns per-stream unread stats.
func (c *Client) UnreadCounts() (*UnreadCountJSON, error) {
	var out UnreadCountJSON
	q := url.Values{}
	q.Set("output", "json")
	if err := c.getJSON("/reader/api/0/unread-count", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// StreamContentsJSON is /stream/contents/{stream}.
type StreamContentsJSON struct {
	Items        []map[string]any `json:"items"`
	Continuation *string          `json:"continuation"`
}

// GetStream fetches articles for a stream id.
func (c *Client) GetStream(streamID string, n int, order string, excludeRead bool, continuation string) (*StreamContentsJSON, error) {
	if order == "" {
		order = "d"
	}
	enc := url.PathEscape(streamID)
	suffix := "/reader/api/0/stream/contents/" + enc

	q := url.Values{}
	q.Set("output", "json")
	q.Set("n", fmt.Sprintf("%d", n))
	q.Set("r", order)
	if excludeRead {
		q.Set("xt", "user/-/state/com.google/read")
	}
	if continuation != "" {
		q.Set("c", continuation)
	}

	var out StreamContentsJSON
	if err := c.getJSON(suffix, q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// EditTag applies tag changes using T + i + a/r (FreshRSS / Google Reader).
// Retries once on 401 by re-authenticating and refreshing the edit token.
func (c *Client) EditTag(itemIDs []string, addTags, removeTags []string) error {
	doOnce := func() (int, []byte, error) {
		tok, err := c.EditToken()
		if err != nil {
			return 0, nil, err
		}
		form := url.Values{}
		form.Set("T", tok)
		for _, t := range addTags {
			form.Add("a", t)
		}
		for _, t := range removeTags {
			form.Add("r", t)
		}
		for _, id := range itemIDs {
			form.Add("i", id)
		}
		req, err := http.NewRequest(http.MethodPost, c.apiURL+"/reader/api/0/edit-tag", bytes.NewBufferString(form.Encode()))
		if err != nil {
			return 0, nil, err
		}
		req.Header = c.authHeader()
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return 0, nil, err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, b, nil
	}

	code, body, err := doOnce()
	if err != nil {
		return err
	}
	if code == 401 {
		if rerr := c.reauth(); rerr != nil {
			return fmt.Errorf("re-auth failed: %w", rerr)
		}
		code, body, err = doOnce()
		if err != nil {
			return err
		}
	}
	if code >= 400 {
		return fmt.Errorf("edit-tag: HTTP %d: %s", code, string(body))
	}
	return nil
}

// MarkRead marks items read.
func (c *Client) MarkRead(itemIDs []string) error {
	if len(itemIDs) == 0 {
		return nil
	}
	return c.EditTag(itemIDs, []string{"user/-/state/com.google/read"}, nil)
}

// MarkUnread marks items unread.
func (c *Client) MarkUnread(itemIDs []string) error {
	if len(itemIDs) == 0 {
		return nil
	}
	return c.EditTag(itemIDs, nil, []string{"user/-/state/com.google/read"})
}

// Star adds the starred tag.
func (c *Client) Star(itemIDs []string) error {
	if len(itemIDs) == 0 {
		return nil
	}
	return c.EditTag(itemIDs, []string{"user/-/state/com.google/starred"}, nil)
}

// Unstar removes the starred tag.
func (c *Client) Unstar(itemIDs []string) error {
	if len(itemIDs) == 0 {
		return nil
	}
	return c.EditTag(itemIDs, nil, []string{"user/-/state/com.google/starred"})
}

// AddLabel adds a user label to articles.
func (c *Client) AddLabel(itemIDs []string, label string) error {
	if len(itemIDs) == 0 || label == "" {
		return nil
	}
	return c.EditTag(itemIDs, []string{"user/-/label/" + label}, nil)
}

// Unsubscribe removes a feed.
func (c *Client) Unsubscribe(feedURL string) error {
	doOnce := func() (int, []byte, error) {
		tok, err := c.EditToken()
		if err != nil {
			return 0, nil, err
		}
		form := url.Values{}
		form.Set("T", tok)
		form.Set("ac", "unsubscribe")
		form.Set("s", "feed/"+feedURL)
		req, err := http.NewRequest(http.MethodPost, c.apiURL+"/reader/api/0/subscription/edit", bytes.NewBufferString(form.Encode()))
		if err != nil {
			return 0, nil, err
		}
		req.Header = c.authHeader()
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return 0, nil, err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, b, nil
	}

	code, body, err := doOnce()
	if err != nil {
		return err
	}
	if code == 401 {
		if rerr := c.reauth(); rerr != nil {
			return fmt.Errorf("re-auth failed: %w", rerr)
		}
		code, body, err = doOnce()
		if err != nil {
			return err
		}
	}
	if code >= 400 {
		return fmt.Errorf("unsubscribe: HTTP %d: %s", code, string(body))
	}
	return nil
}

// MarkAllRead marks all items in a stream as read up to a timestamp.
func (c *Client) MarkAllRead(streamID string, timestampUsec int64) error {
	doOnce := func() (int, []byte, error) {
		tok, err := c.EditToken()
		if err != nil {
			return 0, nil, err
		}
		form := url.Values{}
		form.Set("T", tok)
		form.Set("s", streamID)
		form.Set("ts", fmt.Sprintf("%d", timestampUsec))
		req, err := http.NewRequest(http.MethodPost, c.apiURL+"/reader/api/0/mark-all-as-read", bytes.NewBufferString(form.Encode()))
		if err != nil {
			return 0, nil, err
		}
		req.Header = c.authHeader()
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return 0, nil, err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, b, nil
	}

	code, body, err := doOnce()
	if err != nil {
		return err
	}
	if code == 401 {
		if rerr := c.reauth(); rerr != nil {
			return fmt.Errorf("re-auth failed: %w", rerr)
		}
		code, body, err = doOnce()
		if err != nil {
			return err
		}
	}
	if code >= 400 {
		return fmt.Errorf("mark-all-read: HTTP %d: %s", code, string(body))
	}
	return nil
}

// Subscribe adds a feed. Retries once on 401.
func (c *Client) Subscribe(feedURL string, title, folder string) error {
	doOnce := func() (int, []byte, error) {
		tok, err := c.EditToken()
		if err != nil {
			return 0, nil, err
		}
		form := url.Values{}
		form.Set("T", tok)
		form.Set("ac", "subscribe")
		form.Set("s", "feed/"+feedURL)
		if title != "" {
			form.Set("t", title)
		}
		if folder != "" {
			form.Set("a", "user/-/label/"+folder)
		}
		req, err := http.NewRequest(http.MethodPost, c.apiURL+"/reader/api/0/subscription/edit", bytes.NewBufferString(form.Encode()))
		if err != nil {
			return 0, nil, err
		}
		req.Header = c.authHeader()
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return 0, nil, err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, b, nil
	}

	code, body, err := doOnce()
	if err != nil {
		return err
	}
	if code == 401 {
		if rerr := c.reauth(); rerr != nil {
			return fmt.Errorf("re-auth failed: %w", rerr)
		}
		code, body, err = doOnce()
		if err != nil {
			return err
		}
	}
	if code >= 400 {
		return fmt.Errorf("subscribe: HTTP %d: %s", code, string(body))
	}
	return nil
}
