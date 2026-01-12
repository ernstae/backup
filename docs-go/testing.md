# Testing

## Unit Tests

```sh
GOCACHE=$(pwd)/.gocache go test ./...
```

## Integration Tests (Podman)

```sh
./tests/integration/podman-suite.sh
```

The integration suite spins up Percona MySQL, sshd, and statsd containers
and validates backup/restore across destinations. Podman is required.
