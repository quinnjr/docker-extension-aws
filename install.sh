#!/bin/bash
#
# Install the Docker AWS MFA plugin
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_DIR="${HOME}/.docker/cli-plugins"
PLUGIN_NAME="docker-aws"

echo "Installing Docker AWS MFA plugin..."

# Create plugin directory if it doesn't exist
mkdir -p "$PLUGIN_DIR"

# Copy the plugin
cp "${SCRIPT_DIR}/docker-aws" "${PLUGIN_DIR}/${PLUGIN_NAME}"
chmod +x "${PLUGIN_DIR}/${PLUGIN_NAME}"

echo "Plugin installed to: ${PLUGIN_DIR}/${PLUGIN_NAME}"

# Verify installation
echo ""
echo "Verifying installation..."
if docker aws --help > /dev/null 2>&1; then
    echo "Installation successful!"
    echo ""
    echo "Usage:"
    echo "  docker aws login              # Authenticate with MFA"
    echo "  docker aws status             # Check authentication status"
    echo "  docker aws env -o ./aws.env   # Export credentials to file"
    echo "  docker aws run -- <args>      # Run container with creds"
    echo "  docker aws compose -- <args>  # Run compose with creds"
else
    echo "Warning: Plugin installed but 'docker aws' command not recognized."
    echo "You may need to restart your terminal or Docker daemon."
fi
