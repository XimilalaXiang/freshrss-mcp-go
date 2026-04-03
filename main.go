package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/ximilala/freshrss-mcp-go/internal/greader"
	"github.com/ximilala/freshrss-mcp-go/internal/textutil"
)

var (
	frMu     sync.Mutex
	frClient *greader.Client
	frErr    error
)

func env(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}

func freshClient() (*greader.Client, error) {
	frMu.Lock()
	defer frMu.Unlock()
	if frClient != nil {
		return frClient, frErr
	}
	base := env("FRESHRSS_URL", "")
	if base == "" {
		frErr = fmt.Errorf("missing FRESHRSS_URL environment variable")
		return nil, frErr
	}
	email := env("FRESHRSS_EMAIL", "")
	if email == "" {
		email = env("FRESHRSS_USERNAME", "")
	}
	pass := env("FRESHRSS_API_PASSWORD", "")
	if pass == "" {
		pass = env("FRESHRSS_PASSWORD", "")
	}
	if email == "" || pass == "" {
		frErr = fmt.Errorf("missing credentials: set FRESHRSS_EMAIL (or FRESHRSS_USERNAME) and FRESHRSS_API_PASSWORD (or FRESHRSS_PASSWORD)")
		return nil, frErr
	}
	apiPath := env("FRESHRSS_API_PATH", "/api/greader.php")
	frClient = greader.New(base, email, pass, apiPath)
	frErr = frClient.Authenticate()
	if frErr != nil {
		frClient = nil
		return nil, frErr
	}
	return frClient, nil
}

