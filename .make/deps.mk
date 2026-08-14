# Makefile.deps.mk - Enhanced Dependency Management for gzh-cli
# Alternative to Dependabot for controlled local updates

# ==============================================================================
# Dependency Management Configuration
# ==============================================================================

.PHONY: deps-check deps-update deps-upgrade deps-update-go deps-update-actions deps-update-docker
.PHONY: deps-outdated deps-security deps-audit deps-report deps-clean deps-help
.PHONY: deps-update-minor deps-update-patch deps-update-major
.PHONY: deps-verify deps-why deps-tidy

# ==============================================================================
# Go Dependencies
# ==============================================================================

deps-check: ## check for outdated Go dependencies
	@echo -e "$(CYAN)Checking for outdated Go dependencies...$(RESET)"
	@go list -u -m all | grep '\[' || echo -e "$(GREEN)✅ All Go dependencies are up to date$(RESET)"

deps-outdated: ## detailed report of outdated dependencies (direct dependencies only)
	@echo -e "$(CYAN)Generating detailed outdated dependencies report (direct dependencies only)...$(RESET)"
	@echo -e "$(YELLOW)Direct Dependencies:$(RESET)"
	@direct_deps=$$(go list -m -f '{{if not .Indirect}}{{.Path}}{{end}}' all | grep -v "^$$"); \
	direct_outdated=$$(go list -u -m $$direct_deps | grep '\['); \
	if [ -n "$$direct_outdated" ]; then \
		echo "$$direct_outdated" | while IFS= read -r line; do \
			echo -e "  $(RED)→$(RESET) $$line"; \
		done; \
	else \
		echo -e "  $(GREEN)✅ All direct dependencies are up to date$(RESET)"; \
	fi

deps-tidy: ## run go mod tidy to clean up dependencies
	@echo -e "$(CYAN)Tidying Go modules...$(RESET)"
	@go mod tidy
	@echo -e "$(GREEN)✅ Go modules tidied$(RESET)"

deps-update: ## update all dependencies (safe: patch versions only)
	@echo -e "$(CYAN)Updating dependencies safely (patch versions only)...$(RESET)"
	@echo -e "$(YELLOW)Before update:$(RESET)"
	@go mod tidy
	@cp go.mod go.mod.backup
	@cp go.sum go.sum.backup
	@echo -e "$(CYAN)Updating Go dependencies...$(RESET)"
	@go get -u=patch ./...
	@go mod tidy
	@echo -e "$(GREEN)✅ Dependencies updated safely$(RESET)"
	@echo -e "$(YELLOW)Changes:$(RESET)"
	@diff go.mod.backup go.mod || echo "  No changes in go.mod"
	@rm go.mod.backup go.sum.backup

deps-update-minor: ## update to latest minor versions (more aggressive)
	@echo -e "$(CYAN)Updating to latest minor versions...$(RESET)"
	@cp go.mod go.mod.backup
	@cp go.sum go.sum.backup
	@go get -u ./...
	@go mod tidy
	@echo -e "$(GREEN)✅ Dependencies updated to latest minor versions$(RESET)"
	@echo -e "$(YELLOW)Changes:$(RESET)"
	@diff go.mod.backup go.mod || echo "  No changes in go.mod"
	@rm go.mod.backup go.sum.backup

deps-update-patch: ## update to latest patch versions only (safest)
	@echo -e "$(CYAN)Updating to latest patch versions only...$(RESET)"
	@cp go.mod go.mod.backup
	@cp go.sum go.sum.backup
	@go get -u=patch ./...
	@go mod tidy
	@echo -e "$(GREEN)✅ Dependencies updated to latest patch versions$(RESET)"
	@echo -e "$(YELLOW)Changes:$(RESET)"
	@diff go.mod.backup go.mod || echo "  No changes in go.mod"
	@rm go.mod.backup go.sum.backup

