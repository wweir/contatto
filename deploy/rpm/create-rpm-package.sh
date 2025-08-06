#!/bin/bash
# Script to create RPM packages for Contatto

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
PACKAGE_NAME="contatto"
VERSION=${1:-$(cd "$PROJECT_ROOT" && git describe --tags --always --dirty || echo 'dev')}

# Clean version for RPM (remove 'v' prefix and convert hyphens to dots for pre-release versions)
VERSION_CLEAN=$(echo "$VERSION" | sed 's/^v//' | sed 's/-/./g')
ARCH=${2:-$(uname -m)}

echo "Creating RPM package for $PACKAGE_NAME v$VERSION_CLEAN ($ARCH)"

# Check if rpmbuild is available
if ! command -v rpmbuild >/dev/null 2>&1; then
    echo "Error: rpmbuild not found. This script requires RPM packaging tools."
    echo "On CentOS/RHEL/Fedora, install with: sudo dnf install rpm-build"
    echo "Falling back to tar.gz package creation..."
    exec "$PROJECT_ROOT/deploy/scripts/create-package.sh" "$VERSION" "$ARCH"
fi

# Setup RPM build environment
RPM_TOPDIR=$(mktemp -d)
trap 'rm -rf "$RPM_TOPDIR"' EXIT

mkdir -p "$RPM_TOPDIR"/{BUILD,BUILDROOT,RPMS,SOURCES,SPECS,SRPMS}

echo "Using RPM build directory: $RPM_TOPDIR"

# Copy spec file and update version
cp "$SCRIPT_DIR/contatto.spec.local" "$RPM_TOPDIR/SPECS/contatto.spec"
sed -i "s/VERSION_PLACEHOLDER/$VERSION_CLEAN/g" "$RPM_TOPDIR/SPECS/contatto.spec"

# Create empty source directory (for local build)
mkdir -p "$RPM_TOPDIR/BUILD/$PACKAGE_NAME-$VERSION_CLEAN"

# Build the RPM package
cd "$RPM_TOPDIR"
rpmbuild --define "_topdir $RPM_TOPDIR" -bb SPECS/contatto.spec

# Copy the built RPM to project root
RPM_FILE=$(find "$RPM_TOPDIR/RPMS" -name "*.rpm" -type f | head -1)
if [ -n "$RPM_FILE" ]; then
    cp "$RPM_FILE" "$PROJECT_ROOT/"
    RPM_BASENAME=$(basename "$RPM_FILE")
    echo "RPM package created: $PROJECT_ROOT/$RPM_BASENAME"
    
    # Show package info
    echo ""
    echo "Package information:"
    rpm -qip "$PROJECT_ROOT/$RPM_BASENAME" 2>/dev/null || true
    echo ""
    echo "Package contents:"
    rpm -qlp "$PROJECT_ROOT/$RPM_BASENAME" 2>/dev/null || true
else
    echo "Error: RPM package file not found"
    exit 1
fi