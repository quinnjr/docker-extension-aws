# Build the Go backend
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache gcc musl-dev

# Copy go mod files
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Copy source
COPY backend/ ./

# Build for multiple platforms
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o /backend .

# Build CLI binary for host installation
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS cli-builder

WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./

# Build for all platforms
RUN CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o /darwin-amd64/docker-aws .
RUN CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o /darwin-arm64/docker-aws .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /linux-amd64/docker-aws .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o /linux-arm64/docker-aws .
RUN CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o /windows-amd64/docker-aws.exe .

# Build the UI with Angular 21 and pnpm
FROM --platform=$BUILDPLATFORM node:22-alpine AS ui-builder

WORKDIR /app

# Install pnpm
RUN corepack enable && corepack prepare pnpm@9.15.0 --activate

# Copy package files
COPY ui/package.json ui/pnpm-lock.yaml* ./
RUN pnpm install --frozen-lockfile || pnpm install

# Copy source and build
COPY ui/ ./
RUN pnpm run build

# Final image
FROM alpine:3.19

LABEL org.opencontainers.image.title="AWS-MFA-Credentials" \
    org.opencontainers.image.description="Manage AWS MFA credentials for Docker containers with automatic session token generation" \
    org.opencontainers.image.vendor="quinnjr" \
    org.opencontainers.image.source="https://github.com/quinnjr/docker-plugin-aws" \
    org.opencontainers.image.licenses="MIT" \
    com.docker.desktop.extension.api.version="0.3.4" \
    com.docker.desktop.extension.icon="https://raw.githubusercontent.com/quinnjr/docker-plugin-aws/main/aws-icon.svg" \
    com.docker.extension.screenshots='[{"alt":"AWS MFA Login - Authenticate with MFA token","url":"https://raw.githubusercontent.com/quinnjr/docker-plugin-aws/main/screenshots/login.png"},{"alt":"Settings - Configure credential sources","url":"https://raw.githubusercontent.com/quinnjr/docker-plugin-aws/main/screenshots/settings.png"}]' \
    com.docker.extension.detailed-description="<h2>AWS MFA Credentials Manager</h2><p>Simplify AWS MFA authentication for Docker workflows. This extension manages temporary session credentials so you can seamlessly use AWS services in containers.</p><h3>Features</h3><ul><li>Authenticate with MFA token codes</li><li>Automatic credential caching (12-hour sessions)</li><li>WSL2 integration support</li><li>Multiple credential source options</li><li>CLI tool for terminal usage</li><li>Export credentials as environment files</li></ul><h3>CLI Commands</h3><pre>docker aws login -p profile-name<br/>docker aws run -- aws s3 ls<br/>docker aws compose -- up -d<br/>eval $(docker aws env --export)</pre>" \
    com.docker.extension.publisher-url="https://github.com/quinnjr" \
    com.docker.extension.additional-urls='[{"title":"GitHub Repository","url":"https://github.com/quinnjr/docker-plugin-aws"},{"title":"Report Issues","url":"https://github.com/quinnjr/docker-plugin-aws/issues"}]' \
    com.docker.extension.categories='["cloud","security","utility"]' \
    com.docker.extension.changelog="<h3>v4.0.0</h3><ul><li>WSL2 integration support</li><li>Multiple credential source selection</li><li>Settings UI panel</li><li>Environment detection</li></ul>"

# Copy metadata
COPY metadata.json .
COPY aws-icon.svg .

# Copy backend binary
COPY --from=builder /backend /backend

# Copy UI (Angular outputs to dist/browser/)
COPY --from=ui-builder /app/dist/browser /ui

# Copy CLI binaries for host
COPY --from=cli-builder /darwin-amd64/docker-aws /darwin/docker-aws
COPY --from=cli-builder /darwin-arm64/docker-aws /darwin-arm64/docker-aws
COPY --from=cli-builder /linux-amd64/docker-aws /linux/docker-aws
COPY --from=cli-builder /linux-arm64/docker-aws /linux-arm64/docker-aws
COPY --from=cli-builder /windows-amd64/docker-aws.exe /windows/docker-aws.exe

# Run the backend
CMD ["/backend", "-socket", "/run/guest-services/backend.sock"]