deps-update-major: ## update to latest major versions (use with caution!)
	@echo -e "$(RED)⚠️  WARNING: This will update to latest major versions!$(RESET)"
	@echo -e "$(YELLOW)This may introduce breaking changes. Continue? [y/N]$(RESET)"
	@read -r confirm && [ "$$confirm" = "y" ] || [ "$$confirm" = "Y" ] || exit 1
	@cp go.mod go.mod.backup
	@cp go.sum go.sum.backup
	@go list -u -m all | grep '\[' | cut -d' ' -f1 | xargs -I {} go get {}@latest
	@go mod tidy
	@echo -e "$(GREEN)✅ Dependencies updated to latest major versions$(RESET)"
	@echo -e "$(YELLOW)Changes:$(RESET)"
	@diff go.mod.backup go.mod || echo "  No changes in go.mod"
	@rm go.mod.backup go.sum.backup

# ==============================================================================
# GitHub Actions Dependencies
# ==============================================================================

deps-update-actions: ## check and show GitHub Actions that need updates
	@echo -e "$(CYAN)Checking GitHub Actions dependencies...$(RESET)"
	@if [ -d ".github/workflows" ]; then \
		echo -e "$(YELLOW)GitHub Actions in use:$(RESET)"; \
		grep -r "uses:" .github/workflows/ | sed 's/.*uses: */  → /' | sort | uniq; \
		echo ""; \
		echo -e "$(YELLOW)To update GitHub Actions, manually edit .github/workflows/*.yml files$(RESET)"; \
		echo -e "$(YELLOW)Common updates:$(RESET)"; \
		echo "  → actions/checkout@v4"; \
		echo "  → actions/setup-go@v5"; \
		echo "  → actions/cache@v4"; \
		echo "  → codecov/codecov-action@v4"; \
	else \
		echo -e "$(GREEN)✅ No GitHub Actions found$(RESET)"; \
	fi

# ==============================================================================
# Docker Dependencies
# ==============================================================================

deps-update-docker: ## check and show Docker base images that need updates
	@echo -e "$(CYAN)Checking Docker dependencies...$(RESET)"
	@if [ -f "Dockerfile" ]; then \
		echo -e "$(YELLOW)Docker base images in use:$(RESET)"; \
		grep -E "^FROM" Dockerfile | sed 's/FROM */  → /'; \
		echo ""; \
		echo -e "$(YELLOW)To update Docker images, manually edit Dockerfile$(RESET)"; \
		echo -e "$(YELLOW)Consider using specific version tags instead of 'latest'$(RESET)"; \
	else \
		echo -e "$(GREEN)✅ No Dockerfile found$(RESET)"; \
	fi
	@if [ -f "docker-compose.yml" ]; then \
		echo ""; \
		echo -e "$(YELLOW)Docker Compose images in use:$(RESET)"; \
		grep -E "image:" docker-compose.yml | sed 's/.*image: */  → /' | sort | uniq; \
	fi

# ==============================================================================
# Security and Audit
# ==============================================================================

deps-security: ## run security audit on dependencies
	@echo -e "$(CYAN)Running security audit...$(RESET)"
	@echo -e "$(YELLOW)Checking for known vulnerabilities...$(RESET)"
	@go run golang.org/x/vuln/cmd/govulncheck@latest ./...

deps-audit: ## comprehensive dependency audit and report
	@echo -e "$(CYAN)Comprehensive dependency audit...$(RESET)"
	@echo -e "$(BLUE)=== Go Module Information ===$(RESET)"
	@go version
	@echo "Module: $$(go list -m)"
	@echo "Go version: $$(go list -m -f '{{.GoVersion}}')"
	@echo ""
	@echo -e "$(BLUE)=== Direct Dependencies ===$(RESET)"
	@go list -m -f '{{if not .Indirect}}{{.Path}} {{.Version}}{{end}}' all | grep -v "^$$" | head -20
	@echo ""
	@echo -e "$(BLUE)=== Outdated Dependencies ===$(RESET)"
	@make --no-print-directory deps-outdated
	@echo ""
	@echo -e "$(BLUE)=== Security Check ===$(RESET)"
	@make --no-print-directory deps-security

