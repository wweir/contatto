# CentOS/RHEL RPM Packaging

This directory contains RPM packaging files for Contatto on CentOS, RHEL, Fedora, and other RPM-based distributions.

## Files

- `contatto.spec` - Standard RPM spec file for building from source tarball
- `contatto.spec.local` - Local build spec file (no source tarball required)
- `create-rpm-package.sh` - RPM package creation script
- `create-rpm-fallback.sh` - Fallback package creator (no rpmbuild)

## Building Packages

### On CentOS/RHEL/Fedora (Recommended)
```bash
# Install build tools
sudo dnf install rpm-build golang

# Build RPM package
make rpm
```

### Fallback Method (Any System)
```bash
# Creates .rpm file without rpmbuild
make rpm-fallback
```

## Package Information

- **Package Name**: `contatto`
- **Dependencies**: `systemd`, `shadow-utils`
- **Build Dependencies**: `golang >= 1.20`, `systemd-rpm-macros`
- **Architecture**: `x86_64`, `aarch64`, `armv7hl`

## Installation

### From Built RPM Package
```bash
# CentOS/RHEL 8+, Fedora
sudo dnf install contatto-*.rpm

# CentOS/RHEL 7
sudo yum install contatto-*.rpm

# Or using rpm directly
sudo rpm -ivh contatto-*.rpm
```

### Post-Installation
```bash
# Enable and start service
sudo systemctl enable contatto@contatto.service
sudo systemctl start contatto@contatto.service

# Check status
sudo systemctl status contatto@contatto.service
```

## Package Contents

- `/usr/bin/contatto` - Main binary
- `/etc/contatto.toml` - Configuration file (noreplace)
- `/usr/lib/systemd/system/contatto@.service` - systemd service
- `/usr/lib/sysusers.d/contatto.conf` - System user definition
- `/usr/lib/tmpfiles.d/contatto.conf` - Temporary files configuration
- `/var/lib/contatto/` - Data directory
- `/var/log/contatto/` - Log directory

## RPM Scriptlets

### %pre (Pre-installation)
- Creates `contatto` system user and group

### %post (Post-installation)
- Enables systemd service integration
- Creates directories with proper ownership

### %preun (Pre-uninstallation)
- Stops and disables running services

### %postun (Post-uninstallation)
- Handles service cleanup
- Preserves user data and configuration

## Development

### Testing Package
```bash
# Build and install locally
make rpm
sudo dnf install contatto-*.rpm

# Remove for testing
sudo dnf remove contatto
```

### Building for Different Architectures
```bash
# Specify architecture
./deploy/rpm/create-rpm-package.sh 1.2.3 aarch64
```

### Updating Version
The version is automatically detected from git tags, or can be specified:
```bash
./deploy/rpm/create-rpm-package.sh 1.2.3
```

## Distribution-Specific Notes

### CentOS/RHEL 7
- Uses `/usr/lib/systemd/system/` for service files
- Requires `systemd` and `shadow-utils` packages

### CentOS/RHEL 8+
- Modern systemd with sysusers.d and tmpfiles.d support
- Uses `dnf` package manager

### Fedora
- Latest systemd features supported
- Similar to CentOS/RHEL 8+ but more current versions

## Repository Setup

For enterprise deployment, you can create a YUM/DNF repository:

```bash
# Create repository directory
mkdir -p /var/www/html/contatto-repo

# Copy RPM packages
cp contatto-*.rpm /var/www/html/contatto-repo/

# Create repository metadata
createrepo /var/www/html/contatto-repo/

# Create repo file
cat > /etc/yum.repos.d/contatto.repo << EOF
[contatto]
name=Contatto Repository
baseurl=http://your-server/contatto-repo/
gpgcheck=0
enabled=1
EOF
```