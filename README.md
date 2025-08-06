# Contatto

[简体中文](README.zh-Hans.md)

Contatto is a transparent proxy for container registries, designed to:

1. Solve slow access issues with Docker Hub, Quay.io, k8s.gcr.io and other registries, improving image pull speed.
2. Solve large-scale image distribution problems, reducing pressure on upstream registries.

## Installation

### From Release Packages

Download the appropriate package for your system from the [releases page](https://github.com/wweir/contatto/releases):

#### Debian/Ubuntu (.deb)
```bash
wget https://github.com/wweir/contatto/releases/download/v<version>/contatto_<version>_amd64.deb
sudo dpkg -i contatto_<version>_amd64.deb
sudo apt-get install -f  # Fix dependencies if needed
```

#### Arch Linux (.pkg.tar.xz)
```bash
wget https://github.com/wweir/contatto/releases/download/v<version>/contatto-<version>-x86_64.pkg.tar.xz
sudo pacman -U contatto-<version>-x86_64.pkg.tar.xz
```

#### CentOS/RHEL/Fedora (.rpm)
```bash
wget https://github.com/wweir/contatto/releases/download/v<version>/contatto-<version>-x86_64.rpm
sudo rpm -ivh contatto-<version>-x86_64.rpm
# or
sudo dnf install contatto-<version>-x86_64.rpm
```

#### Generic Linux (.tar.gz)
```bash
wget https://github.com/wweir/contatto/releases/download/v<version>/contatto-linux-amd64.tar.gz
tar -xzf contatto-linux-amd64.tar.gz
sudo ./install.sh  # or manually copy files
```

### From Source

1. Clone the repository
2. Build the binary: `make build`
3. Copy the binary to your desired location
4. Configure the service (see [Configuration](#configuration))

## Configuration

Contatto supports configuration files in JSON, YAML, and TOML formats. By default, it uses TOML format.

Example configuration file (`/etc/contatto.toml`):

```toml
# See config/contatto.example.toml for full configuration options
```

## Usage

### As a System Service

After installation, enable and start the service:

```bash
# Enable the service
sudo systemctl enable contatto@contatto.service

# Start the service  
sudo systemctl start contatto@contatto.service

# Check status
sudo systemctl status contatto@contatto.service
```

## Development

### Building

```bash
# Build the binary
make build

# Run tests
make test

# Build packages
make deb-simple     # Debian package
make rpm-fallback   # RPM package  
make arch-fallback  # Arch package
```

### GitHub Actions

This project uses GitHub Actions for automated testing and package building. The CI/CD pipeline:

1. **master.yaml**: Runs tests on every push to main/master branch
2. **release.yaml**: Builds packages for multiple architectures on tag pushes

Recent fixes include correcting the Arch Linux package build path from `deploy/arch/` to `deploy/archlinux/`.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