deps-verify: ## verify dependency checksums
	@echo -e "$(CYAN)Verifying dependency checksums...$(RESET)"
	@go mod verify
	@echo -e "$(GREEN)✅ All dependency checksums verified$(RESET)"

deps-why: ## show why a specific module is needed (usage: make deps-why MOD=github.com/spf13/cobra)
	@if [ -z "$(MOD)" ]; then \
		echo -e "$(RED)Usage: make deps-why MOD=github.com/spf13/cobra$(RESET)"; \
		exit 1; \
	fi
	@echo -e "$(CYAN)Showing why $(MOD) is needed...$(RESET)"
	@go mod why -m $(MOD)

# ==============================================================================
# Dependency Reports
# ==============================================================================

deps-report: ## generate comprehensive dependency report
	@echo -e "$(CYAN)Generating dependency report...$(RESET)"
	@mkdir -p results/deps
	@report_file="results/deps/dependency-report-$$(date +%Y%m%d-%H%M%S).md"; \
	echo "# Dependency Report - gzh-cli" > $$report_file; \
	echo "Generated: $$(date)" >> $$report_file; \
	echo "" >> $$report_file; \
	echo "## Go Module Information" >> $$report_file; \
	echo "\`\`\`" >> $$report_file; \
	go version >> $$report_file; \
	echo "Module: $$(go list -m)" >> $$report_file; \
	echo "Go version: $$(go list -m -f '{{.GoVersion}}')" >> $$report_file; \
	echo "\`\`\`" >> $$report_file; \
	echo "" >> $$report_file; \
	echo "## Direct Dependencies" >> $$report_file; \
	echo "\`\`\`" >> $$report_file; \
	go list -m -f '{{if not .Indirect}}{{.Path}} {{.Version}}{{end}}' all | grep -v "^$$" >> $$report_file; \
	echo "\`\`\`" >> $$report_file; \
	echo "" >> $$report_file; \
	echo "## Outdated Dependencies" >> $$report_file; \
	echo "" >> $$report_file; \
	echo "### Direct Dependencies (Updatable)" >> $$report_file; \
	echo "\`\`\`" >> $$report_file; \
	direct_deps=$$(go list -m -f '{{if not .Indirect}}{{.Path}}{{end}}' all | grep -v "^$$"); \
	direct_outdated=$$(go list -u -m $$direct_deps | grep '\['); \
	if [ -n "$$direct_outdated" ]; then \
		echo "$$direct_outdated" >> $$report_file; \
	else \
		echo "All direct dependencies are up to date" >> $$report_file; \
	fi; \
	echo "\`\`\`" >> $$report_file; \
	echo "" >> $$report_file; \
	echo "### Indirect Dependencies (Informational)" >> $$report_file; \
	echo "\`\`\`" >> $$report_file; \
	all_outdated=$$(go list -u -m all | grep '\['); \
	if [ -n "$$direct_outdated" ] && [ -n "$$all_outdated" ]; then \
		indirect_outdated=$$(echo "$$all_outdated" | grep -v -F "$$direct_outdated" 2>/dev/null || echo "$$all_outdated"); \
	elif [ -z "$$direct_outdated" ] && [ -n "$$all_outdated" ]; then \
		indirect_outdated="$$all_outdated"; \
	else \
		indirect_outdated=""; \
	fi; \
	if [ -n "$$indirect_outdated" ]; then \
		echo "$$indirect_outdated" >> $$report_file; \
	else \
		echo "All indirect dependencies are up to date" >> $$report_file; \
	fi; \
	echo "\`\`\`" >> $$report_file; \
	echo -e "$(GREEN)✅ Report generated: $$report_file$(RESET)"

# ==============================================================================
# Cleanup and Maintenance
# ==============================================================================

