# 11. Deployment Architecture

> gzh-cli-gitforge 아키텍처 문서 · [인덱스](README.md) · [ARCHITECTURE.md](../../ARCHITECTURE.md)

### 11.1 Build Pipeline

```
┌───────────────┐
│ Source Code   │
│ (main branch) │
└───────┬───────┘
        │
        ▼
┌───────────────┐
│ GitHub Actions│
│ CI/CD         │
└───────┬───────┘
        │
        ├───▶ golangci-lint
        ├───▶ go test (unit, integration)
        ├───▶ go test -race (race detection)
        ├───▶ gosec (security scan)
        │
        ▼
┌───────────────┐
│ Build Binary  │
│ (multi-platform)
└───────┬───────┘
        │
        ├───▶ linux/amd64
        ├───▶ linux/arm64
        ├───▶ darwin/amd64
        ├───▶ darwin/arm64
        ├───▶ windows/amd64
        │
        ▼
┌───────────────┐
│ Create Release│
│ (GitHub)      │
└───────┬───────┘
        │
        ├───▶ Upload binaries
        ├───▶ Generate checksums
        ├───▶ Sign with GPG
        │
        ▼
┌───────────────┐
│ Publish       │
│ - Go module   │
│ - Homebrew    │
│ - APT/YUM     │
└───────────────┘
```

### 11.2 Deployment Targets

**Go Module (Primary):**

```bash
go get github.com/gizzahub/gzh-cli-gitforge@latest
```

**Homebrew Cask (macOS):**

```bash
brew install --cask gizzahub/tap/gz-git
```

**Direct Download:**

Download the archive for your platform from the
[latest GitHub release](https://github.com/gizzahub/gzh-cli-gitforge/releases/latest).
Linux amd64 archives follow the GoReleaser name
`gz-git_<version>_linux_x86_64.tar.gz`; extract the archive before running
`gz-git`.
