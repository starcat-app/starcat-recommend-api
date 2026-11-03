# Contributing

This service follows the same conventions as the other Starcat backend APIs.

Before submitting changes:

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

Keep provider-specific behavior behind `internal/provider`. Do not expose internal model artifacts or public-metadata transport payloads directly to Starcat clients.
