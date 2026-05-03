#!/usr/bin/env bash
#
# cmux installer
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "Installing cmux..."

# Build the Go CLI so the installed command is a normal executable, not a
# development-only wrapper.
if ! command -v go >/dev/null 2>&1; then
    echo "Error: go is required. Install Go 1.24+ first." >&2
    exit 1
fi

echo "Building cmux..."
mkdir -p "${SCRIPT_DIR}/dist"
go build -o "${SCRIPT_DIR}/dist/cmux-go" ./cmd/cmux

# Determine install directory
INSTALL_DIR="${HOME}/bin"
if [[ ! -d "$INSTALL_DIR" ]]; then
    mkdir -p "$INSTALL_DIR"
    echo "Created $INSTALL_DIR"
fi

# Create symlink
if [[ -L "${INSTALL_DIR}/cmux" ]]; then
    rm "${INSTALL_DIR}/cmux"
fi

ln -s "${SCRIPT_DIR}/cmux" "${INSTALL_DIR}/cmux"
echo "Linked cmux to ${INSTALL_DIR}/cmux"

# Check if ~/bin is in PATH
if [[ ":$PATH:" != *":${HOME}/bin:"* ]]; then
    echo ""
    echo "NOTE: Add ~/bin to your PATH if not already done:"
    echo "  export PATH=\"\$HOME/bin:\$PATH\""
    echo ""
fi

echo ""
echo "Installation complete!"
echo ""
echo "Quick start:"
echo "  cmux                          # Open session selector"
echo "  cmux new ~/projects/my-app    # Create a plain session"
echo "  cmux agent start REB-123      # Start Linear-backed agent work"
echo "  cmux agent list               # List structured agent sessions"
echo "  cmux help                     # See all commands"
echo ""
