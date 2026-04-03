package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

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

	s.AddTool(mcp.NewTool("freshrss_subscribe",
		mcp.WithDescription("Subscribe to a new feed URL."),
		writeAnnotation(),
		mcp.WithString("feed_url", mcp.Required(), mcp.Description("RSS/Atom feed URL")),
		mcp.WithString("title", mcp.Description("Optional title")),
		mcp.WithString("folder", mcp.Description("Optional folder/label name")),
	), handleSubscribe)

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
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(`{"ok":true}`), nil
}
