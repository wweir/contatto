#!/bin/bash
# Fallback script to create Arch-style packages without makepkg

set -e

PACKAGE_NAME="contatto"
VERSION=${1:-$(git describe --tags --always || echo 'dev')}
# Clean version for Arch (remove 'v' prefix and convert hyphens to dots)
VERSION_CLEAN=$(echo "$VERSION" | sed 's/^v//' | sed 's/-/./g')
ARCH=${2:-x86_64}

echo "Creating Arch-style package (fallback): ${PACKAGE_NAME}-${VERSION_CLEAN}-1-${ARCH}.pkg.tar.xz"

# Build the binary first
make build

# Create package directory structure
PKG_DIR="pkg-arch"
mkdir -p "$PKG_DIR"/{usr/bin,etc,usr/lib/systemd/system,var/lib/contatto,var/log/contatto}
mkdir -p "$PKG_DIR"/usr/lib/{sysusers.d,tmpfiles.d}
mkdir -p "$PKG_DIR"/usr/share/{licenses/$PACKAGE_NAME,doc/$PACKAGE_NAME}

# Copy files
cp bin/contatto "$PKG_DIR/usr/bin/"
cp config/contatto.example.toml "$PKG_DIR/etc/contatto.toml"
cp deploy/systemd/contatto@.service "$PKG_DIR/usr/lib/systemd/system/"

# Create sysusers.d file
cat > "$PKG_DIR/usr/lib/sysusers.d/contatto.conf" << EOF
u contatto - "Contatto proxy daemon" /var/lib/contatto /bin/false
EOF

# Create tmpfiles.d file
cat > "$PKG_DIR/usr/lib/tmpfiles.d/contatto.conf" << EOF
d /var/lib/contatto 0755 contatto contatto -
d /var/log/contatto 0755 contatto contatto -
EOF

# Copy documentation and license
[ -f LICENSE ] && cp LICENSE "$PKG_DIR/usr/share/licenses/$PACKAGE_NAME/"
[ -f README.md ] && cp README.md "$PKG_DIR/usr/share/doc/$PACKAGE_NAME/"
[ -f deploy/README.md ] && cp deploy/README.md "$PKG_DIR/usr/share/doc/$PACKAGE_NAME/DEPLOYMENT.md"

# Create .PKGINFO file
cat > "$PKG_DIR/.PKGINFO" << EOF
pkgname = $PACKAGE_NAME
pkgver = $VERSION_CLEAN-1
pkgdesc = Container registry transparent proxy
url = https://github.com/wweir/contatto
builddate = $(date +%s)
packager = wweir <wweir@wweir.cc>
size = $(du -sb "$PKG_DIR" | cut -f1)
arch = $ARCH
license = MIT
depend = glibc
backup = etc/contatto.toml
EOF

# Create .INSTALL file (install script)
cat > "$PKG_DIR/.INSTALL" << 'EOF'
post_install() {
    # Create system user
    if ! getent passwd contatto >/dev/null; then
        useradd -r -d /var/lib/contatto -s /bin/false -c "Contatto proxy daemon" contatto
    fi
    
    # Create directories with correct ownership
    mkdir -p /var/lib/contatto /var/log/contatto
    chown contatto:contatto /var/lib/contatto /var/log/contatto
    chmod 755 /var/lib/contatto /var/log/contatto
    
    echo "Contatto has been installed."
    echo "To enable and start the service:"
    echo "  sudo systemctl enable contatto@contatto.service"
    echo "  sudo systemctl start contatto@contatto.service"
}

post_upgrade() {
    post_install
    echo "Contatto has been upgraded."
    echo "You may need to restart the service:"
    echo "  sudo systemctl restart contatto@contatto.service"
}

pre_remove() {
    systemctl stop contatto@contatto.service 2>/dev/null || true
    systemctl disable contatto@contatto.service 2>/dev/null || true
}

post_remove() {
    echo "Contatto has been removed."
    echo "The system user and data directories have been preserved."
    echo "To remove them manually:"
    echo "  sudo userdel contatto"
    echo "  sudo rm -rf /var/lib/contatto /var/log/contatto"
}
EOF

# Create .MTREE file (file manifest)
cd "$PKG_DIR"
{
    echo "#mtree"
    echo -n "# "
    date
    find . -type f -exec stat -c "%n mode=%f uid=%u gid=%g size=%s" {} \; | \
    sed 's|^\./||' | sort
    find . -type d -exec stat -c "%n type=dir mode=%f uid=%u gid=%g" {} \; | \
    sed 's|^\./||' | sort
} > .MTREE

# Create the package
cd ..
PACKAGE_FILE="${PACKAGE_NAME}-${VERSION_CLEAN}-1-${ARCH}.pkg.tar.xz"

# Use tar with xz compression
tar -cJf "$PACKAGE_FILE" -C "$PKG_DIR" .

# Clean up
rm -rf "$PKG_DIR"

echo "Arch-style package created: $PACKAGE_FILE"
echo ""
echo "To install (as root):"
echo "  pacman -U $PACKAGE_FILE"
echo ""
echo "To extract and examine:"
echo "  tar -tf $PACKAGE_FILE"