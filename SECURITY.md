# Security Policy

## Supported versions

Until DevHub reaches 1.0, security fixes are provided for the latest released minor version.

## Threat model

DevHub can start and stop local development processes and stores local project paths and commands. Treat the dashboard as a privileged local developer tool.

The default design intentionally:

- binds the management server only to `127.0.0.1`;
- does not enable CORS;
- requires JSON for mutation endpoints;
- rejects cross-origin browser mutations;
- requires a discovered service to be explicitly kept before process control is enabled;
- never deletes a project directory when a project is forgotten;
- stores state with user-only file permissions where the operating system supports them.

Do not expose the DevHub HTTP port through a public reverse proxy or bind it to an untrusted network.

## Reporting a vulnerability

Please use GitHub's private security advisory flow for this repository rather than opening a public issue with exploit details. Include affected versions, platform, reproduction steps, impact, and any suggested mitigation.
