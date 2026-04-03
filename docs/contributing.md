# Contributing

## Development Setup

### Prerequisites

- Docker Desktop with Extensions enabled
- Go 1.21+ (for the backend)
- Node.js 18+ and pnpm (for the UI)
- Make

### Building

```bash
# Full build (UI dependencies + Docker image)
make build

# Install into Docker Desktop
make install

# Update after changes
make update
```

### Development Mode

```bash
# Enable hot reload for the UI
make dev
```

This builds the extension image, enables Docker Desktop development mode, and starts the Angular dev server at `http://localhost:4200`. The extension UI in Docker Desktop will load from the dev server instead of the bundled files, giving instant feedback on UI changes.

To reset back to the bundled UI:

```bash
make dev-reset
```

### Viewing Logs

```bash
make logs
```

## Architecture

The extension has three components:

### Backend (`backend/`)

Go service running inside the Docker Desktop VM. Handles:
- AWS STS `GetSessionToken` calls for MFA authentication
- Credential caching and expiry tracking
- Profile management from `~/.aws/config`

Entry point: `backend/main.go`

Communication: The Docker Extension API exposes the backend via a Unix socket (`backend.sock`), which the UI and CLI call through the Docker Desktop SDK.

### UI (`ui/`)

Angular application rendered as a Docker Desktop dashboard tab. Provides:
- Profile selector dropdown
- MFA token input
- Credential status dashboard with expiry indicators

The UI communicates with the backend through the Docker Extension API's `ddClient.extension.vm.service.post()` methods.

### CLI (`docker-aws`)

Host binary installed by the extension. Provides terminal-based workflows:
- `docker aws login` -- MFA authentication
- `docker aws status` -- Credential status
- `docker aws env` -- Export credentials as environment variables
- `docker aws run` -- Run containers with AWS credentials injected
- `docker aws compose` -- Docker Compose with AWS credentials

The CLI communicates with the backend via the Docker Extension API.

## Publishing

### Docker Hub

```bash
# Build for amd64 and arm64
make build-cross

# Push to Docker Hub
make push
```

### Extension Marketplace

1. Build multi-architecture: `make build-cross`
2. Push to Docker Hub: `make push`
3. Submit to the [Docker Extension Marketplace](https://hub.docker.com/extensions)

The `metadata.json` file defines the extension structure that Docker Desktop reads.

## Code Style

- Go: Standard `gofmt` formatting
- TypeScript/Angular: Follow the Angular style guide
- Commit messages: Conventional Commits

## Branching

- `main` -- Stable releases, merge via PR only
- `develop` -- Integration branch
- `feature/*` -- Feature branches
