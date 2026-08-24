# Makefile.dev - Development Workflow for gzh-cli
# Development environment, workflow automation, and quick iteration

# ==============================================================================
# Development Configuration
# ==============================================================================

# ==============================================================================
# Quick Access Aliases for Development
# ==============================================================================

.PHONY: start stop restart status logs quick full setup-all

# Quick development aliases
start: run                     ## quick start: run development server
stop:                         ## stop running development server
	@echo -e "$(YELLOW)Stopping development server...$(RESET)"
	@pkill -f "$(executablename)" || echo -e "$(GREEN)No running $(executablename) processes found$(RESET)"

restart: stop start           ## restart development server

status:                       ## check development server status
	@echo -e "$(CYAN)Checking for running $(executablename) processes...$(RESET)"
	@pgrep -f "$(executablename)" > /dev/null && echo -e "$(GREEN)✅ $(executablename) is running$(RESET)" || echo -e "$(RED)❌ $(executablename) is not running$(RESET)"

logs:                         ## show recent log files
	@echo -e "$(CYAN)Recent log files:$(RESET)"
	@find . -name "*.log" -type f -exec ls -la {} \; 2>/dev/null || echo -e "$(YELLOW)No log files found$(RESET)"

# ==============================================================================
# Development Workflow Targets
# ==============================================================================

.PHONY: dev dev-fast verify ci-local pr-check

dev: quality-check ## run the canonical source-non-mutating quality gate
	@echo -e "$(GREEN)✅ Standard development workflow completed!$(RESET)"

dev-fast: fmt test-unit ## quick development cycle (format and unit tests only)
	@echo -e "$(GREEN)✅ Fast development cycle completed!$(RESET)"

verify: quality-check check-consistency ## complete verification before PR
	@echo -e "$(GREEN)✅ Complete verification completed!$(RESET)"

ci-local: quality-check ## run the same canonical CI quality gate locally
	@echo -e "$(GREEN)✅ Local CI pipeline completed!$(RESET)"

pr-check: quality-check check-consistency ## pre-PR submission check
	@echo -e "$(GREEN)✅ Pre-PR check completed - ready for submission!$(RESET)"

# ==============================================================================
# Main Workflow Aliases
# ==============================================================================

quick: fmt lint-check test-unit ## quick development check (format + lint + unit tests)
	@echo -e "$(GREEN)✅ Quick development check completed!$(RESET)"

full: quality-check ## full quality check (comprehensive)
	@echo -e "$(GREEN)✅ Full quality check completed!$(RESET)"

setup-all: bootstrap install-tools ## complete project setup (dependencies + all tools)
	@echo -e "$(GREEN)🎉 Complete project setup finished!$(RESET)"

# ==============================================================================
# Code Analysis and Comments
# ==============================================================================

.PHONY: comments todo fixme notes deps-graph

comments: ## show all TODO/FIXME/NOTE comments in codebase
	@echo -e "$(CYAN)=== TODO comments ===$(RESET)"
	@grep -r "TODO" --include="*.go" . | grep -v vendor | grep -v .git || echo -e "$(GREEN)No TODOs found!$(RESET)"
	@echo ""
	@echo -e "$(CYAN)=== FIXME comments ===$(RESET)"
	@grep -r "FIXME" --include="*.go" . | grep -v vendor | grep -v .git || echo -e "$(GREEN)No FIXMEs found!$(RESET)"
	@echo ""
	@echo -e "$(CYAN)=== NOTE comments ===$(RESET)"
	@grep -r "NOTE" --include="*.go" . | grep -v vendor | grep -v .git || echo -e "$(GREEN)No NOTEs found!$(RESET)"

# Aliases for backward compatibility
todo: comments ## show all TODO comments (alias for comments)
fixme: comments ## show all FIXME comments (alias for comments)
notes: comments ## show all NOTE comments (alias for comments)

deps-graph: ## show module dependency graph
	@echo -e "$(CYAN)Module dependency graph:$(RESET)"
	@go mod graph

# ==============================================================================
# Documentation Generation
# ==============================================================================

.PHONY: docs-serve docs-build godoc docs-check

# No changelog target on purpose. CHANGELOG.md is written by hand — each entry
# explains why a change was made, which a generator cannot produce — and
# `git-chglog -o CHANGELOG.md` overwrites rather than appends. The mechanical
# per-release list already comes from .goreleaser.yaml (changelog.use: github).
# See tasks/issue/24-make-changelog-overwrites-handwritten-log.md

