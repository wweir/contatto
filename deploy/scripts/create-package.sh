#!/bin/bash
# Fallback script to create tar.gz packages with deb structure when dpkg-deb is not available

set -e

PACKAGE_NAME="contatto"
VERSION=${1:-$(git describe --tags --always || echo 'dev')}
ARCH=${2:-amd64}

echo "Creating fallback package: ${PACKAGE_NAME}_${VERSION}_${ARCH}.tar.gz"

# Build the binary
make build

# Create package structure  
mkdir -p pkg-fallback/usr/bin
mkdir -p pkg-fallback/etc
mkdir -p pkg-fallback/lib/systemd/system
mkdir -p pkg-fallback/var/lib/contatto
mkdir -p pkg-fallback/var/log/contatto
mkdir -p pkg-fallback/DEBIAN

# Copy files
cp bin/contatto pkg-fallback/usr/bin/
cp config/contatto.example.toml pkg-fallback/etc/contatto.toml
cp deploy/systemd/contatto@.service pkg-fallback/lib/systemd/system/

# Create control file
cat > pkg-fallback/DEBIAN/control << EOF
Package: ${PACKAGE_NAME}
Version: ${VERSION}
Section: net
Priority: optional
Architecture: ${ARCH}
Maintainer: wweir <wweir@wweir.cc>
Description: Container registry transparent proxy
 Contatto is a transparent proxy for container registries that provides
 registry mirroring and access optimization for Docker, containerd and
 other container runtimes.
Homepage: https://github.com/wweir/contatto
EOF

# Create installation script
cat > pkg-fallback/install.sh << 'EOF'
#!/bin/bash
# Installation script for contatto

set -e

echo "Installing contatto..."

# Copy files to their destinations
sudo cp usr/bin/contatto /usr/bin/contatto
sudo chmod +x /usr/bin/contatto

sudo cp etc/contatto.toml /etc/contatto.toml
sudo cp lib/systemd/system/contatto@.service /lib/systemd/system/contatto@.service

# Create directories
sudo mkdir -p /var/lib/contatto /var/log/contatto

# Create user
if ! getent passwd contatto >/dev/null; then
    sudo adduser --system --group --home /var/lib/contatto \
                 --no-create-home --disabled-password --disabled-login \
                 --gecos "Contatto proxy daemon" contatto
fi

# Set ownership
sudo chown -R contatto:contatto /var/lib/contatto /var/log/contatto

# Enable systemd service
if [ -d /run/systemd/system ]; then
    sudo systemctl daemon-reload
    echo "To enable and start the service, run:"
    echo "  sudo systemctl enable contatto@contatto"
    echo "  sudo systemctl start contatto@contatto"
fi

echo "Installation completed!"
EOF

chmod +x pkg-fallback/install.sh

# Create uninstallation script
cat > pkg-fallback/uninstall.sh << 'EOF'
#!/bin/bash
# Uninstallation script for contatto

set -e

echo "Uninstalling contatto..."

# Stop and disable service
if [ -d /run/systemd/system ]; then
    sudo systemctl stop contatto@contatto 2>/dev/null || true
    sudo systemctl disable contatto@contatto 2>/dev/null || true
fi

# Remove files
sudo rm -f /usr/bin/contatto
sudo rm -f /etc/contatto.toml
sudo rm -f /lib/systemd/system/contatto@.service

# Note: We don't remove user and directories automatically for safety
echo "User 'contatto' and directories /var/lib/contatto, /var/log/contatto were preserved."
echo "To remove them manually:"
echo "  sudo deluser contatto"
echo "  sudo rm -rf /var/lib/contatto /var/log/contatto"

echo "Uninstallation completed!"
EOF

chmod +x pkg-fallback/uninstall.sh

# Create README
cat > pkg-fallback/README.txt << EOF
Contatto ${VERSION} Installation Package

This package contains:
- contatto binary
- systemd service file
- configuration file
- installation/uninstallation scripts

To install:
  ./install.sh

To uninstall:
  ./uninstall.sh

For more information, visit: https://github.com/wweir/contatto
EOF

# Create the package
tar czf "${PACKAGE_NAME}_${VERSION}_${ARCH}.tar.gz" -C pkg-fallback .
rm -rf pkg-fallback

echo "Package created: ${PACKAGE_NAME}_${VERSION}_${ARCH}.tar.gz"
echo "Extract and run ./install.sh to install"