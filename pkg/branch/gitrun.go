// Copyright (c) 2025 Gizzahub
// SPDX-License-Identifier: MIT

package branch

import (
	"context"
	"fmt"
	"strings"

	"github.com/gizzahub/gzh-cli-gitforge/internal/gitcmd"
)

// runGit executes a git command and turns a non-zero exit into an error.
//
// gitcmd.Executor.Run reports a failed git through Result.ExitCode and returns a
// nil error unless the process itself could not be started, so a caller that
// checks err alone accepts every git failure as a success. That reading is right
// for a query whose answer *is* the exit code — `Exists` asks git to resolve a
// ref and treats a non-zero exit as "no such branch" — and wrong for everything
// that asks git to do something, where a non-zero exit means the work did not
// happen.
//
// This package has far more of the second kind, so they share a helper and the
// handful of deliberate exit-code readers call the executor directly, where the
// `result.ExitCode` in their body says which reading they meant.
func runGit(ctx context.Context, executor *gitcmd.Executor, dir string, args ...string) (*gitcmd.Result, error) {
	result, err := executor.Run(ctx, dir, args...)
	if err != nil {
		return nil, err
	}

	if result.ExitCode != 0 {
		return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(result.Stderr))
	}

	return result, nil
}
