MAKEFLAGS += --jobs all
GO:=CGO_ENABLED=0 GODEBUG=httpmuxgo121=1 go

# Package information
PACKAGE_NAME := contatto
VERSION := $(shell git describe --tags --always || echo 'dev')
ARCHITECTURE := $(shell dpkg --print-architecture 2>/dev/null || echo 'amd64')

default: test build

test:
	${GO} vet ./...
	${GO} test ./...

build:
	${GO} build -trimpath -ldflags "\
		-X github.com/wweir/contatto/config.Version=$(VERSION) \
		-X github.com/wweir/contatto/config.Date=$(shell date +%Y-%m-%d)" \
		-o bin/contatto ./cmd/contatto

run: build
	./bin/contatto proxy --debug -c config/contatto.toml

install: build
	sudo install -m 0755 ./bin/contatto /usr/bin/contatto
	sudo install -d /etc /var/lib/contatto /var/log/contatto
	sudo install -m 0644 config/contatto.example.toml /etc/contatto.toml
	sudo install -m 0644 deploy/systemd/contatto@.service /lib/systemd/system/contatto@.service
	sudo systemctl daemon-reload

clean:
	rm -f ./bin/contatto
	rm -f $(PACKAGE_NAME)_*.deb
	rm -f $(PACKAGE_NAME)-*.pkg.tar.*
	rm -f $(PACKAGE_NAME)-*.rpm
	rm -rf deploy/debian/$(PACKAGE_NAME)
	rm -rf deploy/debian/.debhelper
	rm -f deploy/debian/debhelper-build-stamp
	rm -f deploy/debian/files

# Debian packaging targets
deb: clean-deb
	@echo "Building Debian package..."
	cd deploy && dpkg-buildpackage -us -uc -b

deb-source: clean-deb  
	@echo "Building Debian source package..."
	cd deploy && dpkg-buildpackage -us -uc -S

# Build deb package using dpkg-deb (simpler approach)
deb-simple: build
	@echo "Creating simple Debian package..."
	@if command -v dpkg-deb >/dev/null 2>&1; then \
		mkdir -p debian-pkg/DEBIAN; \
		mkdir -p debian-pkg/usr/bin; \
		mkdir -p debian-pkg/etc; \
		mkdir -p debian-pkg/lib/systemd/system; \
		mkdir -p debian-pkg/var/lib/contatto; \
		mkdir -p debian-pkg/var/log/contatto; \
		\
		cp bin/contatto debian-pkg/usr/bin/; \
		cp config/contatto.example.toml debian-pkg/etc/contatto.toml; \
		cp deploy/systemd/contatto@.service debian-pkg/lib/systemd/system/; \
		\
		echo "Package: $(PACKAGE_NAME)" > debian-pkg/DEBIAN/control; \
		echo "Version: $(VERSION)" >> debian-pkg/DEBIAN/control; \
		echo "Section: net" >> debian-pkg/DEBIAN/control; \
		echo "Priority: optional" >> debian-pkg/DEBIAN/control; \
		echo "Architecture: $(ARCHITECTURE)" >> debian-pkg/DEBIAN/control; \
		echo "Maintainer: wweir <wweir@wweir.cc>" >> debian-pkg/DEBIAN/control; \
		echo "Description: Container registry transparent proxy" >> debian-pkg/DEBIAN/control; \
		echo " Contatto is a transparent proxy for container registries that provides" >> debian-pkg/DEBIAN/control; \
		echo " registry mirroring and access optimization." >> debian-pkg/DEBIAN/control; \
		echo "Homepage: https://github.com/wweir/contatto" >> debian-pkg/DEBIAN/control; \
		\
		echo "#!/bin/sh" > debian-pkg/DEBIAN/postinst; \
		echo "set -e" >> debian-pkg/DEBIAN/postinst; \
		echo "if ! getent passwd contatto >/dev/null; then" >> debian-pkg/DEBIAN/postinst; \
		echo "    adduser --system --group --home /var/lib/contatto --no-create-home --disabled-password --disabled-login --gecos 'Contatto proxy daemon' contatto" >> debian-pkg/DEBIAN/postinst; \
		echo "fi" >> debian-pkg/DEBIAN/postinst; \
		echo "chown -R contatto:contatto /var/lib/contatto /var/log/contatto" >> debian-pkg/DEBIAN/postinst; \
		echo "if [ -d /run/systemd/system ]; then systemctl daemon-reload; fi" >> debian-pkg/DEBIAN/postinst; \
		chmod +x debian-pkg/DEBIAN/postinst; \
		\
		echo "#!/bin/sh" > debian-pkg/DEBIAN/prerm; \
		echo "set -e" >> debian-pkg/DEBIAN/prerm; \
		echo "if [ -d /run/systemd/system ] && systemctl is-active contatto@contatto >/dev/null 2>&1; then" >> debian-pkg/DEBIAN/prerm; \
		echo "    systemctl stop contatto@contatto" >> debian-pkg/DEBIAN/prerm; \
		echo "fi" >> debian-pkg/DEBIAN/prerm; \
		chmod +x debian-pkg/DEBIAN/prerm; \
		\
		dpkg-deb --build debian-pkg $(PACKAGE_NAME)_$(VERSION)_$(ARCHITECTURE).deb; \
		rm -rf debian-pkg; \
		echo "Package created: $(PACKAGE_NAME)_$(VERSION)_$(ARCHITECTURE).deb"; \
	else \
		echo "dpkg-deb not found, creating fallback package..."; \
		./deploy/scripts/create-package.sh $(VERSION) $(ARCHITECTURE); \
	fi

