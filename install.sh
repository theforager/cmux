#!/usr/bin/env bash
#
# cmux installer
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "Installing cmux..."

if ! command -v go >/dev/null 2>&1; then
    echo "Error: go is required. Install Go 1.24+ first." >&2
    exit 1
fi

# Determine install directory
INSTALL_DIR="${HOME}/bin"
if [[ ! -d "$INSTALL_DIR" ]]; then
    mkdir -p "$INSTALL_DIR"
    echo "Created $INSTALL_DIR"
fi

if [[ -e "${INSTALL_DIR}/cmux" || -L "${INSTALL_DIR}/cmux" ]]; then
    rm "${INSTALL_DIR}/cmux"
fi

echo "Building cmux..."
go build -o "${INSTALL_DIR}/cmux" ./cmd/cmux

if [[ ! -x "${INSTALL_DIR}/cmux" ]]; then
    echo "Error: failed to install ${INSTALL_DIR}/cmux" >&2
    exit 1
fi

echo "Installed cmux to ${INSTALL_DIR}/cmux"

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
