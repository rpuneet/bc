#!/bin/bash
# mycel installer script
# Usage: curl -fsSL https://raw.githubusercontent.com/rpuneet/mycel/main/scripts/install.sh | bash

set -e

# Configuration
REPO="rpuneet/mycel"
INSTALL_DIR="${MYCEL_INSTALL_DIR:-/usr/local/bin}"
BINARY_NAME="mycel"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Print with arrow prefix
info() {
    echo -e "${CYAN}→${NC} $1"
}

success() {
    echo -e "${GREEN}✓${NC} $1"
}

warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

error() {
    echo -e "${RED}✗${NC} $1"
    exit 1
}

# Detect OS and architecture
detect_platform() {
    OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
    ARCH="$(uname -m)"

    case "$OS" in
        linux)
            OS="linux"
            ;;
        darwin)
            OS="darwin"
            ;;
        *)
            error "Unsupported operating system: $OS"
            ;;
    esac

    case "$ARCH" in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        arm64|aarch64)
            ARCH="arm64"
            ;;
        *)
            error "Unsupported architecture: $ARCH"
            ;;
    esac

    # Linux arm64 builds are not yet available
    if [ "$OS" = "linux" ] && [ "$ARCH" = "arm64" ]; then
        error "Linux arm64 builds are not yet available. Please build from source: go install github.com/rpuneet/mycel/cmd/mycel@latest"
    fi

    info "Detecting OS... ${OS} ${ARCH}"
}

# Get latest release version
get_latest_version() {
    info "Fetching latest version..."
    VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')

    if [ -z "$VERSION" ]; then
        error "Failed to fetch latest version. Check your network connection."
    fi

    info "Latest version: ${VERSION}"
}

# Download and install binary
download_and_install() {
    # Archive naming: mycel_VERSION_OS_ARCH.tar.gz (version without leading v)
    VERSION_NUM="${VERSION#v}"
    ARCHIVE_NAME="mycel_${VERSION_NUM}_${OS}_${ARCH}.tar.gz"
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE_NAME}"
    TMP_DIR=$(mktemp -d)

    info "Downloading mycel ${VERSION}..."

    if ! curl -fsSL "$DOWNLOAD_URL" -o "${TMP_DIR}/${ARCHIVE_NAME}"; then
        rm -rf "$TMP_DIR"
        error "Failed to download mycel. Check if release exists for ${OS}/${ARCH}."
    fi

    # Verify checksum if available
    CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"
    if curl -fsSL "$CHECKSUMS_URL" -o "${TMP_DIR}/checksums.txt" 2>/dev/null; then
        info "Verifying checksum..."
        (cd "$TMP_DIR" && shasum -a 256 -c checksums.txt --ignore-missing 2>/dev/null) || \
        (cd "$TMP_DIR" && sha256sum -c checksums.txt --ignore-missing 2>/dev/null) || \
            warn "Checksum verification failed — continuing anyway"
    fi

    info "Extracting archive..."
    tar -xzf "${TMP_DIR}/${ARCHIVE_NAME}" -C "$TMP_DIR"

    TMP_FILE="${TMP_DIR}/mycel"
    if [ ! -f "$TMP_FILE" ]; then
        rm -rf "$TMP_DIR"
        error "Binary not found in archive."
    fi

    chmod +x "$TMP_FILE"

    info "Installing to ${INSTALL_DIR}..."

    # Check if we need sudo
    if [ -w "$INSTALL_DIR" ]; then
        mv "$TMP_FILE" "${INSTALL_DIR}/${BINARY_NAME}"
    else
        warn "Need elevated permissions to install to ${INSTALL_DIR}"
        sudo mv "$TMP_FILE" "${INSTALL_DIR}/${BINARY_NAME}"
    fi

    rm -rf "$TMP_DIR"
}

# Verify installation
verify_installation() {
    info "Verifying installation..."

    if [ -x "${INSTALL_DIR}/${BINARY_NAME}" ]; then
        success "mycel installed successfully!"
    else
        error "Installation failed. Binary not found."
    fi
}

# Check dependencies
check_dependencies() {
    if ! command -v tmux &> /dev/null; then
        warn "tmux is not installed (required for mycel agents)"
        echo ""
        case "$OS" in
            darwin)
                echo "  Install with: brew install tmux"
                ;;
            linux)
                echo "  Install with: apt install tmux  # or your package manager"
                ;;
        esac
        echo ""
    fi
}

# Print next steps
print_next_steps() {
    echo ""
    success "Run 'mycel' to get started."
    echo ""
    echo "Quick start:"
    echo "  mycel init          # Initialize workspace"
    echo "  mycel up            # Start server"
    echo "  mycel up -d         # Start as daemon"
    echo ""
    echo "Documentation: https://github.com/${REPO}#readme"
}

# Main
main() {
    echo ""
    echo "mycel Installer"
    echo "==============="
    echo ""

    detect_platform
    get_latest_version
    download_and_install
    verify_installation
    check_dependencies
    print_next_steps
}

main "$@"
