.PHONY: help clean clean-docs build-go test-go test-integration docs-go docs-go-serve \
	build-packages build-packages-single-arch build-rpm build-deb

help:
	@printf "Targets:\n"
	@printf "  build-go                Build the Go CLI binary\n"
	@printf "  test-go                 Run Go unit tests\n"
	@printf "  test-integration         Run integration tests (podman)\n"
	@printf "  docs-go                 Build MkDocs site (containerized)\n"
	@printf "  docs-go-serve           Serve MkDocs site (containerized)\n"
	@printf "  build-packages          Build RPM/DEB packages (multi-arch)\n"
	@printf "  build-packages-single-arch  Build RPM/DEB packages (current arch)\n"
	@printf "  build-rpm               Build RPM package (usage: make build-rpm VERSION=8)\n"
	@printf "  build-deb               Build DEB package\n"
	@printf "  clean                   Remove build artifacts\n"

clean:
	rm -rf bin/
	rm -rf site/
	rm -rf .gocache/

clean-docs:
	rm -rf site/

build-go: ## Build the Go CLI binary
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o bin/twindb-backup ./cmd/twindb-backup

test-go: ## Run Go unit tests
	GOCACHE=$(pwd)/.gocache go test ./...

test-integration: ## Run integration tests (podman)
	./tests/integration/podman-suite.sh

docs-go: ## Build MkDocs site for the Go rewrite (containerized)
	./packaging/build-docs.sh

docs-go-serve: ## Serve MkDocs site for the Go rewrite (containerized)
	./packaging/serve-docs.sh

build-packages: ## Build all packages for all architectures (requires podman)
	@echo "Building packages for all platforms and architectures..."
	@mkdir -p packaging/dist
	@for arch in amd64 arm64; do \
		echo "Building for $$arch..."; \
		GOARCH=$$arch ./packaging/build-rpm.sh 8; \
		GOARCH=$$arch ./packaging/build-rpm.sh 9; \
		GOARCH=$$arch ./packaging/build-rpm.sh 10; \
		GOARCH=$$arch ./packaging/build-deb.sh; \
	done
	@echo "Packages built in packaging/dist/"

build-packages-single-arch: ## Build all packages for current architecture only (requires podman)
	@echo "Building packages for all platforms (single architecture)..."
	@mkdir -p packaging/dist
	@./packaging/build-rpm.sh 8
	@./packaging/build-rpm.sh 9
	@./packaging/build-rpm.sh 10
	@./packaging/build-deb.sh
	@echo "Packages built in packaging/dist/"

build-rpm: ## Build RPM package (usage: make build-rpm VERSION=8)
	@if [ -z "$(VERSION)" ]; then \
		echo "Usage: make build-rpm VERSION=8|9|10"; \
		exit 1; \
	fi
	@mkdir -p packaging/dist
	@./packaging/build-rpm.sh $(VERSION)

build-deb: ## Build DEB package for Ubuntu
	@mkdir -p packaging/dist
	@./packaging/build-deb.sh