# Fallback package creation (tar.gz with install scripts)
pkg-fallback: build
	@echo "Creating fallback installation package..."
	./deploy/scripts/create-package.sh $(VERSION) $(ARCHITECTURE)

# Multi-architecture deb packages
deb-multi: clean
	@echo "Building multi-architecture packages..."
	$(MAKE) deb-arch GOARCH=amd64 ARCH=amd64
	$(MAKE) deb-arch GOARCH=arm64 ARCH=arm64  
	$(MAKE) deb-arch GOARCH=arm GOARM=7 ARCH=armhf

deb-arch:
	@echo "Building for $(ARCH)..."
	GOOS=linux GOARCH=$(GOARCH) $(GO) build -trimpath -ldflags "\
		-X github.com/wweir/contatto/config.Version=$(VERSION) \
		-X github.com/wweir/contatto/config.Date=$(shell date +%Y-%m-%d)" \
		-o bin/contatto-$(ARCH) ./cmd/contatto
	
	mkdir -p debian-pkg-$(ARCH)/DEBIAN
	mkdir -p debian-pkg-$(ARCH)/usr/bin
	mkdir -p debian-pkg-$(ARCH)/etc
	mkdir -p debian-pkg-$(ARCH)/lib/systemd/system
	mkdir -p debian-pkg-$(ARCH)/var/lib/contatto
	mkdir -p debian-pkg-$(ARCH)/var/log/contatto
	
	cp bin/contatto-$(ARCH) debian-pkg-$(ARCH)/usr/bin/contatto
	cp config/contatto.example.toml debian-pkg-$(ARCH)/etc/contatto.toml
	cp deploy/systemd/contatto@.service debian-pkg-$(ARCH)/lib/systemd/system/
	
	@echo "Package: $(PACKAGE_NAME)" > debian-pkg-$(ARCH)/DEBIAN/control
	@echo "Version: $(VERSION)" >> debian-pkg-$(ARCH)/DEBIAN/control
	@echo "Section: net" >> debian-pkg-$(ARCH)/DEBIAN/control
	@echo "Priority: optional" >> debian-pkg-$(ARCH)/DEBIAN/control
	@echo "Architecture: $(ARCH)" >> debian-pkg-$(ARCH)/DEBIAN/control
	@echo "Maintainer: wweir <wweir@wweir.cc>" >> debian-pkg-$(ARCH)/DEBIAN/control
	@echo "Description: Container registry transparent proxy" >> debian-pkg-$(ARCH)/DEBIAN/control
	@echo " Contatto is a transparent proxy for container registries." >> debian-pkg-$(ARCH)/DEBIAN/control
	@echo "Homepage: https://github.com/wweir/contatto" >> debian-pkg-$(ARCH)/DEBIAN/control
	
	dpkg-deb --build debian-pkg-$(ARCH) $(PACKAGE_NAME)_$(VERSION)_$(ARCH).deb
	rm -rf debian-pkg-$(ARCH) bin/contatto-$(ARCH)

# Arch Linux packaging targets
arch: build
	@echo "Creating Arch Linux package..."
	@if command -v makepkg >/dev/null 2>&1; then \
		./deploy/archlinux/create-arch-package.sh $(VERSION) $(ARCHITECTURE); \
	else \
		echo "makepkg not found, creating fallback package..."; \
		./deploy/archlinux/create-arch-fallback.sh $(VERSION) $(ARCHITECTURE); \
	fi

# Arch Linux fallback package (without makepkg)
arch-fallback: build
	@echo "Creating Arch-style package (fallback)..."
	./deploy/archlinux/create-arch-fallback.sh $(VERSION) $(ARCHITECTURE)

# Multi-architecture Arch packages
arch-multi: clean
	@echo "Building multi-architecture Arch packages..."
	$(MAKE) arch-arch GOARCH=amd64 ARCH=x86_64
	$(MAKE) arch-arch GOARCH=arm64 ARCH=aarch64  
	$(MAKE) arch-arch GOARCH=arm GOARM=7 ARCH=armv7h

