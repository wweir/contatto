# Arch Linux Packaging

This directory contains Arch Linux packaging files for Contatto.

## Files

- `PKGBUILD` - Standard PKGBUILD for AUR (requires git)
- `PKGBUILD.local` - Local build PKGBUILD (no git dependency)
- `contatto.install` - Install/upgrade/remove scripts
- `create-arch-package.sh` - Package creation script
- `create-arch-fallback.sh` - Fallback package creator (no makepkg)

## Building Packages

### On Arch Linux (Recommended)
```bash
# Using makepkg
make arch

# Or manually
cd deploy/archlinux
makepkg -si
```

### Fallback Method (Any System)
```bash
# Creates .pkg.tar.xz without makepkg
make arch-fallback
```

## Package Information

- **Package Name**: `contatto`
- **Dependencies**: `glibc`
- **Build Dependencies**: `go`
- **Architecture**: `x86_64`, `aarch64`, `armv7h`

## Installation

### From Built Package
```bash
sudo pacman -U contatto-*.pkg.tar.xz
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
- `/etc/contatto.toml` - Configuration file (backed up on upgrades)
- `/usr/lib/systemd/system/contatto@.service` - systemd service
- `/usr/lib/sysusers.d/contatto.conf` - System user definition
- `/usr/lib/tmpfiles.d/contatto.conf` - Temporary files configuration
- `/var/lib/contatto/` - Data directory
- `/var/log/contatto/` - Log directory

## AUR Submission

The `PKGBUILD` file is ready for AUR submission:

1. Create AUR package repository
2. Copy `PKGBUILD` and `contatto.install`
3. Generate `.SRCINFO`: `makepkg --printsrcinfo > .SRCINFO`
4. Commit and push to AUR

## Development

### Testing Package
```bash
# Build and install locally
cd deploy/archlinux
makepkg -si

# Remove for testing
sudo pacman -R contatto
```

### Updating Version
The version is automatically detected from git tags, or can be specified:
```bash
./create-arch-package.sh 1.2.3
```