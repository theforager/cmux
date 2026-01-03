#!/usr/bin/env bash
#
# cmux installer
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "Installing cmux..."

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

# Offer to install slash command
echo ""
read -p "Install Claude Code slash command /cmux_name? [y/N] " -n 1 -r
echo ""

if [[ $REPLY =~ ^[Yy]$ ]]; then
    CLAUDE_CMD_DIR="${HOME}/.claude/commands"
    mkdir -p "$CLAUDE_CMD_DIR"
    cp "${SCRIPT_DIR}/commands/cmux_name.md" "$CLAUDE_CMD_DIR/"
    echo "Installed /cmux_name command to $CLAUDE_CMD_DIR"
fi

echo ""
echo "Installation complete!"
echo ""
echo "Quick start:"
echo "  cmux new ~/projects/my-app    # Create a new session"
echo "  cmux                          # Open session selector"
echo "  cmux switch                   # Switch sessions (inside tmux)"
echo "  cmux help                     # See all commands"
echo ""