docs-serve: ## serve documentation locally (requires mkdocs)
	@command -v mkdocs >/dev/null 2>&1 || { echo -e "$(RED)mkdocs not found. Install with: pip install mkdocs mkdocs-material$(RESET)"; exit 1; }
	@echo -e "$(CYAN)Starting documentation server...$(RESET)"
	@mkdocs serve

docs-build: ## build documentation site
	@command -v mkdocs >/dev/null 2>&1 || { echo -e "$(RED)mkdocs not found. Install with: pip install mkdocs mkdocs-material$(RESET)"; exit 1; }
	@echo -e "$(CYAN)Building documentation site...$(RESET)"
	@mkdocs build

godoc: ## run godoc server
	@echo -e "$(CYAN)Starting godoc server on http://localhost:6060$(RESET)"
	@godoc -http=:6060

docs-check: ## check for missing package documentation
	@echo -e "$(CYAN)Checking for missing package documentation...$(RESET)"
	@for pkg in $$(go list ./...); do \
		if ! go doc -short $$pkg | grep -q "^package"; then \
			echo -e "$(RED)Missing documentation for: $$pkg$(RESET)"; \
		fi; \
	done || echo -e "$(GREEN)✅ All packages have documentation$(RESET)"

# ==============================================================================
# Development Environment Information
# ==============================================================================

.PHONY: dev-info dev-status

dev-info: ## show development environment information
	@echo -e "$(CYAN)"
	@echo "╔══════════════════════════════════════════════════════════════════════════════╗"
	@echo -e "║                         $(MAGENTA)Development Environment$(CYAN)                         ║"
	@echo "╚══════════════════════════════════════════════════════════════════════════════╝"
	@echo -e "$(RESET)"
	@echo -e "$(GREEN)🏗️  Environment Details:$(RESET)"
	@echo "  Go Version:     $$(go version | cut -d' ' -f3)"
	@echo -e "  GOPROXY:        $(GOPROXY)"
	@echo -e "  GOSUMDB:        $(GOSUMDB)"
	@echo "  GOPATH:         $$(go env GOPATH)"
	@echo "  GOROOT:         $$(go env GOROOT)"
	@echo ""
	@echo -e "$(GREEN)🔄 Development Workflows:$(RESET)"
	@echo -e "  • $(CYAN)dev$(RESET)                 Standard development workflow"
	@echo -e "  • $(CYAN)dev-fast$(RESET)            Quick development cycle"
	@echo -e "  • $(CYAN)quick$(RESET)               Quick check (format + lint + unit tests)"
	@echo -e "  • $(CYAN)full$(RESET)                Full quality check"
	@echo -e "  • $(CYAN)verify$(RESET)              Complete verification before PR"
	@echo -e "  • $(CYAN)ci-local$(RESET)            Run full CI pipeline locally"
	@echo -e "  • $(CYAN)pr-check$(RESET)            Pre-PR submission check"
	@echo ""
	@echo -e "$(GREEN)🚀 Quick Commands:$(RESET)"
	@echo -e "  • $(CYAN)start$(RESET)               Start development server"
	@echo -e "  • $(CYAN)stop$(RESET)                Stop development server"
	@echo -e "  • $(CYAN)restart$(RESET)             Restart development server"
	@echo -e "  • $(CYAN)status$(RESET)              Check server status"
	@echo -e "  • $(CYAN)logs$(RESET)                Show recent log files"

dev-status: ## show current development status
	@echo -e "$(CYAN)Development Status Check$(RESET)"
	@echo -e "$(BLUE)========================$(RESET)"
	@echo ""
	@echo -e "$(GREEN)📊 Project Status:$(RESET)"
	@printf "  %-20s " "Git Status:"; if git status --porcelain | grep -q .; then echo -e "$(YELLOW)Modified files$(RESET)"; else echo -e "$(GREEN)Clean$(RESET)"; fi
	@printf "  %-20s " "Current Branch:"; git branch --show-current 2>/dev/null || echo -e "$(RED)Unknown$(RESET)"
	@printf "  %-20s " "Last Commit:"; git log -1 --format="%h %s" 2>/dev/null | cut -c1-50 || echo -e "$(RED)No commits$(RESET)"
	@echo ""
	@echo -e "$(GREEN)🔧 Build Status:$(RESET)"
	@printf "  %-20s " "Binary Exists:"; if [ -f "$(executablename)" ]; then echo -e "$(GREEN)Yes$(RESET)"; else echo -e "$(YELLOW)No$(RESET)"; fi
	@printf "  %-20s " "Coverage File:"; if [ -f "coverage.out" ]; then echo -e "$(GREEN)Yes$(RESET)"; else echo -e "$(YELLOW)No$(RESET)"; fi
	@echo ""
	@echo -e "$(GREEN)🎯 Quick Actions:$(RESET)"
	@echo -e "  • $(CYAN)make quick$(RESET)          Quick development check"
	@echo -e "  • $(CYAN)make dev$(RESET)            Full development workflow"
	@echo -e "  • $(CYAN)make setup-all$(RESET)      Set up everything from scratch"

