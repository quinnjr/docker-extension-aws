# Docker AWS MFA Extension

A Docker Desktop Extension that automatically handles AWS MFA authentication and injects credentials into Docker containers.

![AWS MFA Extension](screenshots/login.png)

## Features

- **Visual Dashboard**: Manage AWS MFA credentials directly from Docker Desktop
- **Multi-Profile Support**: Handle multiple AWS profiles with MFA
- **Auto-Expiry Tracking**: See credential expiration status at a glance
- **CLI Integration**: Full CLI tool for terminal workflows
- **Docker Integration**: Inject credentials into `docker run` and `docker compose`

## Installation

### From Docker Desktop Extension Marketplace

Search for "AWS MFA" in the Docker Desktop Extensions marketplace and click Install.

### Manual Installation

```bash
docker extension install quinnjr/docker-aws-mfa:latest
```

### From Source

```bash
git clone https://github.com/quinnjr/docker-plugin-aws.git
cd docker-plugin-aws
make install
```

## Prerequisites

AWS CLI configured with MFA serial in `~/.aws/config`:

```ini
[default]
region = us-west-2
mfa_serial = arn:aws:iam::123456789012:mfa/username

[profile myprofile]
region = us-east-1
mfa_serial = arn:aws:iam::987654321098:mfa/username
```

## Usage

### Docker Desktop UI

1. Open Docker Desktop
2. Click on "AWS MFA" in the left sidebar
3. Select your AWS profile
4. Enter your MFA token code
5. Click "Login with MFA"

Your credentials will be cached and shown in the dashboard.

### CLI Commands

The extension also installs a CLI tool:

```bash
# Authenticate with MFA
docker aws login
docker aws login -p myprofile

# Check status
docker aws status
docker aws status -a  # All profiles

# Export credentials
docker aws env -o ./aws.env
eval $(docker aws env --export)

# Run containers with AWS credentials
docker aws run -- -it amazon/aws-cli s3 ls
docker aws run -p myprofile -- myimage:latest

# Docker Compose with credentials
docker aws compose -- up -d
docker aws compose -p myprofile -- logs -f
```

## Development

### Build locally

```bash
make build
make install
```

### Development mode with hot reload

```bash
make dev
```

### View logs

```bash
make logs
```

## Publishing

### To Docker Hub

```bash
make build-cross
make push
```

### To Extension Marketplace

1. Build multi-architecture image: `make build-cross`
2. Push to Docker Hub: `make push`
3. Submit to [Docker Extension Marketplace](https://hub.docker.com/extensions)

## Architecture

```
docker-plugin-aws/
├── backend/           # Go backend for AWS operations
│   └── main.go
├── ui/                # React frontend
│   └── src/
│       ├── App.tsx
│       └── main.tsx
├── Dockerfile         # Multi-stage build
├── metadata.json      # Extension metadata
└── Makefile          # Build automation
```

## How It Works

1. **Backend**: Go service running in Docker Desktop VM handles AWS STS calls
2. **UI**: React dashboard communicates with backend via Docker Extension API
3. **CLI**: Binary installed on host for terminal workflows
4. **Caching**: Credentials cached in `~/.docker/aws-mfa-cache/` with auto-expiry

## License

MIT License - see [LICENSE](LICENSE)
