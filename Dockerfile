# Build
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /freshrss-mcp-go .

# Run
FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /freshrss-mcp-go .
ENV MCP_TRANSPORT=http
ENV MCP_PORT=8080
EXPOSE 8080
ENTRYPOINT ["/app/freshrss-mcp-go"]
