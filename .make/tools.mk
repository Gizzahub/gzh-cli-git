# Makefile.tools - Tool Installation and Management for gzh-cli
# Development tools, linters, formatters, and utilities

# ==============================================================================
# Tool Configuration
# ==============================================================================

# Keep quality tooling reproducible. Override these variables deliberately when
# testing a newer tool instead of silently following a moving latest tag.
GOFUMPT_VERSION ?= v0.10.0
GOIMPORTS_VERSION ?= v0.38.0
GCI_VERSION ?= v0.14.0
GOSEC_VERSION ?= v2.22.10
GOVULNCHECK_VERSION ?= v1.1.4
MDFORMAT_VERSION ?= 0.7.22
MDFORMAT_GFM_VERSION ?= 0.4.1
MDFORMAT_TABLES_VERSION ?= 1.0.0

# ==============================================================================
# Core Tool Installation
# ==============================================================================

.PHONY: install-tools install-quality-tools install-format-tools install-analysis-tools install-goreleaser
.PHONY: install-golangci-lint install-pre-commit-tools install-docs-tools

install-quality-tools: install-format-tools install-golangci-lint install-security-tools install-vuln-tools ## install the pinned tools required by quality-check
	@echo -e "$(GREEN)✅ Quality tools installed!$(RESET)"

install-tools: install-quality-tools install-analysis-tools install-goreleaser ## install all development tools
	@echo -e "$(GREEN)✅ All development tools installed!$(RESET)"

# A bare `which <tool>` cannot enforce the pins above. A developer machine almost
# always has these binaries already, so the guard short-circuits and the pinned
# version is never installed locally; only CI, which starts empty, ever runs the
# version this file names. That gap is not theoretical — it let a file pass
# `make format-check` locally under gofumpt v0.7.0 and fail the identical check
# in CI under the pinned v0.10.0, which is the worse failure because the gate
# reported success to the person able to fix it.
#
# `go version -m` reads the module version recorded inside the binary, so it
# identifies any go-installed tool regardless of what its own --version prints.
# When a tool resolves to something GOPATH/bin cannot replace (mise, Homebrew),
# reinstalling silently changes nothing, so the version is re-checked afterwards
# and the mismatch is reported with the offending path rather than ignored.
define ensure-go-tool
	@installed=$$(command -v $(1) 2>/dev/null); \
	current=$$([ -n "$$installed" ] && go version -m "$$installed" 2>/dev/null | awk '$$1=="mod"{print $$3; exit}'); \
	if [ "$$current" != "$(3)" ]; then \
		echo "Installing $(1) $(3) (found: $${current:-none})..."; \
		go install $(2)@$(3) || exit 1; \
		installed=$$(command -v $(1) 2>/dev/null); \
		current=$$([ -n "$$installed" ] && go version -m "$$installed" 2>/dev/null | awk '$$1=="mod"{print $$3; exit}'); \
		if [ "$$current" != "$(3)" ]; then \
			echo "$(1) resolves to $${current:-unknown} at $$installed, not the pinned $(3)." >&2; \
			echo "Another installation is shadowing GOPATH/bin; remove it or fix PATH." >&2; \
			exit 1; \
		fi; \
	fi
endef

install-format-tools: ## install advanced formatting tools
	@echo -e "$(CYAN)Installing formatting tools...$(RESET)"
	$(call ensure-go-tool,gofumpt,mvdan.cc/gofumpt,$(GOFUMPT_VERSION))
	$(call ensure-go-tool,goimports,golang.org/x/tools/cmd/goimports,$(GOIMPORTS_VERSION))
	$(call ensure-go-tool,gci,github.com/daixiang0/gci,$(GCI_VERSION))
	@command -v uv >/dev/null 2>&1 || { echo "uv is required to install mdformat" >&2; exit 1; }
	@command -v mdformat >/dev/null 2>&1 || (echo "Installing mdformat $(MDFORMAT_VERSION)..." && uv tool install --with mdformat-gfm==$(MDFORMAT_GFM_VERSION) --with mdformat-tables==$(MDFORMAT_TABLES_VERSION) mdformat==$(MDFORMAT_VERSION))
	@echo -e "$(GREEN)✅ All formatting tools installed!$(RESET)"

