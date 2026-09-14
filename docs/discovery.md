# Discovery model

Discovery is designed to prefer false negatives over dangerous false positives.

## Inputs

Depending on the platform, DevHub starts from `ss`, `lsof`, or PowerShell `Get-NetTCPConnection`. It then attempts to obtain:

- local address and port;
- PID and process name;
- process command line;
- working directory where available;
- nearest project manifest;
- local HTTP or HTTPS responsiveness.

## Confidence signals

The current score is intentionally simple and auditable:

| Signal | Score |
| --- | ---: |
| Loopback or local wildcard listener | +10 |
| Project root manifest found | +25 |
| Development runtime hint | +15 |
| Known framework hint | +20 |
| HTTP(S) probe succeeds | +30 |
| Known database/system service | -80 |

Candidates must reach 55 points. Scores are clamped to 0-100.

## Identity

When a working directory is available, the stable identity is based primarily on that directory plus framework/runtime information. This allows a project to move from one port to another without becoming a new card.

When DevHub already has a managed project, discovery also reconciles candidates by normalized working directory or project root so manually added projects do not duplicate themselves after they start.

## Extending detection

New detectors should be deterministic, fast, local-only, and conservative. Avoid deep filesystem crawls or long network probes. Framework-specific logic belongs in inference code rather than the platform listener enumeration layer.
