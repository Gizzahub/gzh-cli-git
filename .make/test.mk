# Makefile.test - Testing targets for gzh-cli
# Unit tests, integration tests, benchmarks, and coverage

# ==============================================================================
# Testing Configuration
# ==============================================================================

# ==============================================================================
# Testing Targets
# ==============================================================================

.PHONY: test test-unit test-unit-quality test-integration-quality test-integration test-integration-only test-e2e test-e2e-only test-all
.PHONY: cover cover-html cover-report bench test-coverage test-docker

test: clean build ## run all tests with coverage (requires binary for integration tests)
	@echo -e "$(CYAN)Running all tests with coverage...$(RESET)"
	go test --cover -parallel=1 -v -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | sort -rnk3
	@echo -e "$(GREEN)✅ Tests completed$(RESET)"

test-unit: ## run only unit tests (exclude integration and e2e)
	@echo -e "$(CYAN)Running unit tests...$(RESET)"
	go test -short --cover -parallel=1 -v -coverprofile=coverage-unit.out \
		$$(go list ./... | grep -v -E '(tests/integration|tests/e2e)')
	go tool cover -func=coverage-unit.out | sort -rnk3
	@echo -e "$(GREEN)✅ Unit tests completed$(RESET)"

# quality-check is deliberately source-non-mutating with respect to the
# checkout. Keep the coverage profile in a private temporary directory; CI can
# opt into a runner-temporary output by setting QUALITY_COVERAGE_OUT for Codecov
# upload.
test-unit-quality: ## run unit tests without leaving a coverage artifact in the checkout
	@set -eu; \
	export GOWORK=off; \
	quality_tmp=$$(mktemp -d "$${TMPDIR:-/tmp}/gzh-cli-quality-tests.XXXXXX"); \
	trap 'rm -rf "$$quality_tmp"' EXIT HUP INT TERM; \
	coverage_out="$${QUALITY_COVERAGE_OUT:-$$quality_tmp/coverage-unit.out}"; \
	echo -e "$(CYAN)Running unit tests with coverage...$(RESET)"; \
	GOWORK=off go test -short --cover -parallel=1 -v -coverprofile="$$coverage_out" \
		$$(GOWORK=off go list ./... | grep -v -E '(tests/integration|tests/e2e)'); \
	go tool cover -func="$$coverage_out" | sort -rnk3; \
	echo -e "$(GREEN)✅ Unit tests completed$(RESET)"

# Keep the integration package self-contained for the canonical quality gate.
# Its TestMain builds gz-git in a private temporary directory and each test
# uses t.TempDir, so this target must not require a checked-out binary or leave
# build/coverage artifacts in the repository.
test-integration-quality: ## run package integration tests without repository artifacts
	@set -eu; \
	export GOWORK=off; \
	echo -e "$(CYAN)Running package integration tests...$(RESET)"; \
	GOWORK=off go test -short -count=1 -v ./tests/integration/...; \
	echo -e "$(GREEN)✅ Package integration tests completed$(RESET)"

test-integration-only: build ## run only integration tests with build tag
	@echo -e "$(CYAN)Running integration tests...$(RESET)"
	@if [ -d "./tests/integration" ]; then \
		cd tests/integration && go test -v .; \
	else \
		echo -e "$(YELLOW)No integration tests found$(RESET)"; \
	fi
	@echo -e "$(GREEN)✅ Integration tests completed$(RESET)"

test-e2e-only: ## run only e2e tests with build tag
	@echo -e "$(CYAN)Running E2E tests...$(RESET)"
	@if [ -d "./tests/e2e" ]; then \
		go test -tags=e2e -v ./tests/e2e/...; \
	else \
		echo -e "$(YELLOW)No e2e tests found$(RESET)"; \
	fi
	@echo -e "$(GREEN)✅ E2E tests completed$(RESET)"

test-integration: ## run Docker-based integration tests (alias for test-docker)
	@echo -e "$(CYAN)Running Docker integration tests...$(RESET)"
	@if [ -f "./tests/integration/run_docker_tests.sh" ]; then \
		./tests/integration/run_docker_tests.sh all; \
	else \
		echo -e "$(YELLOW)No Docker integration test script found$(RESET)"; \
		make test-integration-only; \
	fi
	@echo -e "$(GREEN)✅ Integration tests completed$(RESET)"

test-e2e: build ## run End-to-End test scenarios
	@echo -e "$(CYAN)Running E2E tests...$(RESET)"
	@if [ -f "./tests/e2e/run_e2e_tests.sh" ]; then \
		./tests/e2e/run_e2e_tests.sh all; \
	else \
		echo -e "$(YELLOW)No E2E test script found$(RESET)"; \
		make test-e2e-only; \
	fi
	@echo -e "$(GREEN)✅ E2E tests completed$(RESET)"
test-all: test test-integration test-e2e ## run all tests (unit, integration, e2e)
	@echo -e "$(GREEN)✅ All tests completed successfully!$(RESET)"

test-docker: test-integration ## alias for test-integration

# ==============================================================================
# Coverage Targets
# ==============================================================================

cover: ## display test coverage
	@echo -e "$(CYAN)Generating test coverage report...$(RESET)"
	go test -v -race $$(go list ./... | grep -v /vendor/) -v -coverprofile=coverage.out
	go tool cover -func=coverage.out
	@echo -e "$(GREEN)✅ Coverage report generated$(RESET)"

cover-html: ## generate HTML coverage report
	@echo -e "$(CYAN)Generating HTML coverage report...$(RESET)"
	go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo -e "$(GREEN)✅ Coverage report generated: coverage.html$(RESET)"

