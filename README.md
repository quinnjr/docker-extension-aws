# Docker AWS MFA Plugin

A Docker CLI plugin that automatically handles AWS MFA authentication and injects credentials into Docker containers.

## Installation

```bash
./install.sh
```

Or manually:

```bash
cp docker-aws ~/.docker/cli-plugins/docker-aws
chmod +x ~/.docker/cli-plugins/docker-aws
```

## Prerequisites

- AWS CLI configured with MFA serial in `~/.aws/config`
- `jq` installed for JSON parsing

Example `~/.aws/config`:
```ini
[default]
region = us-west-2
mfa_serial = arn:aws:iam::123456789012:mfa/username

[profile myprofile]
region = us-east-1
mfa_serial = arn:aws:iam::987654321098:mfa/username
```

## Usage

### Authenticate with MFA

```bash
# Login with default profile (prompts for MFA token)
docker aws login

# Login with specific profile
docker aws login -p myprofile

# Login with token provided
docker aws login -t 123456

# Custom session duration (default: 12 hours)
docker aws login -d 3600
```

### Check Authentication Status

```bash
# Check default profile
docker aws status

# Check all cached profiles
docker aws status -a
```

### Export Credentials

```bash
# Output as KEY=VALUE (for env files)
docker aws env

# Output as export statements (for shell)
docker aws env --export

# Write to a file
docker aws env -o ./aws.env

# Load into current shell
eval $(docker aws env --export)

# Different formats
docker aws env -f env      # KEY=VALUE
docker aws env -f export   # export KEY=VALUE
docker aws env -f docker   # -e KEY=VALUE flags
docker aws env -f json     # JSON format
```

### Run Containers with AWS Credentials

```bash
# Run a container with AWS credentials injected
docker aws run -- -it amazon/aws-cli s3 ls

# Use a specific profile
docker aws run -p myprofile -- myimage:latest

# Any docker run arguments work after --
docker aws run -- -v $(pwd):/app -w /app myimage cmd
```

### Run Docker Compose with AWS Credentials

```bash
# Run compose with credentials from default profile
docker aws compose -- up -d

# Use specific profile
docker aws compose -p myprofile -- up

# Pass any compose arguments
docker aws compose -- logs -f myservice
```

### Clear Cached Credentials

```bash
# Clear default profile
docker aws clear

# Clear specific profile
docker aws clear -p myprofile

# Clear all cached credentials
docker aws clear -a
```

## How It Works

1. **Login**: When you run `docker aws login`, it prompts for your MFA token and calls `aws sts get-session-token` to get temporary credentials.

2. **Caching**: Credentials are cached in `~/.docker/aws-mfa-cache/` with 600 permissions. The plugin automatically checks if credentials are still valid (with a 5-minute buffer).

3. **Auto-refresh**: When running `docker aws run` or `docker aws compose`, the plugin checks if credentials are valid and prompts for re-authentication if needed.

4. **Injection**: For `docker aws run`, credentials are passed as `-e` environment variables. For `docker aws compose`, an env file is generated and passed via `--env-file`.

## Integration with docker-compose.yml

You can reference the generated env file in your compose file:

```yaml
services:
  myservice:
    image: myimage
    env_file:
      - ~/.docker/aws-mfa-cache/default.env
```

Then run:
```bash
docker aws login  # Ensure credentials are fresh
docker compose up
```

Or use the plugin directly:
```bash
docker aws compose -- up
```
