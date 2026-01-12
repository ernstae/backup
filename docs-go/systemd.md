# Systemd Scheduling

Systemd timers avoid overlapping runs and replace cron/anacron.

## Service

```ini
[Unit]
Description=TwinDB Backup (daily)
Wants=network-online.target
After=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/twindb-backup backup daily --config /etc/twindb/twindb-backup.yaml
```

## Timer

```ini
[Unit]
Description=Run TwinDB Backup daily

[Timer]
OnCalendar=daily
Persistent=true

[Install]
WantedBy=timers.target
```

Enable:

```sh
systemctl enable --now twindb-backup-daily.timer
```

The RPM/DEB packages also install template units in
`/lib/systemd/system/twindb-backup@.service` and `.timer`.
