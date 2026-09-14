# Architecture

DevHub is intentionally a single Go binary with an embedded static dashboard.

## Components

```text
Browser dashboard
      |
      | same-origin HTTP/JSON
      v
Local HTTP server (127.0.0.1 only)
      |
      +-- discovery scanner
      +-- project state store
      +-- process manager
      +-- screenshot capturer
      +-- embedded web assets
```

## Discovery scanner

The scanner enumerates TCP listeners using the platform's native tooling, maps listeners to PIDs, then enriches each candidate with process command line, working directory when available, project manifests, framework hints, and an HTTP(S) probe.

Only candidates above the confidence threshold are surfaced. Common database and system-daemon names receive a strong negative score.

## Store

The first release uses an atomic JSON state file instead of a database. This keeps the binary dependency-free and makes state inspectable. The file is rewritten through a temporary file and rename operation. Cover images are separate PNG files.

The store tracks both projects and ignored discovery identities. Ignoring a discovered service therefore survives restarts.

## Process manager

Commands are launched through the platform shell so existing project scripts keep familiar shell behavior. On Unix-like systems, DevHub starts commands in a dedicated process group. On Windows, tree termination uses `taskkill /T`.

Externally discovered processes are never controllable until the user explicitly keeps that project.

## Screenshot capture

DevHub does not bundle a browser. It locates Chrome, Chromium, or Edge already installed on the machine and invokes headless screenshot mode. Automatic captures are rate-limited per project by time and deduplicated while a capture is in flight.

## Web security

The server binds to loopback only. Browser mutation endpoints require JSON and reject non-local origins. CORS is not enabled. Static responses include a restrictive Content Security Policy and framing is disabled.