func getArgs(req mcp.CallToolRequest) map[string]any {
	if m, ok := req.Params.Arguments.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func boolPtr(v bool) *bool { return &v }

func readOnlyAnnotation() mcp.ToolOption {
	return mcp.WithToolAnnotation(mcp.ToolAnnotation{
		ReadOnlyHint:    boolPtr(true),
		DestructiveHint: boolPtr(false),
		IdempotentHint:  boolPtr(true),
		OpenWorldHint:   boolPtr(true),
	})
}

func writeAnnotation() mcp.ToolOption {
	return mcp.WithToolAnnotation(mcp.ToolAnnotation{
		ReadOnlyHint:    boolPtr(false),
		DestructiveHint: boolPtr(false),
		IdempotentHint:  boolPtr(true),
		OpenWorldHint:   boolPtr(true),
	})
}

func main() {
	s := server.NewMCPServer("freshrss-mcp-go", "1.0.0")

	s.AddTool(mcp.NewTool("freshrss_list_subscriptions",
		mcp.WithDescription("List all subscribed RSS feeds (id, title, url, html_url)."),
		readOnlyAnnotation(),
	), handleListSubscriptions)

	s.AddTool(mcp.NewTool("freshrss_list_folders",
		mcp.WithDescription("List folder/label tags in FreshRSS."),
		readOnlyAnnotation(),
	), handleListFolders)

	s.AddTool(mcp.NewTool("freshrss_get_unread_count",
		mcp.WithDescription("Unread counts by feed id and folder (raw Reader API ids)."),
		readOnlyAnnotation(),
	), handleUnreadCount)

	s.AddTool(mcp.NewTool("freshrss_get_articles",
		mcp.WithDescription("Fetch articles. Supports folder, feed_id, starred_only, pagination. Token options: trim_content (default true), strip_html (default true), max_summary_length (default 400)."),
		readOnlyAnnotation(),
		mcp.WithString("folder", mcp.Description("Folder/label name (optional)")),
		mcp.WithString("feed_id", mcp.Description("Feed id to filter, e.g. '15' or 'feed/15' (from list_subscriptions)")),
		mcp.WithBoolean("starred_only", mcp.Description("Only starred articles")),
		mcp.WithBoolean("show_read", mcp.Description("Include read articles (default false = unread only)")),
		mcp.WithNumber("count", mcp.Description("Max articles (default 30)")),
		mcp.WithString("continuation", mcp.Description("Pagination token from previous response")),
		mcp.WithString("order", mcp.Description("newest or oldest (default newest)")),
		mcp.WithBoolean("trim_content", mcp.Description("Truncate body/summary (default true)")),
		mcp.WithBoolean("strip_html", mcp.Description("Strip HTML tags from text fields (default true)")),
		mcp.WithNumber("max_summary_length", mcp.Description("Max chars for summary+content after strip (default 400)")),
	), handleGetArticles)

	s.AddTool(mcp.NewTool("freshrss_mark_read",
		mcp.WithDescription("Mark articles as read by full item id strings."),
		writeAnnotation(),
		mcp.WithArray("article_ids", mcp.Required(), mcp.Description("Full FreshRSS item ids"), mcp.WithStringItems()),
	), handleMarkRead)

	s.AddTool(mcp.NewTool("freshrss_mark_unread",
		mcp.WithDescription("Mark articles as unread by full item id strings."),
		writeAnnotation(),
		mcp.WithArray("article_ids", mcp.Required(), mcp.Description("Full FreshRSS item ids"), mcp.WithStringItems()),
	), handleMarkUnread)

	s.AddTool(mcp.NewTool("freshrss_star_article",
		mcp.WithDescription("Star (favorite) articles."),
		writeAnnotation(),
		mcp.WithArray("article_ids", mcp.Required(), mcp.Description("Full FreshRSS item ids"), mcp.WithStringItems()),
	), handleStar)

	s.AddTool(mcp.NewTool("freshrss_unstar_article",
		mcp.WithDescription("Remove star from articles."),
		writeAnnotation(),
		mcp.WithArray("article_ids", mcp.Required(), mcp.Description("Full FreshRSS item ids"), mcp.WithStringItems()),
	), handleUnstar)

	s.AddTool(mcp.NewTool("freshrss_add_label",
		mcp.WithDescription("Add a label/tag to articles."),
		writeAnnotation(),
		mcp.WithArray("article_ids", mcp.Required(), mcp.Description("Full FreshRSS item ids"), mcp.WithStringItems()),
		mcp.WithString("label", mcp.Required(), mcp.Description("Label name to add")),
	), handleAddLabel)

	s.AddTool(mcp.NewTool("freshrss_unsubscribe",
		mcp.WithDescription("Unsubscribe from a feed."),
		writeAnnotation(),
		mcp.WithString("feed_url", mcp.Required(), mcp.Description("RSS/Atom feed URL to unsubscribe")),
	), handleUnsubscribe)

	s.AddTool(mcp.NewTool("freshrss_subscribe",
		mcp.WithDescription("Subscribe to a new feed URL."),
		writeAnnotation(),
		mcp.WithString("feed_url", mcp.Required(), mcp.Description("RSS/Atom feed URL")),
		mcp.WithString("title", mcp.Description("Optional title")),
		mcp.WithString("folder", mcp.Description("Optional folder/label name")),
	), handleSubscribe)

	s.AddTool(mcp.NewTool("freshrss_search_articles",
		mcp.WithDescription("Search articles by keyword in title/content. Uses client-side filtering."),
		readOnlyAnnotation(),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search keyword (case-insensitive)")),
		mcp.WithNumber("count", mcp.Description("Max articles to scan (default 100)")),
		mcp.WithNumber("max_results", mcp.Description("Max matching results to return (default 20)")),
		mcp.WithBoolean("show_read", mcp.Description("Include read articles (default true for search)")),
		mcp.WithBoolean("strip_html", mcp.Description("Strip HTML (default true)")),
		mcp.WithBoolean("trim_content", mcp.Description("Truncate body/summary (default true)")),
		mcp.WithNumber("max_summary_length", mcp.Description("Max chars for summary+content (default 400)")),
	), handleSearch)

	s.AddTool(mcp.NewTool("freshrss_get_article_detail",
		mcp.WithDescription("Get full article content without truncation. Pass a single article id."),
		readOnlyAnnotation(),
		mcp.WithString("article_id", mcp.Required(), mcp.Description("Full FreshRSS item id")),
		mcp.WithBoolean("strip_html", mcp.Description("Strip HTML tags (default true)")),
	), handleArticleDetail)

	s.AddTool(mcp.NewTool("freshrss_mark_all_read",
		mcp.WithDescription("Mark all articles in a feed or folder as read."),
		writeAnnotation(),
		mcp.WithString("feed_id", mcp.Description("Feed id, e.g. '15' or 'feed/15'")),
		mcp.WithString("folder", mcp.Description("Folder/label name")),
	), handleMarkAllRead)

	transport := env("MCP_TRANSPORT", "")
	port := env("MCP_PORT", "8080")

	switch transport {
	case "http":
		log.Printf("freshrss-mcp-go streamable-http on :%s", port)
		httpServer := server.NewStreamableHTTPServer(s)
		if err := httpServer.Start(":" + port); err != nil {
			log.Fatal(err)
		}
	case "sse":
		log.Printf("freshrss-mcp-go SSE on :%s", port)
		sseServer := server.NewSSEServer(s, server.WithSSEEndpoint("/sse"), server.WithMessageEndpoint("/message"))
		if err := sseServer.Start(":" + port); err != nil {
			log.Fatal(err)
		}
	default:
		log.Println("freshrss-mcp-go (stdio)")
		if err := server.ServeStdio(s); err != nil {
			log.Fatal(err)
		}
	}
}

func handleListSubscriptions(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	c, err := freshClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	list, err := c.ListSubscriptions()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	b, _ := json.MarshalIndent(list.Subscriptions, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}

func handleListFolders(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	c, err := freshClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	tags, err := c.ListTags()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	var folders []map[string]string
	for _, t := range tags.Tags {
		if strings.Contains(t.ID, "/label/") {
			name := t.Sort
			if name == "" {
				name = pathLast(t.ID)
			}
			folders = append(folders, map[string]string{"id": t.ID, "name": name})
		}
	}
	b, _ := json.MarshalIndent(folders, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}

func pathLast(id string) string {
	i := strings.LastIndex(id, "/")
	if i < 0 {
		return id
	}
	return id[i+1:]
}

func handleUnreadCount(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	c, err := freshClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	u, err := c.UnreadCounts()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	b, _ := json.MarshalIndent(u.UnreadCounts, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}

func handleGetArticles(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	c, err := freshClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	args := getArgs(req)

	var streamID string
	if b, ok := args["starred_only"].(bool); ok && b {
		streamID = "user/-/state/com.google/starred"
	} else if folder, ok := args["folder"].(string); ok && folder != "" {
		streamID = "user/-/label/" + folder
	} else if fid, ok := args["feed_id"].(string); ok && fid != "" {
		if strings.HasPrefix(fid, "feed/") {
			streamID = fid
		} else {
			streamID = "feed/" + fid
		}
	} else {
		streamID = "user/-/state/com.google/reading-list"
	}

	count := 30.0
	if v, ok := args["count"].(float64); ok {
		count = v
	}
	n := int(count)
	if n < 1 {
		n = 30
	}
	if n > 1000 {
		n = 1000
	}

	order := "d"
	if o, ok := args["order"].(string); ok && strings.EqualFold(o, "oldest") {
		order = "o"
	}

	excludeRead := true
	if sr, ok := args["show_read"].(bool); ok && sr {
		excludeRead = false
	}

	cont, _ := args["continuation"].(string)

	trim := true
	if t, ok := args["trim_content"].(bool); ok {
		trim = t
	}
	strip := true
	if t, ok := args["strip_html"].(bool); ok {
		strip = t
	}
	maxLen := 400.0
	if m, ok := args["max_summary_length"].(float64); ok {
		maxLen = m
	}
	max := int(maxLen)
	if max < 50 {
		max = 50
	}

	stream, err := c.GetStream(streamID, n, order, excludeRead, cont)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var out []greader.Article
	for _, it := range stream.Items {
		a, e := greader.ParseArticle(it)
		if e != nil {
			continue
		}
		if strip {
			a.Content = textutil.StripHTML(a.Content)
			a.Summary = textutil.StripHTML(a.Summary)
			a.Title = textutil.StripHTML(a.Title)
		}
		if trim {
			a.Content = textutil.TruncateAtWord(a.Content, max)
			a.Summary = textutil.TruncateAtWord(a.Summary, max)
		}
		out = append(out, a)
	}

	type response struct {
		Articles     []greader.Article `json:"articles"`
		Count        int               `json:"count"`
		HasMore      bool              `json:"has_more"`
		Continuation *string           `json:"continuation,omitempty"`
	}
	resp := response{
		Articles:     out,
		Count:        len(out),
		HasMore:      stream.Continuation != nil,
		Continuation: stream.Continuation,
	}
	b, _ := json.MarshalIndent(resp, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}

func handleMarkRead(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	c, err := freshClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	ids, err := req.RequireStringSlice("article_ids")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := c.MarkRead(ids); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf(`{"ok":true,"marked":%d}`, len(ids))), nil
}

func handleSubscribe(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	c, err := freshClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	args := getArgs(req)
	fu, _ := args["feed_url"].(string)
	if fu == "" {
		return mcp.NewToolResultError("feed_url required"), nil
	}
	title, _ := args["title"].(string)
	folder, _ := args["folder"].(string)
	if err := c.Subscribe(fu, title, folder); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "400") {
			errMsg += " (possible causes: feed already subscribed, invalid feed URL, or feed unreachable)"
		}
		return mcp.NewToolResultError(errMsg), nil
	}
	return mcp.NewToolResultText(`{"ok":true}`), nil
}

func handleMarkUnread(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	c, err := freshClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	ids, err := req.RequireStringSlice("article_ids")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := c.MarkUnread(ids); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf(`{"ok":true,"marked_unread":%d}`, len(ids))), nil
}

func handleStar(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	c, err := freshClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	ids, err := req.RequireStringSlice("article_ids")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := c.Star(ids); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf(`{"ok":true,"starred":%d}`, len(ids))), nil
}

