#!/usr/bin/env sh
set -eu

if [ ! -f /authorized_keys ]; then
  echo "authorized_keys not found" >&2
  exit 1
fi

mkdir -p /home/backup/.ssh
cp /authorized_keys /home/backup/.ssh/authorized_keys
chmod 700 /home/backup/.ssh
chmod 600 /home/backup/.ssh/authorized_keys
chown -R backup:backup /home/backup/.ssh

mkdir -p /data
chown -R backup:backup /data

sed -i "s/#Port 22/Port 2222/" /etc/ssh/sshd_config
sed -i "s/#PubkeyAuthentication yes/PubkeyAuthentication yes/" /etc/ssh/sshd_config
sed -i "s/#PasswordAuthentication yes/PasswordAuthentication no/" /etc/ssh/sshd_config
sed -i "s/#PermitRootLogin prohibit-password/PermitRootLogin no/" /etc/ssh/sshd_config

if command -v passwd >/dev/null 2>&1; then
  passwd -u backup >/dev/null 2>&1 || true
fi

ssh-keygen -A >/dev/null 2>&1

exec /usr/sbin/sshd -D -e