install-analysis-tools: ## install code analysis tools
	@echo -e "$(CYAN)Installing code analysis tools...$(RESET)"
	@command -v gocyclo >/dev/null 2>&1 || { echo "Installing gocyclo v0.6.0..." && go install github.com/fzipp/gocyclo/cmd/gocyclo@v0.6.0; }
	@command -v ineffassign >/dev/null 2>&1 || { echo "Installing ineffassign v0.1.0..." && go install github.com/gordonklaus/ineffassign@v0.1.0; }
	@command -v dupl >/dev/null 2>&1 || { echo "Installing dupl v0.3.0..." && go install github.com/mibk/dupl@v0.3.0; }
	@command -v staticcheck >/dev/null 2>&1 || { echo "Installing staticcheck 2025.1.1..." && go install honnef.co/go/tools/cmd/staticcheck@2025.1.1; }
	@command -v gosec >/dev/null 2>&1 || { echo "Installing gosec $(GOSEC_VERSION)..." && go install github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION); }
	@echo -e "$(GREEN)✅ All analysis tools installed!$(RESET)"

# Pin to a v2 release. A v1 binary on PATH rejects this repo's version: "2"
# config with "you are using a configuration file for golangci-lint v2 with
# golangci-lint v1". Prefer mise-managed golangci-lint when present; only
# install into GOPATH/bin when the active binary is missing or not v2.
GOLANGCI_LINT_VERSION ?= v2.12.2

install-golangci-lint: ## install golangci-lint v2 (skip when PATH already has v2)
	@echo -e "$(CYAN)Ensuring golangci-lint $(GOLANGCI_LINT_VERSION)...$(RESET)"
	@if command -v golangci-lint >/dev/null 2>&1 && golangci-lint version 2>/dev/null | grep -q "$(GOLANGCI_LINT_VERSION)"; then \
		echo -e "$(GREEN)✅ golangci-lint v2 already on PATH: $$(command -v golangci-lint)$(RESET)"; \
	else \
		echo -e "$(YELLOW)Installing golangci-lint $(GOLANGCI_LINT_VERSION) into $$(go env GOPATH)/bin$(RESET)"; \
		GOBIN=$$(go env GOPATH)/bin go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
		echo -e "$(GREEN)✅ golangci-lint $(GOLANGCI_LINT_VERSION) installed$(RESET)"; \
	fi

install-goreleaser: ## install goreleaser
	@echo -e "$(CYAN)Installing goreleaser...$(RESET)"
	@go install github.com/goreleaser/goreleaser/v2@v2.10.2
	@echo -e "$(GREEN)✅ goreleaser installed$(RESET)"

# ==============================================================================
# Mock and Generation Tools
# ==============================================================================

.PHONY: install-mock-tools generate-mocks clean-mocks regenerate-mocks

install-mock-tools: ## install mock generation tools
	@echo -e "$(CYAN)Installing mock generation tools...$(RESET)"
	@command -v mockgen >/dev/null 2>&1 || { echo "Installing mockgen v0.6.0..." && go install go.uber.org/mock/mockgen@v0.6.0; }
	@echo -e "$(GREEN)✅ Mock generation tools installed!$(RESET)"

