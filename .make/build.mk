# Makefile.build - Build and Installation targets for gzh-cli
# Build, compilation, and installation management

# ==============================================================================
# Build Configuration
# ==============================================================================

# Go environment configuration
# Use system Go installation
# NOTE: rangefunc was GOEXPERIMENT in Go 1.22, built-in since Go 1.23+

# Detect OS-specific executable extension (e.g., .exe on Windows)
BINEXT := $(shell go env GOEXE)
BINARY := $(executablename)$(BINEXT)
GOBIN := $(shell go env GOBIN)
GOPATH := $(shell go env GOPATH)
HOST_GOCACHE := $(shell go env GOCACHE)
HOST_GOMODCACHE := $(shell go env GOMODCACHE)
HOST_GOTOOLCHAIN := $(shell go env GOTOOLCHAIN)

# OS-specific path separator and binary install dir
ifeq ($(OS),Windows_NT)
SEP := \\\\
else
SEP := /
endif

ifndef BINDIR
  ifeq ($(strip $(GOBIN)),)
    BINDIR := $(GOPATH)$(SEP)bin
  else
    BINDIR := $(GOBIN)
  endif
endif

# ==============================================================================
# Build Targets
# ==============================================================================

.PHONY: build install install-check-target test-install install-git-plugin run bootstrap clean release-dry-run release-snapshot release-check deploy

build: ## build golang binary
	@printf "$(CYAN)Building %s...$(RESET)\n" "$(BINARY)"
	@go build -ldflags "-X main.version=$(VERSION)" -o "$(BINARY)" ./cmd/gz-git
	@printf "$(GREEN)Built %s successfully$(RESET)\n" "$(BINARY)"


install: install-check-target build ## install golang binary
	@printf "$(CYAN)Installing $(BINARY) $(VERSION) to %s$(RESET)\n" "$(BINDIR)$(SEP)$(BINARY)"
	@mkdir -p "$(BINDIR)"
	@mv "$(BINARY)" "$(BINDIR)$(SEP)$(BINARY)"
	@printf "$(GREEN)✅ Installed $(BINARY) $(VERSION) to %s$(RESET)\n" "$(BINDIR)$(SEP)$(BINARY)"
	@echo ""
	@printf "$(CYAN)Verifying installation...$(RESET)\n"
	@"$(BINDIR)$(SEP)$(BINARY)" --version
	@echo ""
	@printf "$(GREEN)🎉 Installation complete! Run '$(BINARY) --help' to get started.$(RESET)\n"

install-check-target:
	@if [ -d "$(BINDIR)$(SEP)$(BINARY)" ]; then \
		printf "$(RED)Error: install target is a directory: %s$(RESET)\n" "$(BINDIR)$(SEP)$(BINARY)" >&2; \
		exit 1; \
	fi

test-install: ## verify install uses only BINDIR without touching user directories
	@set -eu; \
		tmpdir="$$(mktemp -d "$${TMPDIR:-/tmp}/gz-git-install.XXXXXX")"; \
		trap 'chmod -R u+w "$$tmpdir"; rm -rf "$$tmpdir"' EXIT; \
		for mode in gobin gopath bindir; do \
			test_home="$$tmpdir/$$mode home"; \
			mkdir -p "$$test_home"; \
			if [ "$$mode" = gobin ]; then \
				bindir="$$tmpdir/gobin dir"; \
				GOWORK=off GOCACHE="$(HOST_GOCACHE)" GOMODCACHE="$(HOST_GOMODCACHE)" GOTOOLCHAIN="$(HOST_GOTOOLCHAIN)" $(MAKE) --no-print-directory install GOBIN="$$bindir" HOME="$$test_home"; \
			elif [ "$$mode" = gopath ]; then \
				gopath="$$tmpdir/gopath dir"; \
				bindir="$$gopath$(SEP)bin"; \
				GOWORK=off GOCACHE="$(HOST_GOCACHE)" GOMODCACHE="$(HOST_GOMODCACHE)" GOTOOLCHAIN="$(HOST_GOTOOLCHAIN)" $(MAKE) --no-print-directory install GOBIN= GOPATH="$$gopath" HOME="$$test_home"; \
			else \
				bindir="$$tmpdir/bindir override"; \
				GOWORK=off GOCACHE="$(HOST_GOCACHE)" GOMODCACHE="$(HOST_GOMODCACHE)" GOTOOLCHAIN="$(HOST_GOTOOLCHAIN)" $(MAKE) --no-print-directory install BINDIR="$$bindir" GOBIN="$$tmpdir/ignored gobin" HOME="$$test_home"; \
				test ! -e "$$tmpdir/ignored gobin$(SEP)$(BINARY)"; \
			fi; \
			test -x "$$bindir$(SEP)$(BINARY)"; \
			test ! -e "$$test_home$(SEP).local$(SEP)bin$(SEP)$(BINARY)"; \
		done; \
		failure_target="$$tmpdir/target directory$(SEP)$(BINARY)"; \
		mkdir -p "$$failure_target"; \
		if GOWORK=off GOCACHE="$(HOST_GOCACHE)" GOMODCACHE="$(HOST_GOMODCACHE)" GOTOOLCHAIN="$(HOST_GOTOOLCHAIN)" $(MAKE) --no-print-directory install BINDIR="$$tmpdir/target directory" HOME="$$tmpdir/failure home" >"$$tmpdir/failure.log" 2>&1; then \
			echo "expected install to reject a directory target" >&2; \
			exit 1; \
		fi; \
		grep -F "Error: install target is a directory" "$$tmpdir/failure.log" >/dev/null; \
		printf "$(GREEN)✅ Install path regression checks passed$(RESET)\n"