cover-report: ## generate detailed coverage report
	@echo -e "$(CYAN)Generating detailed coverage report...$(RESET)"
	@go test -coverprofile=coverage.out -covermode=atomic ./...
	@echo ""
	@echo -e "$(YELLOW)=== Coverage Summary ===$(RESET)"
	@go tool cover -func=coverage.out | grep total | awk '{print "Total Coverage: " $$3}'
	@echo ""
	@echo -e "$(YELLOW)=== Package Coverage ===$(RESET)"
	@go tool cover -func=coverage.out | grep -v total | sort -k3 -nr | head -20
	@echo ""
	@echo -e "$(BLUE)For detailed HTML report, run: make cover-html$(RESET)"

test-coverage: cover-report ## alias for cover-report

# ==============================================================================
# Benchmark Targets
# ==============================================================================

bench: ## run all benchmarks
	@echo -e "$(CYAN)Running benchmarks...$(RESET)"
	@go test -bench=. -benchmem ./...
	@echo -e "$(GREEN)✅ Benchmarks completed$(RESET)"

bench-cpu: ## run CPU benchmarks with profiling
	@echo -e "$(CYAN)Running CPU benchmarks with profiling...$(RESET)"
	@go test -bench=. -benchmem -cpuprofile=cpu.prof ./...
	@echo -e "$(GREEN)✅ CPU benchmarks completed$(RESET)"
	@echo -e "$(YELLOW)Use 'go tool pprof cpu.prof' to analyze$(RESET)"

bench-mem: ## run memory benchmarks with profiling
	@echo -e "$(CYAN)Running memory benchmarks with profiling...$(RESET)"
	@go test -bench=. -benchmem -memprofile=mem.prof ./...
	@echo -e "$(GREEN)✅ Memory benchmarks completed$(RESET)"
	@echo -e "$(YELLOW)Use 'go tool pprof mem.prof' to analyze$(RESET)"

bench-compare: ## compare benchmarks (requires benchstat)
	@echo -e "$(CYAN)Comparing benchmarks...$(RESET)"
	@command -v benchstat >/dev/null 2>&1 || { echo "Installing benchstat..." && go install golang.org/x/perf/cmd/benchstat@latest; }
	@go test -bench=. -count=5 ./... > new.bench
	@echo -e "$(GREEN)✅ Benchmark comparison data generated: new.bench$(RESET)"
	@echo -e "$(YELLOW)Run 'benchstat old.bench new.bench' to compare$(RESET)"

# ==============================================================================
# Test Utilities
# ==============================================================================

test-race: ## run tests with race detection
	@echo -e "$(CYAN)Running tests with race detection...$(RESET)"
	@go test -race -short ./...
	@echo -e "$(GREEN)✅ Race detection tests completed$(RESET)"

test-verbose: ## run tests with verbose output
	@echo -e "$(CYAN)Running tests with verbose output...$(RESET)"
	@go test -v ./...
	@echo -e "$(GREEN)✅ Verbose tests completed$(RESET)"

test-timeout: ## run tests with custom timeout
	@echo -e "$(CYAN)Running tests with 30s timeout...$(RESET)"
	@go test -timeout=30s ./...
	@echo -e "$(GREEN)✅ Timeout tests completed$(RESET)"

test-list: ## list all available tests
	@echo -e "$(CYAN)Listing all available tests...$(RESET)"
	@go test -list . ./... | grep -E '^Test|^Benchmark'
	@echo -e "$(GREEN)✅ Test listing completed$(RESET)"

# ==============================================================================
# Test Information
# ==============================================================================

.PHONY: test-info

test-info: ## show testing information and available targets
	@echo -e "$(CYAN)"
	@echo "╔══════════════════════════════════════════════════════════════════════════════╗"
	@echo -e "║                         $(YELLOW)Testing Information$(CYAN)                             ║"
	@echo "╚══════════════════════════════════════════════════════════════════════════════╝"
	@echo -e "$(RESET)"
	@echo -e "$(GREEN)🧪 Test Categories:$(RESET)"
	@echo -e "  • $(CYAN)Unit Tests$(RESET)          Fast, isolated component tests"
	@echo -e "  • $(CYAN)Integration Tests$(RESET)   Docker-based service integration"
	@echo -e "  • $(CYAN)E2E Tests$(RESET)           End-to-end scenario testing"
	@echo ""
	@echo -e "$(GREEN)📊 Coverage Targets:$(RESET)"
	@echo -e "  • $(CYAN)cover$(RESET)               Display test coverage"
	@echo -e "  • $(CYAN)cover-html$(RESET)          Generate HTML coverage report"
	@echo -e "  • $(CYAN)cover-report$(RESET)        Detailed coverage analysis"
	@echo ""
	@echo -e "$(GREEN)⚡ Benchmark Targets:$(RESET)"
	@echo -e "  • $(CYAN)bench$(RESET)               Run all benchmarks"
	@echo -e "  • $(CYAN)bench-cpu$(RESET)           CPU benchmarks with profiling"
	@echo -e "  • $(CYAN)bench-mem$(RESET)           Memory benchmarks with profiling"
	@echo -e "  • $(CYAN)bench-compare$(RESET)       Compare benchmark results"
	@echo ""
	@echo -e "$(GREEN)🔧 Test Utilities:$(RESET)"
	@echo -e "  • $(CYAN)test-race$(RESET)           Run with race detection"
	@echo -e "  • $(CYAN)test-verbose$(RESET)        Run with verbose output"
	@echo -e "  • $(CYAN)test-timeout$(RESET)        Run with custom timeout"
	@echo -e "  • $(CYAN)test-list$(RESET)           List all available tests"