generate-mocks: install-mock-tools ## generate all mock files using gomock
	@echo -e "$(CYAN)Generating mocks...$(RESET)"
	@echo "Generating GitHub interface mocks..."
	@if [ -f "pkg/github/interfaces.go" ]; then \
		mockgen -source=pkg/github/interfaces.go -destination=pkg/github/mocks/github_mocks.go -package=mocks; \
		echo "  ✅ GitHub mocks generated"; \
	else \
		echo "  ⚠️  pkg/github/interfaces.go not found"; \
	fi
	@echo "Generating filesystem interface mocks..."
	@if [ -f "internal/filesystem/interfaces.go" ]; then \
		mockgen -source=internal/filesystem/interfaces.go -destination=internal/filesystem/mocks/filesystem_mocks.go -package=mocks; \
		echo "  ✅ Filesystem mocks generated"; \
	else \
		echo "  ⚠️  internal/filesystem/interfaces.go not found"; \
	fi
	@echo "Generating HTTP client interface mocks..."
	@if [ -f "internal/httpclient/interfaces.go" ]; then \
		mockgen -source=internal/httpclient/interfaces.go -destination=internal/httpclient/mocks/httpclient_mocks.go -package=mocks; \
		echo "  ✅ HTTP client mocks generated"; \
	else \
		echo "  ⚠️  internal/httpclient/interfaces.go not found"; \
	fi
	@echo "Generating Git interface mocks..."
	@if [ -f "internal/git/interfaces.go" ]; then \
		mockgen -source=internal/git/interfaces.go -destination=internal/git/mocks/git_mocks.go -package=mocks; \
		echo "  ✅ Git mocks generated"; \
	else \
		echo "  ⚠️  internal/git/interfaces.go not found"; \
	fi
	@echo -e "$(GREEN)✅ Mock generation complete!$(RESET)"

clean-mocks: ## remove all generated mock files
	@echo -e "$(CYAN)Cleaning generated mocks...$(RESET)"
	@rm -f pkg/github/mocks/github_mocks.go
	@rm -f internal/filesystem/mocks/filesystem_mocks.go
	@rm -f internal/httpclient/mocks/httpclient_mocks.go
	@rm -f internal/git/mocks/git_mocks.go
	@echo -e "$(GREEN)✅ Mock cleanup complete!$(RESET)"

regenerate-mocks: clean-mocks generate-mocks ## clean and regenerate all mocks

# ==============================================================================
# Pre-commit and Git Hooks
# ==============================================================================

.PHONY: install-pre-commit-tools pre-commit-update

install-pre-commit-tools: ## install pre-commit and related tools
	@echo -e "$(CYAN)Installing pre-commit tools...$(RESET)"
	@command -v pre-commit >/dev/null 2>&1 || { echo -e "$(RED)pre-commit not found. Install with: pip install pre-commit$(RESET)"; }
	@echo -e "$(GREEN)✅ Pre-commit tools ready!$(RESET)"


# ==============================================================================
# Documentation Tools
# ==============================================================================

.PHONY: install-docs-tools

install-docs-tools: ## install documentation tools
	@echo -e "$(CYAN)Installing documentation tools...$(RESET)"
	@go install golang.org/x/perf/cmd/benchstat@latest
	@go install golang.org/x/tools/cmd/godoc@latest
	@which git-chglog >/dev/null 2>&1 || echo -e "$(YELLOW)Consider installing git-chglog for changelog generation$(RESET)"
	@which mkdocs >/dev/null 2>&1 || echo -e "$(YELLOW)Consider installing mkdocs for documentation: pip install mkdocs mkdocs-material$(RESET)"
	@echo -e "$(GREEN)✅ Documentation tools installed$(RESET)"

# ==============================================================================
# Security Tools
# ==============================================================================

.PHONY: install-security-tools

install-security-tools: ## install security analysis tools
	@echo -e "$(CYAN)Installing security tools...$(RESET)"
	@command -v gosec >/dev/null 2>&1 || { echo "Installing gosec $(GOSEC_VERSION)..." && go install github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION); }
	@echo -e "$(GREEN)✅ Security tools installed!$(RESET)"

# ==============================================================================
# Vulnerability Scanning
# ==============================================================================

.PHONY: install-vuln-tools

install-vuln-tools: ## install vulnerability scanning tools
	@echo -e "$(CYAN)Installing vulnerability scanning tools...$(RESET)"
	@command -v govulncheck >/dev/null 2>&1 || { echo "Installing govulncheck $(GOVULNCHECK_VERSION)..." && go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION); }
	@echo -e "$(GREEN)✅ Vulnerability tools ready!$(RESET)"

# ==============================================================================
# Tool Status and Information
# ==============================================================================

.PHONY: tools-status tools-info

