# freshrss-mcp-go

Merged design: PyPI feature set (GReader `Auth` + edit `T` token, string item ids) + Chris-style token helpers (strip HTML, word-boundary truncation, lean JSON).

**Path:** `/data/tmp/freshrss-mcp-go` (writable data disk; `/data/` root was not writable here).

## Env

See `.env.example`. Required: `FRESHRSS_URL`, `FRESHRSS_EMAIL` (or `FRESHRSS_USERNAME`), `FRESHRSS_API_PASSWORD` (or `FRESHRSS_PASSWORD`).

## Run

```bash
export FRESHRSS_URL=... FRESHRSS_EMAIL=... FRESHRSS_API_PASSWORD=...
go run .
# default: stdio MCP

MCP_TRANSPORT=http MCP_PORT=8080 go run .   # streamable HTTP
MCP_TRANSPORT=sse MCP_PORT=8080 go run .
```

## Docker

```bash
docker build -t freshrss-mcp-go .
docker run --rm -e FRESHRSS_URL -e FRESHRSS_EMAIL -e FRESHRSS_API_PASSWORD -e MCP_TRANSPORT=http -p 8080:8080 freshrss-mcp-go
```

## Tools

`freshrss_list_subscriptions`, `freshrss_list_folders`, `freshrss_get_unread_count`, `freshrss_get_articles`, `freshrss_mark_read`, `freshrss_subscribe`.
