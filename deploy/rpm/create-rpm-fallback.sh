#!/bin/bash
# Fallback script to create RPM-style packages without rpmbuild

set -e

PACKAGE_NAME="contatto"
VERSION=${1:-$(git describe --tags --always || echo 'dev')}
# Clean version for RPM (remove 'v' prefix and convert hyphens to dots)
VERSION_CLEAN=$(echo "$VERSION" | sed 's/^v//' | sed 's/-/./g')
ARCH=${2:-x86_64}
RELEASE="1"

echo "Creating RPM-style package (fallback): ${PACKAGE_NAME}-${VERSION_CLEAN}-${RELEASE}.${ARCH}.rpm"

# Build the binary first
make build

# Create package directory structure (following RPM conventions)
PKG_DIR="pkg-rpm"
mkdir -p "$PKG_DIR"/{usr/bin,etc,usr/lib/systemd/system,var/lib/contatto,var/log/contatto}
mkdir -p "$PKG_DIR"/usr/lib/{sysusers.d,tmpfiles.d}
mkdir -p "$PKG_DIR"/usr/share/{licenses/$PACKAGE_NAME,doc/$PACKAGE_NAME}

# Copy files
cp bin/contatto "$PKG_DIR/usr/bin/"
cp config/contatto.example.toml "$PKG_DIR/etc/contatto.toml"
cp deploy/systemd/contatto@.service "$PKG_DIR/usr/lib/systemd/system/"

# Create sysusers.d file
cat > "$PKG_DIR/usr/lib/sysusers.d/contatto.conf" << EOF
u contatto - "Contatto proxy daemon" /var/lib/contatto /sbin/nologin
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

# Create RPM-style metadata files
mkdir -p "$PKG_DIR/rpm-metadata"

# Create package info (similar to RPM database format)
cat > "$PKG_DIR/rpm-metadata/package-info" << EOF
Name: $PACKAGE_NAME
Version: $VERSION_CLEAN
Release: $RELEASE
Architecture: $ARCH
Summary: Container registry transparent proxy
License: MIT
URL: https://github.com/wweir/contatto
Packager: wweir <wweir@wweir.cc>
Build Date: $(date)
Description: Contatto is a transparent proxy for container registries that provides
 registry mirroring and access optimization for Docker, containerd and
 other container runtimes.
EOF

# Create installation script
cat > "$PKG_DIR/rpm-metadata/install.sh" << 'EOF'
#!/bin/bash
# RPM-style installation script for contatto

set -e

echo "Installing contatto..."

# Create system user and group
if ! getent group contatto >/dev/null; then
    groupadd -r contatto
fi
if ! getent passwd contatto >/dev/null; then
    useradd -r -g contatto -d /var/lib/contatto -s /sbin/nologin \
            -c "Contatto proxy daemon" contatto
fi

# Copy files to their destinations
cp -r usr/* /usr/
cp -r etc/* /etc/
mkdir -p /var/lib/contatto /var/log/contatto

# Set ownership and permissions
chown -R contatto:contatto /var/lib/contatto /var/log/contatto
chmod 755 /var/lib/contatto /var/log/contatto
chmod +x /usr/bin/contatto

# Handle systemd service
if [ -d /run/systemd/system ]; then
    systemctl daemon-reload
    echo "To enable and start the service, run:"
    echo "  sudo systemctl enable contatto@contatto.service"
    echo "  sudo systemctl start contatto@contatto.service"
fi

echo "Installation completed!"
echo ""
echo "Configuration file: /etc/contatto.toml"
echo "Edit this file to configure your registry mirrors and rules."
EOF

chmod +x "$PKG_DIR/rpm-metadata/install.sh"

# Create uninstallation script
cat > "$PKG_DIR/rpm-metadata/uninstall.sh" << 'EOF'
#!/bin/bash
# RPM-style uninstallation script for contatto

set -e

echo "Uninstalling contatto..."

# Stop and disable service
if [ -d /run/systemd/system ]; then
    systemctl stop contatto@contatto.service 2>/dev/null || true
    systemctl disable contatto@contatto.service 2>/dev/null || true
fi

# Remove files (be careful with config file)
rm -f /usr/bin/contatto
rm -f /usr/lib/systemd/system/contatto@.service
rm -f /usr/lib/sysusers.d/contatto.conf
rm -f /usr/lib/tmpfiles.d/contatto.conf

# Ask about config file
if [ -f /etc/contatto.toml ]; then
    echo "Configuration file /etc/contatto.toml preserved."
    echo "To remove it: sudo rm /etc/contatto.toml"
fi

# Note about user and directories
echo "User 'contatto' and directories /var/lib/contatto, /var/log/contatto were preserved."
echo "To remove them manually:"
echo "  sudo userdel contatto"
echo "  sudo rm -rf /var/lib/contatto /var/log/contatto"

# Reload systemd
if [ -d /run/systemd/system ]; then
    systemctl daemon-reload
fi

echo "Uninstallation completed!"
EOF

chmod +x "$PKG_DIR/rpm-metadata/uninstall.sh"

# Create file manifest
find "$PKG_DIR" -type f ! -path "*/rpm-metadata/*" | \
    sed "s|^$PKG_DIR||" | sort > "$PKG_DIR/rpm-metadata/file-list.txt"

# Create README
cat > "$PKG_DIR/README.txt" << EOF
Contatto ${VERSION_CLEAN} RPM-style Installation Package

This package contains:
- contatto binary
- systemd service file
- configuration file
- installation/uninstallation scripts

To install:
  sudo ./rpm-metadata/install.sh

To uninstall:
  sudo ./rpm-metadata/uninstall.sh

Package Information:
$(cat "$PKG_DIR/rpm-metadata/package-info")

For more information, visit: https://github.com/wweir/contatto
EOF

# Create the package (using tar format since cpio might not be available)
cd "$PKG_DIR"
tar -czf "../${PACKAGE_NAME}-${VERSION_CLEAN}-${RELEASE}.${ARCH}.rpm" .
cd ..

# Clean up
rm -rf "$PKG_DIR"

echo "RPM-style package created: ${PACKAGE_NAME}-${VERSION_CLEAN}-${RELEASE}.${ARCH}.rpm"
echo ""
echo "To install:"
echo "  # Extract the package:"
echo "  tar -xzf ${PACKAGE_NAME}-${VERSION_CLEAN}-${RELEASE}.${ARCH}.rpm"
echo "  # Run installation script:"
echo "  sudo ./rpm-metadata/install.sh"
echo ""
echo "Or on systems with rpm support:"
echo "  sudo rpm -ivh ${PACKAGE_NAME}-${VERSION_CLEAN}-${RELEASE}.${ARCH}.rpm"