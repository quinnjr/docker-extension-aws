#!/bin/bash
#
# Remote installer for Docker AWS MFA Plugin
# Usage: curl -fsSL https://raw.githubusercontent.com/quinnjr/docker-plugin-aws/main/install-remote.sh | bash
#

set -e

REPO="quinnjr/docker-plugin-aws"
PLUGIN_DIR="${HOME}/.docker/cli-plugins"
PLUGIN_NAME="docker-aws"

echo "Installing Docker AWS MFA Plugin..."

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

# Create plugin directory
mkdir -p "$PLUGIN_DIR"

# Get latest release tag
LATEST_TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$LATEST_TAG" ]; then
    echo "Error: Could not determine latest release"
    exit 1
fi

echo "Latest version: ${LATEST_TAG}"

# Download the plugin
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${LATEST_TAG}/docker-aws"
echo "Downloading from: ${DOWNLOAD_URL}"

if ! curl -fsSL "$DOWNLOAD_URL" -o "${PLUGIN_DIR}/${PLUGIN_NAME}"; then
    echo "Error: Failed to download plugin"
    exit 1
fi

chmod +x "${PLUGIN_DIR}/${PLUGIN_NAME}"

echo ""
echo "Plugin installed to: ${PLUGIN_DIR}/${PLUGIN_NAME}"

# Verify installation
if docker aws --help > /dev/null 2>&1; then
    echo "Installation successful!"
    echo ""
    echo "Quick start:"
    echo "  docker aws login              # Authenticate with MFA"
    echo "  docker aws status             # Check authentication status"
    echo "  docker aws run -- <args>      # Run container with AWS creds"
    echo ""
    echo "Run 'docker aws --help' for full usage information."
else
    echo ""
    echo "Warning: Plugin installed but verification failed."
    echo "Try restarting your terminal."
fi
