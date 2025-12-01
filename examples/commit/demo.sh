#!/bin/bash
# Commit Automation Demo
# Demonstrates gzh-git commit features

echo "=== gzh-git Commit Automation Demo ==="
echo

# Check if gzh-git is installed
if ! command -v gzh-git &> /dev/null; then
    echo "Error: gzh-git is not installed"
    echo "Install with: go install github.com/gizzahub/gzh-cli-git/cmd/gzh-git@latest"
    exit 1
fi

# Example 1: Validate commit messages
echo "1. Validate Commit Messages"
echo "----------------------------"

echo "Valid message:"
gzh-git commit validate "feat(cli): add status command"
echo

echo "Invalid message (missing scope):"
gzh-git commit validate "feat add status command" || echo "❌ Validation failed (expected)"
echo

# Example 2: List templates
echo "2. List Available Templates"
echo "----------------------------"
gzh-git commit template list
echo

# Example 3: Show template details
echo "3. Show Template Details"
echo "------------------------"
gzh-git commit template show conventional
echo

# Example 4: Auto-generate (requires staged changes)
echo "4. Auto-Generate Commit Message"
echo "--------------------------------"
echo "This requires staged changes in the repository"
echo "Example usage:"
echo "  git add <files>"
echo "  gzh-git commit auto"
echo

echo "Demo complete!"
echo
echo "Try these commands yourself:"
echo "  gzh-git commit validate \"<message>\""
echo "  gzh-git commit template list"
echo "  gzh-git commit auto"