deps-clean: ## clean up dependency cache and temporary files
	@echo -e "$(CYAN)Cleaning dependency cache...$(RESET)"
	@go clean -modcache
	@go clean -cache
	@rm -f go.mod.backup go.sum.backup
	@rm -f go.mod.monthly-backup go.sum.monthly-backup
	@echo -e "$(GREEN)✅ Dependency cache cleaned$(RESET)"

# ==============================================================================
# Dependabot Alternative - Use individual commands as needed
# ==============================================================================

# ==============================================================================
# Help System
# ==============================================================================

deps-help: ## show comprehensive help for dependency management commands
	@echo -e "$(BLUE)📦 Dependency Management Commands:$(RESET)"
	@echo ""
	@echo -e "$(YELLOW)📋 Daily Operations:$(RESET)"
	@echo -e "  $(CYAN)deps-check$(RESET)            Check for outdated dependencies"
	@echo -e "  $(CYAN)deps-outdated$(RESET)         Detailed outdated dependencies report"
	@echo -e "  $(CYAN)deps-tidy$(RESET)             Run go mod tidy to clean up dependencies"
	@echo -e "  $(CYAN)deps-update$(RESET)           Safe update (patch + minor only)"
	@echo ""
	@echo -e "$(YELLOW)🔄 Update Levels:$(RESET)"
	@echo -e "  $(CYAN)deps-update-patch$(RESET)     Update patch versions only (safest)"
	@echo -e "  $(CYAN)deps-update-minor$(RESET)     Update minor versions (moderate risk)"
	@echo -e "  $(CYAN)deps-update-major$(RESET)     Update major versions (⚠️  breaking changes!)"
	@echo ""
	@echo -e "$(YELLOW)🔒 Security & Audit:$(RESET)"
	@echo -e "  $(CYAN)deps-security$(RESET)         Run security vulnerability scan"
	@echo -e "  $(CYAN)deps-audit$(RESET)            Comprehensive dependency audit"
	@echo -e "  $(CYAN)deps-verify$(RESET)           Verify dependency checksums"
	@echo ""
	@echo -e "$(YELLOW)📊 Analysis & Reporting:$(RESET)"
	@echo -e "  $(CYAN)deps-report$(RESET)           Generate comprehensive dependency report"
	@echo -e "  $(CYAN)deps-why MOD=...$(RESET)      Show why a module is needed"
	@echo ""
	@echo -e "$(YELLOW)🔄 Other Dependencies:$(RESET)"
	@echo -e "  $(CYAN)deps-update-actions$(RESET)   Check GitHub Actions updates"
	@echo -e "  $(CYAN)deps-update-docker$(RESET)    Check Docker base image updates"
	@echo ""
	@echo -e "$(YELLOW)💡 Maintenance Workflows:$(RESET)"
	@echo -e "  $(CYAN)Weekly:$(RESET)   deps-check → deps-security → deps-update-patch → test"
	@echo -e "  $(CYAN)Monthly:$(RESET)  deps-check → deps-update-minor → test → deps-security"
	@echo ""
	@echo -e "$(YELLOW)🧹 Cleanup:$(RESET)"
	@echo -e "  $(CYAN)deps-clean$(RESET)            Clean dependency cache and temporary files"
	@echo ""
	@echo -e "$(YELLOW)💡 Usage Examples:$(RESET)"
	@echo -e "  $(GREEN)make deps-outdated$(RESET)                 # Check direct dependencies"
	@echo -e "  $(GREEN)make deps-update-patch$(RESET)             # Safe patch updates"
	@echo -e "  $(GREEN)make deps-update-minor$(RESET)             # Minor version updates"
	@echo -e "  $(GREEN)make deps-why MOD=github.com/spf13/cobra$(RESET)  # Why is cobra needed?"
	@echo ""
	@echo -e "$(BLUE)📝 Configuration:$(RESET)"
	@echo "  This replaces Dependabot for more controlled dependency management"
	@echo -e "  Recommended: Run weekly $(YELLOW)deps-update-patch$(RESET) and monthly $(YELLOW)deps-update-minor$(RESET)"