arch-arch:
	@echo "Building Arch package for $(ARCH)..."
	GOOS=linux GOARCH=$(GOARCH) $(GO) build -trimpath -ldflags "\
		-X github.com/wweir/contatto/config.Version=$(VERSION) \
		-X github.com/wweir/contatto/config.Date=$(shell date +%Y-%m-%d)" \
		-o bin/contatto-$(ARCH) ./cmd/contatto
	
	./deploy/archlinux/create-arch-fallback.sh $(VERSION) $(ARCH)
	rm -f bin/contatto-$(ARCH)

# RPM packaging targets
rpm: build
	@echo "Creating RPM package..."
	@if command -v rpmbuild >/dev/null 2>&1; then \
		./deploy/rpm/create-rpm-package.sh $(VERSION) $(ARCHITECTURE); \
	else \
		echo "rpmbuild not found, creating fallback package..."; \
		./deploy/rpm/create-rpm-fallback.sh $(VERSION) $(ARCHITECTURE); \
	fi

# RPM fallback package (without rpmbuild)
rpm-fallback: build
	@echo "Creating RPM-style package (fallback)..."
	./deploy/rpm/create-rpm-fallback.sh $(VERSION) $(ARCHITECTURE)

# Multi-architecture RPM packages
rpm-multi: clean
	@echo "Building multi-architecture RPM packages..."
	$(MAKE) rpm-arch GOARCH=amd64 ARCH=x86_64
	$(MAKE) rpm-arch GOARCH=arm64 ARCH=aarch64  
	$(MAKE) rpm-arch GOARCH=arm GOARM=7 ARCH=armv7hl

rpm-arch:
	@echo "Building RPM package for $(ARCH)..."
	GOOS=linux GOARCH=$(GOARCH) $(GO) build -trimpath -ldflags "\
		-X github.com/wweir/contatto/config.Version=$(VERSION) \
		-X github.com/wweir/contatto/config.Date=$(shell date +%Y-%m-%d)" \
		-o bin/contatto-$(ARCH) ./cmd/contatto
	
	./deploy/rpm/create-rpm-fallback.sh $(VERSION) $(ARCH)
	rm -f bin/contatto-$(ARCH)

clean-deb:
	rm -f $(PACKAGE_NAME)_*.deb
	rm -f $(PACKAGE_NAME)_*.tar.xz
	rm -f $(PACKAGE_NAME)_*.dsc
	rm -f $(PACKAGE_NAME)_*.changes
	rm -rf deploy/debian/$(PACKAGE_NAME)
	rm -rf deploy/debian/.debhelper
	rm -f deploy/debian/debhelper-build-stamp
	rm -f deploy/debian/files

clean-arch:
	rm -f $(PACKAGE_NAME)-*.pkg.tar.*

clean-rpm:
	rm -f $(PACKAGE_NAME)-*.rpm

clean-all: clean clean-deb clean-arch clean-rpm

# Help target
help:
	@echo "Available targets:"
	@echo "  test         - Run tests"
	@echo "  build        - Build binary"
	@echo "  run          - Build and run with debug"
	@echo "  install      - Install to system"
	@echo "  clean        - Clean build artifacts"
	@echo ""
	@echo "Debian packaging:"
	@echo "  deb          - Build Debian package (requires dpkg-buildpackage)"
	@echo "  deb-simple   - Build simple Debian package (or fallback)"
	@echo "  deb-multi    - Build multi-architecture packages"
	@echo "  pkg-fallback - Create tar.gz package with install scripts"
	@echo "  clean-deb    - Clean Debian packaging artifacts"
	@echo ""
	@echo "Arch Linux packaging:"
	@echo "  arch         - Build Arch package (requires makepkg or fallback)"
	@echo "  arch-fallback - Create .pkg.tar.xz package without makepkg"
	@echo "  arch-multi   - Build multi-architecture Arch packages"
	@echo "  clean-arch   - Clean Arch packaging artifacts"
	@echo ""
	@echo "CentOS/RHEL RPM packaging:"
	@echo "  rpm          - Build RPM package (requires rpmbuild or fallback)"
	@echo "  rpm-fallback - Create .rpm package without rpmbuild"
	@echo "  rpm-multi    - Build multi-architecture RPM packages"
	@echo "  clean-rpm    - Clean RPM packaging artifacts"
	@echo ""
	@echo "  clean-all    - Clean all packaging artifacts"
	@echo "  help         - Show this help"

.PHONY: default test build run install clean deb deb-source deb-simple deb-multi deb-arch pkg-fallback arch arch-fallback arch-multi arch-arch rpm rpm-fallback rpm-multi rpm-arch clean-deb clean-arch clean-rpm clean-all help