func handleUnstar(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	c, err := freshClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	ids, err := req.RequireStringSlice("article_ids")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := c.Unstar(ids); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf(`{"ok":true,"unstarred":%d}`, len(ids))), nil
}

func handleAddLabel(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	c, err := freshClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	ids, err := req.RequireStringSlice("article_ids")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	args := getArgs(req)
	label, _ := args["label"].(string)
	if label == "" {
		return mcp.NewToolResultError("label required"), nil
	}
	if err := c.AddLabel(ids, label); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf(`{"ok":true,"labeled":%d,"label":%q}`, len(ids), label)), nil
}

func handleUnsubscribe(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	c, err := freshClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	args := getArgs(req)
	fu, _ := args["feed_url"].(string)
	if fu == "" {
		return mcp.NewToolResultError("feed_url required"), nil
	}
	if err := c.Unsubscribe(fu); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "400") {
			errMsg += " (possible cause: feed not found in subscriptions)"
		}
		return mcp.NewToolResultError(errMsg), nil
	}
	return mcp.NewToolResultText(`{"ok":true}`), nil
}

func handleSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	c, err := freshClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	args := getArgs(req)
	query, _ := args["query"].(string)
	if query == "" {
		return mcp.NewToolResultError("query required"), nil
	}
	queryLower := strings.ToLower(query)

	scanCount := 100
	if v, ok := args["count"].(float64); ok && v > 0 {
		scanCount = int(v)
	}
	maxResults := 20
	if v, ok := args["max_results"].(float64); ok && v > 0 {
		maxResults = int(v)
	}

	strip := true
	if v, ok := args["strip_html"].(bool); ok {
		strip = v
	}
	trim := true
	if v, ok := args["trim_content"].(bool); ok {
		trim = v
	}
	maxLen := 400
	if v, ok := args["max_summary_length"].(float64); ok {
		maxLen = int(v)
	}

	stream, err := c.GetStream("user/-/state/com.google/reading-list", scanCount, "d", false, "")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var results []greader.Article
	for _, it := range stream.Items {
		a, e := greader.ParseArticle(it)
		if e != nil {
			continue
		}
		titleLower := strings.ToLower(a.Title)
		contentLower := strings.ToLower(a.Content + a.Summary)
		if !strings.Contains(titleLower, queryLower) && !strings.Contains(contentLower, queryLower) {
			continue
		}
		if strip {
			a.Content = textutil.StripHTML(a.Content)
			a.Summary = textutil.StripHTML(a.Summary)
			a.Title = textutil.StripHTML(a.Title)
		}
		if trim {
			a.Content = textutil.TruncateAtWord(a.Content, maxLen)
			a.Summary = textutil.TruncateAtWord(a.Summary, maxLen)
		}
		results = append(results, a)
		if len(results) >= maxResults {
			break
		}
	}

	type searchResp struct {
		Query   string           `json:"query"`
		Results []greader.Article `json:"results"`
		Count   int              `json:"count"`
		Scanned int              `json:"scanned"`
	}
	resp := searchResp{
		Query:   query,
		Results: results,
		Count:   len(results),
		Scanned: len(stream.Items),
	}
	b, _ := json.MarshalIndent(resp, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}

