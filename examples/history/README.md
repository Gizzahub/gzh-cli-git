# History Analysis Example

This example demonstrates gzh-git history analysis features using the CLI.

## Features Demonstrated

1. **Commit Statistics**: Analyze commit counts, authors, and changes over time
2. **Contributor Analysis**: Rank contributors by commits and code changes
3. **File History**: Track changes to specific files

## Usage

### Get Commit Statistics

```bash
# Stats for last 30 days
gzh-git history stats --since "30 days ago"

# Stats for specific date range
gzh-git history stats --since "2025-01-01" --until "2025-01-31"
```

### Analyze Contributors

```bash
# Top 10 contributors
gzh-git history contributors --top 10

# Contributors in last month
gzh-git history contributors --since "1 month ago"
```

### View File History

```bash
# History of specific file
gzh-git history file README.md

# Last 5 commits affecting file
gzh-git history file src/main.go --limit 5
```

## Output Formats

All history commands support multiple output formats:

```bash
# Table format (default)
gzh-git history stats

# JSON format
gzh-git history stats --format json

# CSV format
gzh-git history contributors --format csv
```

## Library Usage

For library integration, see [Library Guide](../../docs/LIBRARY.md).

See [pkg/history](../../pkg/history) for complete API documentation.