install-git-plugin: install ## install as git plugin (git forge)
	@printf "$(CYAN)Installing git plugin symlink...$(RESET)\n"
	@ln -sf "$(BINDIR)$(SEP)$(BINARY)" "$(BINDIR)$(SEP)git-forge"
	@printf "$(GREEN)✅ Git plugin installed! Use 'git forge' to run.$(RESET)\n"

run: ## run the application (usage: make run [args...] or ARGS="args" make run)
	@echo -e "$(CYAN)Running application with version $(VERSION)...$(RESET)"
	@if [ "$(words $(MAKECMDGOALS))" -gt 1 ]; then \
		ARGS="$(filter-out run,$(MAKECMDGOALS))"; \
		echo -e "$(YELLOW)Arguments: $$ARGS$(RESET)"; \
		go run -ldflags "-X main.version=$(VERSION)" ./cmd/gz-git $$ARGS; \
	elif [ -n "$(ARGS)" ]; then \
		echo -e "$(YELLOW)Arguments: $(ARGS)$(RESET)"; \
		go run -ldflags "-X main.version=$(VERSION)" ./cmd/gz-git $(ARGS); \
	else \
		go run -ldflags "-X main.version=$(VERSION)" ./cmd/gz-git; \
	fi

# Prevent make from interpreting arguments as targets
%:
	@:

bootstrap: ## install build dependencies
	@echo -e "$(CYAN)Installing build dependencies...$(RESET)"
	go generate -tags tools tools/tools.go
	@echo -e "$(GREEN)✅ Build dependencies installed$(RESET)"

clean: ## clean up environment
	@echo -e "$(CYAN)Cleaning up build artifacts...$(RESET)"
	@rm -rf coverage.out coverage.html dist/ $(executablename) $(BINARY)
	@rm -f $(shell go env GOPATH)/bin/$(executablename)
	@rm -f $(shell go env GOPATH)/bin/$(BINARY)
	@rm -f lint-report.json gosec-report.json
	@echo -e "$(GREEN)✅ Cleanup completed$(RESET)"

# ==============================================================================
# Release Targets
# ==============================================================================

release-dry-run: ## run goreleaser in dry-run mode
	@echo -e "$(CYAN)Running goreleaser in dry-run mode...$(RESET)"
	@command -v goreleaser >/dev/null 2>&1 || { echo -e "$(RED)goreleaser not found. Install with: make install-goreleaser$(RESET)"; exit 1; }
	@goreleaser release --snapshot --clean --skip=publish

release-snapshot: ## create a snapshot release
	@echo -e "$(CYAN)Creating snapshot release...$(RESET)"
	@command -v goreleaser >/dev/null 2>&1 || { echo -e "$(RED)goreleaser not found. Install with: make install-goreleaser$(RESET)"; exit 1; }
	@goreleaser release --snapshot --clean

release-check: ## check goreleaser configuration
	@echo -e "$(CYAN)Checking goreleaser configuration...$(RESET)"
	@command -v goreleaser >/dev/null 2>&1 || { echo -e "$(RED)goreleaser not found. Install with: make install-goreleaser$(RESET)"; exit 1; }
	@goreleaser check

deploy: release-dry-run ## alias for release-dry-run

# ==============================================================================
# Version Bump Targets
# ==============================================================================

.PHONY: verup-a verup-b verup-c verup-show

# Parse current version components
CURRENT_VERSION := $(shell cat VERSION 2>/dev/null || echo "0.0.0")
VERSION_MAJOR := $(shell echo $(CURRENT_VERSION) | cut -d. -f1)
VERSION_MINOR := $(shell echo $(CURRENT_VERSION) | cut -d. -f2)
VERSION_PATCH := $(shell echo $(CURRENT_VERSION) | cut -d. -f3)

verup-show: ## show current version
	@echo -e "$(CYAN)Current version: $(YELLOW)$(CURRENT_VERSION)$(RESET)"
	@echo -e "  Major: $(VERSION_MAJOR)"
	@echo -e "  Minor: $(VERSION_MINOR)"
	@echo -e "  Patch: $(VERSION_PATCH)"

