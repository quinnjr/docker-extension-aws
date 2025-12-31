# Build the Go backend
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

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
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS cli-builder

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

# Build the UI
FROM --platform=$BUILDPLATFORM node:20-alpine AS ui-builder

WORKDIR /app
COPY ui/package*.json ./
RUN npm ci
COPY ui/ ./
RUN npm run build

# Final image
FROM alpine:3.19

LABEL org.opencontainers.image.title="AWS MFA Credentials" \
    org.opencontainers.image.description="Docker Desktop extension for AWS MFA credential management" \
    org.opencontainers.image.vendor="quinnjr" \
    com.docker.desktop.extension.api.version="0.3.4" \
    com.docker.desktop.extension.icon="https://raw.githubusercontent.com/quinnjr/docker-plugin-aws/main/aws-icon.svg" \
    com.docker.extension.screenshots='[{"alt":"AWS MFA Login","url":"https://raw.githubusercontent.com/quinnjr/docker-plugin-aws/main/screenshots/login.png"}]' \
    com.docker.extension.detailed-description="Automatically manage AWS MFA credentials for Docker containers" \
    com.docker.extension.publisher-url="https://github.com/quinnjr" \
    com.docker.extension.additional-urls='[{"title":"GitHub","url":"https://github.com/quinnjr/docker-plugin-aws"}]' \
    com.docker.extension.changelog="Initial release"

# Copy metadata
COPY metadata.json .
COPY aws-icon.svg .

# Copy backend binary
COPY --from=builder /backend /backend

# Copy UI
COPY --from=ui-builder /app/dist /ui

# Copy CLI binaries for host
COPY --from=cli-builder /darwin-amd64/docker-aws /darwin/docker-aws
COPY --from=cli-builder /darwin-arm64/docker-aws /darwin-arm64/docker-aws
COPY --from=cli-builder /linux-amd64/docker-aws /linux/docker-aws
COPY --from=cli-builder /linux-arm64/docker-aws /linux-arm64/docker-aws
COPY --from=cli-builder /windows-amd64/docker-aws.exe /windows/docker-aws.exe

# Run the backend
CMD ["/backend", "-socket", "/run/guest-services/backend.sock"]