tools-status: ## show installed tool status
	@echo -e "$(CYAN)Checking development tool status...$(RESET)"
	@echo ""
	@echo -e "$(GREEN)📦 Core Tools:$(RESET)"
	@printf "  %-20s " "go:"; go version 2>/dev/null | cut -d' ' -f3 || echo -e "$(RED)Not found$(RESET)"
	@printf "  %-20s " "git:"; git --version 2>/dev/null | cut -d' ' -f3 || echo -e "$(RED)Not found$(RESET)"
	@echo ""
	@echo -e "$(GREEN)🔧 Build Tools:$(RESET)"
	@printf "  %-20s " "goreleaser:"; goreleaser --version 2>/dev/null | head -1 | awk '{print $3}' || echo -e "$(RED)Not installed$(RESET)"
	@echo ""
	@echo -e "$(GREEN)✨ Format Tools:$(RESET)"
	@printf "  %-20s " "gofumpt:"; gofumpt --version 2>/dev/null || echo -e "$(RED)Not installed$(RESET)"
	@printf "  %-20s " "gci:"; gci --version 2>/dev/null || echo -e "$(RED)Not installed$(RESET)"
	@echo ""
	@echo -e "$(GREEN)🔍 Lint Tools:$(RESET)"
	@printf "  %-20s " "golangci-lint:"; golangci-lint --version 2>/dev/null | cut -d' ' -f4 || echo -e "$(RED)Not installed$(RESET)"
	@printf "  %-20s " "staticcheck:"; staticcheck -version 2>/dev/null || echo -e "$(RED)Not installed$(RESET)"
	@echo ""
	@echo -e "$(GREEN)🛡️  Security Tools:$(RESET)"
	@printf "  %-20s " "gosec:"; gosec -version 2>/dev/null || echo -e "$(RED)Not installed$(RESET)"
	@echo ""
	@echo -e "$(GREEN)🎭 Mock Tools:$(RESET)"
	@printf "  %-20s " "mockgen:"; mockgen --version 2>/dev/null || echo -e "$(RED)Not installed$(RESET)"
	@echo ""
	@echo -e "$(GREEN)🎣 Git Hooks:$(RESET)"
	@printf "  %-20s " "pre-commit:"; pre-commit --version 2>/dev/null || echo -e "$(RED)Not installed$(RESET)"

tools-info: ## show comprehensive tool information
	@echo -e "$(CYAN)"
	@echo "╔══════════════════════════════════════════════════════════════════════════════╗"
	@echo -e "║                         $(YELLOW)Development Tools Information$(CYAN)                   ║"
	@echo "╚══════════════════════════════════════════════════════════════════════════════╝"
	@echo -e "$(RESET)"
	@echo -e "$(GREEN)🔧 Available Tool Categories:$(RESET)"
	@echo -e "  • $(CYAN)Format Tools$(RESET)        Code formatting (gofumpt, gci)"
	@echo -e "  • $(CYAN)Analysis Tools$(RESET)      Static analysis (staticcheck, gosec)"
	@echo -e "  • $(CYAN)Lint Tools$(RESET)          Code linting (golangci-lint)"
	@echo -e "  • $(CYAN)Build Tools$(RESET)         Build and release (goreleaser)"
	@echo -e "  • $(CYAN)Mock Tools$(RESET)          Mock generation (mockgen)"
	@echo -e "  • $(CYAN)Security Tools$(RESET)      Security scanning (gosec, govulncheck)"
	@echo -e "  • $(CYAN)Git Hooks$(RESET)           Pre-commit hooks and validation"
	@echo -e "  • $(CYAN)Documentation$(RESET)       Documentation tools (godoc, benchstat)"
	@echo ""
	@echo -e "$(GREEN)🚀 Quick Installation:$(RESET)"
	@echo -e "  $(CYAN)make install-tools$(RESET)        Install all development tools"
	@echo -e "  $(CYAN)make tools-status$(RESET)         Check current tool installation status"
	@echo ""
	@echo -e "$(GREEN)💡 Individual Categories:$(RESET)"
	@echo -e "  $(CYAN)make install-format-tools$(RESET)     Format tools only"
	@echo -e "  $(CYAN)make install-analysis-tools$(RESET)   Analysis tools only"
	@echo -e "  $(CYAN)make install-security-tools$(RESET)   Security tools only"
	@echo -e "  $(CYAN)make install-mock-tools$(RESET)       Mock generation tools only"
