# Contatto Deployment

This directory contains all packaging and deployment scripts for Contatto.

## Directory Structure

```
deploy/
├── debian/          # Debian/Ubuntu packaging files
├── archlinux/       # Arch Linux packaging files
├── scripts/         # Packaging and installation scripts  
├── systemd/         # systemd service files
└── README.md        # This file
```

## Packaging Options

### 1. Debian/Ubuntu Packages (.deb)

#### Full Debian Package
```bash
make deb
```
Requires: `dpkg-buildpackage`, `debhelper`, `golang-go`

#### Simple Debian Package  
```bash
make deb-simple
```
Auto-fallback to tar.gz if dpkg tools unavailable.

#### Multi-Architecture Packages
```bash
make deb-multi
```
Builds for: amd64, arm64, armhf

### 2. Arch Linux Packages (.pkg.tar.xz)

#### Arch Linux Package
```bash
make arch
```
Requires: `makepkg`, `golang` (auto-fallback if unavailable)

#### Arch Linux Fallback
```bash
make arch-fallback
```
Creates .pkg.tar.xz without makepkg, works on any system.

#### Multi-Architecture Arch Packages
```bash
make arch-multi
```
Builds for: x86_64, aarch64, armv7h

### 3. Fallback Package (.tar.gz)
```bash
make pkg-fallback
```
Creates tar.gz with installation scripts, works on any Linux system.

## Installation

### From .deb Package
```bash
sudo dpkg -i contatto_*.deb
sudo apt-get install -f  # Fix dependencies if needed
```

### From .pkg.tar.xz Package
```bash
sudo pacman -U contatto-*.pkg.tar.xz
```

### From .tar.gz Package
```bash
tar -xzf contatto_*.tar.gz
cd contatto_*
sudo ./install.sh
```

### Service Management
```bash
# Enable and start
sudo systemctl enable contatto@contatto
sudo systemctl start contatto@contatto

# Check status
sudo systemctl status contatto@contatto

# View logs
sudo journalctl -u contatto@contatto -f
```

## Package Contents

- **Binary**: `/usr/bin/contatto`
- **Config**: `/etc/contatto.toml`
- **Service**: `/lib/systemd/system/contatto@.service` (Debian) or `/usr/lib/systemd/system/contatto@.service` (Arch)
- **Data**: `/var/lib/contatto`
- **Logs**: `/var/log/contatto`
- **User**: `contatto` (system user)

## Development

### Building Requirements

#### Debian/Ubuntu
```bash
sudo apt-get install golang-go dpkg-dev debhelper build-essential
```

#### Arch Linux
```bash
sudo pacman -S go base-devel
```

#### For all systems
```bash
make deb-simple   # Will use fallback if tools unavailable
make arch-fallback # Will create .pkg.tar.xz without makepkg
```

### Manual Installation (Development)
```bash
make install  # Install directly to system
```

### Cleaning
```bash
make clean-deb    # Clean Debian packaging artifacts
make clean-arch   # Clean Arch packaging artifacts
make clean-all    # Clean all packaging artifacts
```

## Files Description

### debian/
- `control` - Package metadata and dependencies
- `rules` - Build rules for dpkg-buildpackage
- `changelog` - Version history
- `copyright` - License information
- `*.postinst/.prerm/.postrm` - Installation lifecycle scripts

### archlinux/
- `PKGBUILD` - Standard PKGBUILD for AUR
- `PKGBUILD.local` - Local build PKGBUILD
- `contatto.install` - Install/upgrade/remove scripts
- `create-arch-package.sh` - Package creation script
- `create-arch-fallback.sh` - Fallback package creator

### scripts/
- `create-package.sh` - Fallback package creation script

### systemd/
- `contatto@.service` - systemd service template for per-user instances