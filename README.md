# freshrss-mcp-go

A Go-based MCP (Model Context Protocol) server for FreshRSS, combining the best of existing implementations with token-efficient output.

## Features

- **14 MCP tools** for complete FreshRSS management
- **HTML stripping** removes tags for clean text output
- **Word-boundary truncation** for token-efficient summaries
- **Auth retry** on 401 (automatic re-authentication)
- **Streamable HTTP, SSE, and stdio** transport modes
- **30MB Docker image** (Alpine multi-stage build)

## Tools

| Tool | Description | Type |
|------|-------------|------|
| `freshrss_list_subscriptions` | List all subscribed feeds | Read |
| `freshrss_list_folders` | List folder/label tags | Read |
| `freshrss_get_unread_count` | Unread counts per feed | Read |
| `freshrss_get_articles` | Fetch articles with filtering and pagination | Read |
| `freshrss_search_articles` | Search articles by keyword | Read |
| `freshrss_get_article_detail` | Get full article content (no truncation) | Read |
| `freshrss_mark_read` | Mark articles as read | Write |
| `freshrss_mark_unread` | Mark articles as unread | Write |
| `freshrss_mark_all_read` | Mark all in feed/folder as read | Write |
| `freshrss_star_article` | Star articles | Write |
| `freshrss_unstar_article` | Remove star from articles | Write |
| `freshrss_add_label` | Add label/tag to articles | Write |
| `freshrss_subscribe` | Subscribe to a feed | Write |
| `freshrss_unsubscribe` | Unsubscribe from a feed | Write |

## Setup

### Environment Variables

See `.env.example`. Required:

- `FRESHRSS_URL` — Your FreshRSS instance URL
- `FRESHRSS_EMAIL` (or `FRESHRSS_USERNAME`) — Login username
- `FRESHRSS_API_PASSWORD` (or `FRESHRSS_PASSWORD`) — API password (set in FreshRSS Settings → Authentication)

Optional:

- `FRESHRSS_API_PATH` — API path (default: `/api/greader.php`)
- `MCP_TRANSPORT` — `http`, `sse`, or empty for stdio
- `MCP_PORT` — Port for HTTP/SSE mode (default: `8080`)

### Run with Go

```bash
export FRESHRSS_URL=... FRESHRSS_EMAIL=... FRESHRSS_API_PASSWORD=...
go run .                                        # stdio
MCP_TRANSPORT=http MCP_PORT=8080 go run .       # Streamable HTTP
MCP_TRANSPORT=sse MCP_PORT=8080 go run .        # SSE
```

### Run with Docker

```bash
docker build -t freshrss-mcp-go .
docker run --rm \
  -e FRESHRSS_URL \
  -e FRESHRSS_EMAIL \
  -e FRESHRSS_API_PASSWORD \
  -e MCP_TRANSPORT=http \
  -p 8080:8080 \
  freshrss-mcp-go
```

### Docker Compose

```bash
cp .env.example .env
# Edit .env with your credentials
docker compose up -d
```

## MCP Client Configuration

### Streamable HTTP (Docker)

```json
{
  "mcpServers": {
    "freshrss": {
      "type": "streamableHttp",
      "url": "http://localhost:8080/mcp"
    }
  }
}
```

### stdio (local binary)

```json
{
  "mcpServers": {
    "freshrss": {
      "command": "/path/to/freshrss-mcp-go",
      "env": {
        "FRESHRSS_URL": "https://rss.example.com",
        "FRESHRSS_EMAIL": "user",
        "FRESHRSS_API_PASSWORD": "api-password"
      }
    }
  }
}
```

## Token Efficiency

The `get_articles` tool supports three token-saving options:

- `strip_html` (default: true) — Removes HTML tags, collapses whitespace
- `trim_content` (default: true) — Truncates at word boundaries
- `max_summary_length` (default: 400) — Maximum characters for content/summary

Typical savings: **~90% reduction** in token usage compared to raw HTML content.
