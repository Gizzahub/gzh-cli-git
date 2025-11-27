# Installation Guide

Complete installation instructions for `gzh-git`.

## Prerequisites

- **Go**: 1.21 or later
- **Git**: 2.30 or later
- **Operating System**: Linux, macOS, or Windows

## Quick Install

### Option 1: Using Go Install (Recommended)

```bash
go install github.com/gizzahub/gzh-cli-git/cmd/gzh-git@latest
```

The binary will be installed to `$GOPATH/bin` (usually `~/go/bin`).

### Option 2: From Source

```bash
# Clone the repository
git clone https://github.com/gizzahub/gzh-cli-git.git
cd gzh-cli-git

# Build
make build

# Install (requires sudo)
sudo make install

# Or install to custom location
make install PREFIX=$HOME/.local
```

### Option 3: Download Binary (Coming Soon)

Pre-built binaries will be available on the [Releases](https://github.com/gizzahub/gzh-cli-git/releases) page.

## Detailed Installation

### Building from Source

#### 1. Clone the Repository

```bash
git clone https://github.com/gizzahub/gzh-cli-git.git
cd gzh-cli-git
```

#### 2. Install Dependencies

```bash
go mod download
```

#### 3. Build

```bash
# Development build
make build

# Production build (optimized)
make build-release

# Build for specific platform
GOOS=linux GOARCH=amd64 make build
GOOS=darwin GOARCH=arm64 make build
GOOS=windows GOARCH=amd64 make build
```

The binary will be created in `build/gzh-git`.

#### 4. Install

```bash
# Install to /usr/local/bin (requires sudo)
sudo make install

# Install to custom location
make install PREFIX=$HOME/.local

# Or manually copy
cp build/gzh-git /usr/local/bin/
```

#### 5. Verify Installation

```bash
gzh-git --version
gzh-git --help
```

## Platform-Specific Instructions

### Linux

```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install -y git golang-go

# Fedora/RHEL
sudo dnf install -y git golang

# Arch Linux
sudo pacman -S git go

# Then install gzh-git
go install github.com/gizzahub/gzh-cli-git/cmd/gzh-git@latest
```

Add Go binaries to your PATH if not already done:

```bash
echo 'export PATH=$PATH:$HOME/go/bin' >> ~/.bashrc
source ~/.bashrc
```

### macOS

```bash
# Install prerequisites with Homebrew
brew install git go

# Install gzh-git
go install github.com/gizzahub/gzh-cli-git/cmd/gzh-git@latest
```

Add Go binaries to your PATH if needed:

```bash
echo 'export PATH=$PATH:$HOME/go/bin' >> ~/.zshrc
source ~/.zshrc
```

### Windows

```powershell
# Install prerequisites with Chocolatey
choco install git golang

# Install gzh-git
go install github.com/gizzahub/gzh-cli-git/cmd/gzh-git@latest
```

Add Go binaries to your PATH:

```powershell
$env:Path += ";$env:USERPROFILE\go\bin"
[Environment]::SetEnvironmentVariable("Path", $env:Path, [EnvironmentVariableTarget]::User)
```

## Shell Completion (Optional)

### Bash

```bash
# Generate completion script
gzh-git completion bash > /usr/local/etc/bash_completion.d/gzh-git

# Or for user-specific:
gzh-git completion bash > ~/.bash_completion.d/gzh-git
echo 'source ~/.bash_completion.d/gzh-git' >> ~/.bashrc
```

### Zsh

```zsh
# Generate completion script
gzh-git completion zsh > /usr/local/share/zsh/site-functions/_gzh-git

# Or for user-specific:
mkdir -p ~/.zsh/completion
gzh-git completion zsh > ~/.zsh/completion/_gzh-git
echo 'fpath=(~/.zsh/completion $fpath)' >> ~/.zshrc
echo 'autoload -Uz compinit && compinit' >> ~/.zshrc
```

### Fish

```fish
gzh-git completion fish > ~/.config/fish/completions/gzh-git.fish
```

## Configuration

### Default Configuration

gzh-git works out-of-the-box with sensible defaults.

### Custom Templates

Create custom commit templates:

```bash
# Create templates directory
mkdir -p ~/.config/gzh-git/templates

# Copy and customize a template
gzh-git commit template show conventional > ~/.config/gzh-git/templates/my-template.yaml
```

Edit the template and use it:

```bash
gzh-git commit auto --template my-template
```

### Environment Variables

Set default behavior with environment variables:

```bash
# Add to ~/.bashrc or ~/.zshrc
export GZH_GIT_TEMPLATE=conventional
export GZH_GIT_EDITOR=vim
```

## Upgrading

### From Go Install

```bash
go install github.com/gizzahub/gzh-cli-git/cmd/gzh-git@latest
```

### From Source

```bash
cd gzh-cli-git
git pull
make build
sudo make install
```

## Uninstallation

### Installed via Go

```bash
rm $(which gzh-git)
```

### Installed from Source

```bash
cd gzh-cli-git
sudo make uninstall
```

### Clean All Data

```bash
# Remove binaries
rm $(which gzh-git)

# Remove configuration (optional)
rm -rf ~/.config/gzh-git
```

## Troubleshooting Installation

### "command not found: gzh-git"

The binary is not in your PATH. Check:

```bash
# Find the binary
which gzh-git

# Check Go bin directory
ls -la $HOME/go/bin

# Add to PATH
export PATH=$PATH:$HOME/go/bin
```

### "permission denied"

You don't have permission to install to the target directory:

```bash
# Use sudo
sudo make install

# Or install to user directory
make install PREFIX=$HOME/.local
```

### Build Errors

Ensure you have the correct Go version:

```bash
go version  # Should be 1.21 or later

# Update Go if needed
# Visit https://go.dev/dl/
```

### Dependency Issues

```bash
# Clean and reinstall dependencies
go clean -modcache
go mod download
go mod tidy
```

## Verification

After installation, verify everything works:

```bash
# Check version
gzh-git --version

# Run help
gzh-git --help

# Test with a repository
cd /path/to/git/repo
gzh-git status
```

## Next Steps

- Read the [Quick Start Guide](QUICKSTART.md)
- Explore the [Command Reference](commands/README.md)
- Check out [Examples](examples/)

## Getting Help

- **GitHub Issues**: https://github.com/gizzahub/gzh-cli-git/issues
- **Documentation**: https://github.com/gizzahub/gzh-cli-git/tree/main/docs
- **Discussions**: https://github.com/gizzahub/gzh-cli-git/discussions
