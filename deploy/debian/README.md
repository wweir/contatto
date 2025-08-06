# Debian Package Creation

This directory contains the Debian packaging configuration for contatto.

## Requirements

To build Debian packages, you need:

```bash
# Install Debian packaging tools
sudo apt-get update
sudo apt-get install dpkg-dev debhelper golang-go

# For building packages
sudo apt-get install build-essential
```

## Package Building

### Simple Package (recommended for development)
```bash
make deb-simple
```

### Full Debian Package (for distribution)
```bash
make deb
```

### Multi-architecture Packages
```bash
make deb-multi
```

### Clean Packaging Artifacts
```bash
make clean-deb
```

## Package Installation

After building a package:

```bash
# Install the package
sudo dpkg -i contatto_*.deb

# Fix dependencies if needed
sudo apt-get install -f

# Start the service
sudo systemctl enable contatto@contatto
sudo systemctl start contatto@contatto
```

## Package Contents

- Binary: `/usr/bin/contatto`
- Config: `/etc/contatto.toml`
- Service: `/lib/systemd/system/contatto@.service`
- Data directory: `/var/lib/contatto`
- Log directory: `/var/log/contatto`
- System user: `contatto`

## Files Structure

- `debian/control` - Package metadata
- `debian/rules` - Build rules
- `debian/changelog` - Version history
- `debian/copyright` - License information
- `debian/*.postinst` - Post-installation scripts
- `debian/*.prerm` - Pre-removal scripts
- `debian/*.postrm` - Post-removal scripts