# ==============================================================================
# Version Management (VERSION file based, no tags for patch versions)
# ==============================================================================

.PHONY: ver ver-up-a ver-up-b ver-up-c ver-set ver-tag-push

ver: ## show current version
	@echo -e "$(CYAN)Current version: $(GREEN)$(VERSION)$(RESET)"

ver-up-a: ## bump major version (a.b.c -> a+1.0.0) + commit + create new vN tag
	@current=$$(cat VERSION 2>/dev/null || echo "0.0.0"); \
	major=$$(echo $$current | cut -d. -f1); \
	new_major=$$((major + 1)); \
	new_version="$$new_major.0.0"; \
	echo $$new_version > VERSION; \
	echo -e "$(GREEN)Version bumped: $$current -> $$new_version$(RESET)"; \
	git add VERSION; \
	git commit -m "chore(version): bump to $$new_version"; \
	echo -e "$(CYAN)Creating tag v$$new_major...$(RESET)"; \
	git tag -a "v$$new_major" -m "Release v$$new_major ($$new_version)"; \
	echo -e "$(GREEN)✅ Committed and tagged. Push with:$(RESET)"; \
	echo -e "   git push origin master && git push origin v$$new_major"

ver-up-b: ## bump minor version (a.b.c -> a.b+1.0) + commit + update rolling vN tag
	@current=$$(cat VERSION 2>/dev/null || echo "0.0.0"); \
	major=$$(echo $$current | cut -d. -f1); \
	minor=$$(echo $$current | cut -d. -f2); \
	new_minor=$$((minor + 1)); \
	new_version="$$major.$$new_minor.0"; \
	echo $$new_version > VERSION; \
	echo -e "$(GREEN)Version bumped: $$current -> $$new_version$(RESET)"; \
	git add VERSION; \
	git commit -m "chore(version): bump to $$new_version"; \
	echo -e "$(CYAN)Updating rolling tag v$$major...$(RESET)"; \
	git tag -d "v$$major" 2>/dev/null || true; \
	git tag -a "v$$major" -m "Release v$$major ($$new_version)"; \
	echo -e "$(GREEN)✅ Committed and tagged. Push with:$(RESET)"; \
	echo -e "   git push origin master && git push origin v$$major --force"

ver-up-c: ## bump patch version (a.b.c -> a.b.c+1) + commit + update rolling vN tag
	@current=$$(cat VERSION 2>/dev/null || echo "0.0.0"); \
	major=$$(echo $$current | cut -d. -f1); \
	minor=$$(echo $$current | cut -d. -f2); \
	patch=$$(echo $$current | cut -d. -f3); \
	new_patch=$$((patch + 1)); \
	new_version="$$major.$$minor.$$new_patch"; \
	echo $$new_version > VERSION; \
	echo -e "$(GREEN)Version bumped: $$current -> $$new_version$(RESET)"; \
	git add VERSION; \
	git commit -m "chore(version): bump to $$new_version"; \
	echo -e "$(CYAN)Updating rolling tag v$$major...$(RESET)"; \
	git tag -d "v$$major" 2>/dev/null || true; \
	git tag -a "v$$major" -m "Release v$$major ($$new_version)"; \
	echo -e "$(GREEN)✅ Committed and tagged. Push with:$(RESET)"; \
	echo -e "   git push origin master && git push origin v$$major --force"

ver-set: ## set version manually (usage: make ver-set V=1.2.3)
	@if [ -z "$(V)" ]; then \
		echo -e "$(RED)Usage: make ver-set V=1.2.3$(RESET)"; \
		exit 1; \
	fi; \
	echo $(V) > VERSION; \
	echo -e "$(GREEN)Version set to: $(V)$(RESET)"

ver-tag-push: ## push the rolling major tag (force push)
	@version=$$(cat VERSION 2>/dev/null); \
	major=$$(echo $$version | cut -d. -f1); \
	echo -e "$(CYAN)Pushing tag v$$major (force)...$(RESET)"; \
	git push origin "v$$major" --force; \
	echo -e "$(GREEN)Tag v$$major pushed$(RESET)"