verup-a: ## bump major version (X.0.0)
	@echo -e "$(CYAN)Bumping major version...$(RESET)"
	@NEW_MAJOR=$$(( $(VERSION_MAJOR) + 1 )); \
	NEW_VERSION="$$NEW_MAJOR.0.0"; \
	echo -e "$(YELLOW)$(CURRENT_VERSION)$(RESET) → $(GREEN)$$NEW_VERSION$(RESET)"; \
	echo "$$NEW_VERSION" > VERSION; \
	perl -i -pe 's/Version = "[^"]*"/Version = "'$$NEW_VERSION'"/' version.go; \
	echo -e "$(GREEN)✅ Version bumped to $$NEW_VERSION$(RESET)"; \
	echo -e "$(CYAN)Updated files:$(RESET)"; \
	echo "  - VERSION"; \
	echo "  - version.go"

verup-b: ## bump minor version (0.X.0)
	@echo -e "$(CYAN)Bumping minor version...$(RESET)"
	@NEW_MINOR=$$(( $(VERSION_MINOR) + 1 )); \
	NEW_VERSION="$(VERSION_MAJOR).$$NEW_MINOR.0"; \
	echo -e "$(YELLOW)$(CURRENT_VERSION)$(RESET) → $(GREEN)$$NEW_VERSION$(RESET)"; \
	echo "$$NEW_VERSION" > VERSION; \
	perl -i -pe 's/Version = "[^"]*"/Version = "'$$NEW_VERSION'"/' version.go; \
	echo -e "$(GREEN)✅ Version bumped to $$NEW_VERSION$(RESET)"; \
	echo -e "$(CYAN)Updated files:$(RESET)"; \
	echo "  - VERSION"; \
	echo "  - version.go"

verup-c: ## bump patch version (0.0.X)
	@echo -e "$(CYAN)Bumping patch version...$(RESET)"
	@NEW_PATCH=$$(( $(VERSION_PATCH) + 1 )); \
	NEW_VERSION="$(VERSION_MAJOR).$(VERSION_MINOR).$$NEW_PATCH"; \
	echo -e "$(YELLOW)$(CURRENT_VERSION)$(RESET) → $(GREEN)$$NEW_VERSION$(RESET)"; \
	echo "$$NEW_VERSION" > VERSION; \
	perl -i -pe 's/Version = "[^"]*"/Version = "'$$NEW_VERSION'"/' version.go; \
	echo -e "$(GREEN)✅ Version bumped to $$NEW_VERSION$(RESET)"; \
	echo -e "$(CYAN)Updated files:$(RESET)"; \
	echo "  - VERSION"; \
	echo "  - version.go"

# ==============================================================================
# Build Information
# ==============================================================================

.PHONY: build-info

build-info: ## show build information and current configuration
	@echo -e "$(CYAN)"
	@echo "╔══════════════════════════════════════════════════════════════════════════════╗"
	@echo -e "║                         $(YELLOW)Build Information$(CYAN)                              ║"
	@echo "╚══════════════════════════════════════════════════════════════════════════════╝"
	@echo -e "$(RESET)"
	@echo -e "$(GREEN)📋 Project Details:$(RESET)"
	@echo -e "  Name:           $(YELLOW)$(projectname)$(RESET)"
	@echo -e "  Executable:     $(YELLOW)$(BINARY)$(RESET)"
	@echo -e "  Version:        $(YELLOW)$(VERSION)$(RESET)"
	@echo ""
	@echo -e "$(GREEN)🏗️  Build Environment:$(RESET)"
	@echo "  Go Version:     $$(go version | cut -d' ' -f3)"
	@echo -e "  GOPROXY:        $(GOPROXY)"
	@echo -e "  GOSUMDB:        $(GOSUMDB)"
	@echo "  GOPATH:         $$(go env GOPATH)"
	@echo "  GOROOT:         $$(go env GOROOT)"
	@echo ""
	@echo -e "$(GREEN)🎯 Build Targets:$(RESET)"
	@echo -e "  • $(CYAN)build$(RESET)               Build golang binary"
	@echo -e "  • $(CYAN)install$(RESET)             Install golang binary to GOPATH/bin"
	@echo -e "  • $(CYAN)run$(RESET)                 Run the application"
	@echo -e "  • $(CYAN)bootstrap$(RESET)           Install build dependencies"
	@echo -e "  • $(CYAN)clean$(RESET)               Clean up build artifacts"
	@echo -e "  • $(CYAN)release-dry-run$(RESET)     Test goreleaser configuration"
	@echo -e "  • $(CYAN)release-snapshot$(RESET)    Create snapshot release"
	@echo -e "  • $(CYAN)release-check$(RESET)       Check goreleaser configuration"
