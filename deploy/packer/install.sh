#!/bin/bash
# Runs as root on Packer's build instance: installs what the image holds and enables nothing. The instance's first
# boot renders the configuration from user-data and starts both units (files/first-boot.sh).
set -euo pipefail

files=/tmp/ubiq-files

# The Server runs the host's git.
dnf install -y git

# Pomerium from its rpm repository; the package brings its own unit and user.
install -m 0644 "$files/pomerium.repo" /etc/yum.repos.d/pomerium.repo
dnf install -y "pomerium-${POMERIUM_VERSION:?}"
install -D -m 0644 "$files/pomerium-ubiq.conf" /etc/systemd/system/pomerium.service.d/ubiq.conf
install -D -m 0644 "$files/pomerium.yaml.tmpl" /usr/local/share/ubiq/pomerium.yaml.tmpl

# The Server: the binary, its service user with git's credential store, and its unit, whose ExecStart first boot
# fills in.
install -m 0755 /tmp/ubiq /usr/local/bin/ubiq
{ /usr/local/bin/ubiq 2>&1 || true; } | grep -q 'usage: ubiq' # the binary runs on this architecture
useradd --system --home-dir /var/lib/ubiq --create-home --shell /sbin/nologin ubiq
install -m 0644 -o ubiq -g ubiq "$files/gitconfig" /var/lib/ubiq/.gitconfig
install -m 0644 "$files/ubiq.service" /etc/systemd/system/ubiq.service
install -d -m 0700 /etc/ubiq
install -D -m 0755 "$files/first-boot.sh" /usr/local/libexec/ubiq/first-boot

systemctl daemon-reload
systemctl disable pomerium.service ubiq.service

dnf clean all
rm -rf /tmp/ubiq /tmp/ubiq-files
