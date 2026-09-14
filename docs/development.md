# Development guide

## Run locally

```bash
go run ./cmd/devhub --no-open
```

The dashboard is embedded at compile time from `internal/server/web`.

## Tests and static checks

```bash
gofmt -w ./cmd ./internal
go test ./...
go vet ./...
```

## Cross-platform compile check

Because DevHub uses no CGO dependencies, basic cross compilation is straightforward:

```bash
make cross-build
```

Runtime behavior still needs testing on the target operating system, especially listener discovery and process-tree termination.

## Testing discovery manually

Start a normal development server such as Vite or Next.js from a Git repository, then open DevHub. A high-confidence card should appear within one scan interval. Verify that its working directory and inferred start command are correct before keeping the project.