func handleArticleDetail(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	c, err := freshClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	args := getArgs(req)
	articleID, _ := args["article_id"].(string)
	if articleID == "" {
		return mcp.NewToolResultError("article_id required"), nil
	}

	strip := true
	if v, ok := args["strip_html"].(bool); ok {
		strip = v
	}

	stream, err := c.GetItemContents([]string{articleID})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if len(stream.Items) == 0 {
		return mcp.NewToolResultError(fmt.Sprintf("article %q not found", articleID)), nil
	}

	a, e := greader.ParseArticle(stream.Items[0])
	if e != nil {
		return mcp.NewToolResultError(e.Error()), nil
	}
	if strip {
		a.Content = textutil.StripHTML(a.Content)
		a.Summary = textutil.StripHTML(a.Summary)
		a.Title = textutil.StripHTML(a.Title)
	}
	b, _ := json.MarshalIndent(a, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}

func handleMarkAllRead(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	c, err := freshClient()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	args := getArgs(req)

	var streamID string
	if folder, ok := args["folder"].(string); ok && folder != "" {
		streamID = "user/-/label/" + folder
	} else if fid, ok := args["feed_id"].(string); ok && fid != "" {
		if strings.HasPrefix(fid, "feed/") {
			streamID = fid
		} else {
			streamID = "feed/" + fid
		}
	} else {
		return mcp.NewToolResultError("provide either feed_id or folder"), nil
	}

	now := time.Now().UnixMicro()
	if err := c.MarkAllRead(streamID, now); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf(`{"ok":true,"stream":%q}`, streamID)), nil
}
