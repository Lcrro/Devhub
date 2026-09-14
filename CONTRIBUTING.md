# Contributing to DevHub

Thanks for helping improve DevHub.

## Before opening a pull request

1. Search existing issues and pull requests first.
2. Open an issue before large behavior or architecture changes.
3. Keep discovery heuristics conservative: a false positive can expose process controls for the wrong local service.
4. Preserve the loopback-only default and same-origin mutation protections.
5. Add or update tests for discovery, persistence, and process behavior when relevant.

## Local workflow

```bash
gofmt -w ./cmd ./internal
go test ./...
go vet ./...
go build ./cmd/devhub
```

`make test` runs the same quality checks.

## Pull requests

Keep pull requests focused. Explain the user-facing problem, the approach, platform implications, and any security tradeoffs. Screenshots are encouraged for dashboard changes.

## Platform changes

DevHub aims to support Linux, macOS, and Windows. Platform-specific process code should use Go build tags where necessary. If a feature cannot be equivalent on all platforms, document the difference rather than silently pretending parity.

## Security-sensitive code

Changes to process execution, service discovery, HTTP mutation endpoints, filesystem access, and screenshot capture deserve extra review. Do not weaken loopback binding or origin/content-type checks for convenience.
