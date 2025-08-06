#!/bin/bash
# Script to create Arch Linux packages for Contatto

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
PACKAGE_NAME="contatto"
VERSION=${1:-$(cd "$PROJECT_ROOT" && git describe --tags --always --dirty || echo 'dev')}

# Clean version for Arch (remove 'v' prefix and convert hyphens to dots)
VERSION_CLEAN=$(echo "$VERSION" | sed 's/^v//' | sed 's/-/./g')
ARCH=${2:-$(uname -m)}

# Map common architectures
case "$ARCH" in
    "x86_64")
        ARCH_PKG="x86_64"
        ;;
    "aarch64")
        ARCH_PKG="aarch64"
        ;;
    "armv7l")
        ARCH_PKG="armv7h"
        ;;
    *)
        ARCH_PKG="$ARCH"
        ;;
esac

echo "Creating Arch Linux package for $PACKAGE_NAME v$VERSION_CLEAN ($ARCH_PKG)"

# Check if makepkg is available
if ! command -v makepkg >/dev/null 2>&1; then
    echo "Error: makepkg not found. This script requires Arch Linux packaging tools."
    echo "Falling back to tar.gz package creation..."
    exec "$PROJECT_ROOT/deploy/scripts/create-package.sh" "$VERSION" "$ARCH"
fi

# Create temporary build directory
BUILD_DIR=$(mktemp -d)
trap 'rm -rf "$BUILD_DIR"' EXIT

cd "$BUILD_DIR"

# Copy PKGBUILD and install files
cp "$SCRIPT_DIR/PKGBUILD.local" PKGBUILD
cp "$SCRIPT_DIR/contatto.install" .

# Replace version placeholder
sed -i "s/VERSION_PLACEHOLDER/$VERSION_CLEAN/g" PKGBUILD

# Build the package
echo "Building package in $BUILD_DIR"
MAKEFLAGS="-j$(nproc)" makepkg -s --noconfirm

# Copy the package to project root
PKG_FILE=$(ls *.pkg.tar.* 2>/dev/null | head -1)
if [ -n "$PKG_FILE" ]; then
    cp "$PKG_FILE" "$PROJECT_ROOT/"
    echo "Package created: $PROJECT_ROOT/$PKG_FILE"
    
    # Show package info
    echo ""
    echo "Package information:"
    pacman -Qip "$PROJECT_ROOT/$PKG_FILE" || true
else
    echo "Error: Package file not found"
    exit 1
fi