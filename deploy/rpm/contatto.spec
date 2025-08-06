Name:           contatto
Version:        1.0.0
Release:        1%{?dist}
Summary:        Container registry transparent proxy

License:        MIT
URL:            https://github.com/wweir/contatto
Source0:        %{name}-%{version}.tar.gz

BuildRequires:  golang >= 1.20
BuildRequires:  systemd-rpm-macros
Requires:       systemd
Requires(pre):  shadow-utils

%description
Contatto is a transparent proxy for container registries that provides
registry mirroring and access optimization for Docker, containerd and
other container runtimes.

Key features:
- Registry mirroring and caching
- Docker daemon configuration injection
- Containerd configuration support
- Transparent proxy functionality
- Multi-architecture support

%prep
%setup -q

%build
export CGO_ENABLED=0
export GODEBUG=httpmuxgo121=1

go build -trimpath -ldflags "\
    -X github.com/wweir/contatto/config.Version=%{version}-%{release} \
    -X github.com/wweir/contatto/config.Date=$(date +%%Y-%%m-%%d)" \
    -o contatto ./cmd/contatto

%install
# Install binary
install -Dm755 contatto %{buildroot}%{_bindir}/contatto

# Install configuration file
install -Dm644 config/contatto.example.toml %{buildroot}%{_sysconfdir}/contatto.toml

# Install systemd service file
install -Dm644 deploy/systemd/contatto@.service %{buildroot}%{_unitdir}/contatto@.service

# Create directories
install -dm755 %{buildroot}%{_sharedstatedir}/contatto
install -dm755 %{buildroot}%{_localstatedir}/log/contatto

# Install systemd user/tmpfiles configuration
install -Dm644 - %{buildroot}%{_sysusersdir}/contatto.conf << 'EOF'
u contatto - "Contatto proxy daemon" /var/lib/contatto /sbin/nologin
EOF

install -Dm644 - %{buildroot}%{_tmpfilesdir}/contatto.conf << 'EOF'
d /var/lib/contatto 0755 contatto contatto -
d /var/log/contatto 0755 contatto contatto -
EOF

# Install documentation
install -Dm644 README.md %{buildroot}%{_docdir}/%{name}/README.md
install -Dm644 deploy/README.md %{buildroot}%{_docdir}/%{name}/DEPLOYMENT.md

%pre
# Create system user
getent group contatto >/dev/null || groupadd -r contatto
getent passwd contatto >/dev/null || \
    useradd -r -g contatto -d %{_sharedstatedir}/contatto -s /sbin/nologin \
    -c "Contatto proxy daemon" contatto
exit 0

%post
%systemd_post contatto@.service
# Create directories with correct ownership
%tmpfiles_create contatto.conf

%preun
%systemd_preun contatto@contatto.service

%postun
%systemd_postun_with_restart contatto@contatto.service
# Note: We don't remove the user and data directories for safety

%files
%license LICENSE
%doc %{_docdir}/%{name}/README.md
%doc %{_docdir}/%{name}/DEPLOYMENT.md
%{_bindir}/contatto
%config(noreplace) %{_sysconfdir}/contatto.toml
%{_unitdir}/contatto@.service
%{_sysusersdir}/contatto.conf
%{_tmpfilesdir}/contatto.conf
%dir %attr(0755,contatto,contatto) %{_sharedstatedir}/contatto
%dir %attr(0755,contatto,contatto) %{_localstatedir}/log/contatto

%changelog
* %(date "+%%a %%b %%d %%Y") wweir <wweir@wweir.cc> - %{version}-%{release}
- Initial RPM package
- Container registry transparent proxy
- Support for Docker and containerd configuration injection
- Multi-architecture build support