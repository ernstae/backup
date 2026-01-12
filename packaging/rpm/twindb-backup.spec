Name:           twindb-backup
Version:        %{?version}%{!?version:5.0.0}
Release:        1%{?dist}
Summary:        TwinDB Backup v5.0.0

License:        Apache-2.0
URL:            https://github.com/twindb/backup
Source0:        %{name}-%{version}.tar.gz

# Note: golang is not listed because we install Go 1.23+ from upstream tarball
# in the build container. The build environment is controlled and Go is available
# via /usr/local/go/bin/go, which satisfies the build requirements.
BuildRequires:  make
BuildRequires:  git
Requires:       xtrabackup
Requires:       xbcloud
Requires:       mysql
Requires:       gzip
Requires:       bzip2

%description
TwinDB Backup is a tool for streaming MySQL backups to blob storage with
binlog-based point-in-time recovery.

%prep
%autosetup -n %{name}-%{version}

%build
export CGO_ENABLED=0
export GOOS=linux
%{__make} build-go

%install
rm -rf %{buildroot}
install -D -m 0755 bin/twindb-backup %{buildroot}%{_bindir}/twindb-backup
install -D -m 0644 packaging/systemd/twindb-backup@.service %{buildroot}%{_unitdir}/twindb-backup@.service
install -D -m 0644 packaging/systemd/twindb-backup@.timer %{buildroot}%{_unitdir}/twindb-backup@.timer
install -D -m 0644 docs/GO_REWRITE.md %{buildroot}%{_docdir}/%{name}/GO_REWRITE.md

%post
%systemd_post twindb-backup@.service

%preun
%systemd_preun twindb-backup@.service

%postun
%systemd_postun_with_restart twindb-backup@.service

%files
%license LICENSE
%doc docs/GO_REWRITE.md
%{_bindir}/twindb-backup
%{_unitdir}/twindb-backup@.service
%{_unitdir}/twindb-backup@.timer

%changelog
* Thu Jan 01 1970 TwinDB <support@twindb.com> - 5.0.0-1
- Initial v5.0.0 release
