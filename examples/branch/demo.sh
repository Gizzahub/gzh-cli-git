#!/bin/bash
# Branch Management Demo
# Demonstrates gzh-git branch features

echo "=== gzh-git Branch Management Demo ==="
echo

# Check if gzh-git is installed
if ! command -v gzh-git &> /dev/null; then
    echo "Error: gzh-git is not installed"
    echo "Install with: go install github.com/gizzahub/gzh-cli-git/cmd/gzh-git@latest"
    exit 1
fi

# Example 1: List branches
echo "1. List Local Branches"
echo "----------------------"
gzh-git branch list
echo

echo "2. List All Branches (including remote)"
echo "---------------------------------------"
gzh-git branch list --all
echo

# Example 2: Branch creation (demo only)
echo "3. Create Branch (Example)"
echo "--------------------------"
echo "To create a new branch:"
echo "  gzh-git branch create feature/new-feature"
echo
echo "To create with worktree:"
echo "  gzh-git branch create feature/parallel --worktree /tmp/parallel-work"
echo

# Example 3: Branch deletion (demo only)
echo "4. Delete Branch (Example)"
echo "--------------------------"
echo "To delete a branch:"
echo "  gzh-git branch delete feature/old-feature"
echo

echo "Demo complete!"
echo
echo "Try these commands yourself:"
echo "  gzh-git branch list --all"
echo "  gzh-git branch create <name>"
echo "  gzh-git branch delete <name>"
