#!/bin/sh
# Codeany Installer
# Usage: curl -fsSL https://raw.githubusercontent.com/codeany-ai/codeany/main/install.sh | sh
set -e

REPO="codeany-ai/codeany"
BINARY="codeany"

# Colors
if [ -t 1 ]; then
  GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; BOLD='\033[1m'; NC='\033[0m'
else
  GREEN=''; YELLOW=''; RED=''; BOLD=''; NC=''
fi

info()    { printf "${GREEN}[INFO]${NC} %s\n" "$*"; }
warn()    { printf "${YELLOW}[WARN]${NC} %s\n" "$*"; }
error()   { printf "${RED}[ERROR]${NC} %s\n" "$*" >&2; exit 1; }
success() { printf "${GREEN}[✓]${NC} %s\n" "$*"; }

# Detect OS & arch
detect_platform() {
  _os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  _arch="$(uname -m)"

  case "$_os" in
    linux)  OS="linux" ;;
    darwin) OS="darwin" ;;
    msys*|mingw*|cygwin*) OS="windows" ;;
    *) error "Unsupported OS: $_os" ;;
  esac

  case "$_arch" in
    x86_64|amd64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) error "Unsupported architecture: $_arch" ;;
  esac

  PLATFORM="${OS}_${ARCH}"
  EXT="tar.gz"
  [ "$OS" = "windows" ] && EXT="zip"
}

# Choose install directory
choose_install_dir() {
  if [ -n "${CODEANY_INSTALL_DIR:-}" ]; then
    INSTALL_DIR="$CODEANY_INSTALL_DIR"
  else
    INSTALL_DIR="$HOME/.local/bin"
  fi
  mkdir -p "$INSTALL_DIR"
}

# Ensure install dir is in PATH
ensure_path() {
  case ":$PATH:" in
    *":$INSTALL_DIR:"*) return 0 ;;
  esac

  SHELL_NAME="$(basename "$SHELL" 2>/dev/null || echo sh)"
  case "$SHELL_NAME" in
    zsh)  RC="$HOME/.zshrc" ;;
    bash) RC="$HOME/.bashrc" ;;
    fish) RC="$HOME/.config/fish/config.fish" ;;
    *)    RC="$HOME/.profile" ;;
  esac

  LINE="export PATH=\"$INSTALL_DIR:\$PATH\""
  if [ "$SHELL_NAME" = "fish" ]; then
    LINE="set -gx PATH $INSTALL_DIR \$PATH"
  fi

  if [ -f "$RC" ] && grep -qF "$INSTALL_DIR" "$RC" 2>/dev/null; then
    return 0
  fi

  printf "\n# Added by codeany installer\n%s\n" "$LINE" >> "$RC"
  warn "Added $INSTALL_DIR to PATH in $RC"
  warn "Run: source $RC  (or restart your terminal)"
}

# Get latest release URL
get_download_url() {
  LATEST_URL="https://api.github.com/repos/$REPO/releases/latest"
  info "Fetching latest release..."

  if command -v curl >/dev/null 2>&1; then
    RELEASE_JSON=$(curl -fsSL "$LATEST_URL")
  elif command -v wget >/dev/null 2>&1; then
    RELEASE_JSON=$(wget -qO- "$LATEST_URL")
  else
    error "Neither curl nor wget found"
  fi

  VERSION=$(echo "$RELEASE_JSON" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
  ASSET_NAME="${BINARY}_${PLATFORM}.${EXT}"
  DOWNLOAD_URL="https://github.com/$REPO/releases/download/$VERSION/$ASSET_NAME"

  info "Latest version: $VERSION"
  info "Platform: $PLATFORM"
}

# Download and install
install() {
  TMP_DIR=$(mktemp -d)
  trap 'rm -rf "$TMP_DIR"' EXIT

  info "Downloading $BINARY $VERSION..."
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$DOWNLOAD_URL" -o "$TMP_DIR/$ASSET_NAME"
  else
    wget -qO "$TMP_DIR/$ASSET_NAME" "$DOWNLOAD_URL"
  fi

  info "Installing to $INSTALL_DIR..."
  if [ "$EXT" = "tar.gz" ]; then
    tar -xzf "$TMP_DIR/$ASSET_NAME" -C "$TMP_DIR"
  elif [ "$EXT" = "zip" ]; then
    unzip -qo "$TMP_DIR/$ASSET_NAME" -d "$TMP_DIR"
  fi

  chmod +x "$TMP_DIR/$BINARY"
  mv "$TMP_DIR/$BINARY" "$INSTALL_DIR/$BINARY"
}

# Main
main() {
  printf "\n${BOLD}Codeany Installer${NC}\n\n"

  detect_platform
  choose_install_dir
  get_download_url
  install
  ensure_path

  printf "\n"
  success "Codeany $VERSION installed to $INSTALL_DIR/$BINARY"
  printf "\n"
  info "Get started:"
  printf "  ${BOLD}codeany${NC}              Start interactive mode\n"
  printf "  ${BOLD}codeany --help${NC}       Show all options\n"
  printf "  ${BOLD}codeany doctor${NC}       Check your environment\n"
  printf "  ${BOLD}codeany update${NC}       Update to latest version\n"
  printf "\n"
}

main "$@"
