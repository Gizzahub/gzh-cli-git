// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package contextref

import "errors"

var (
	errPath            = errors.New("entrypoint path rejected")
	errSymlink         = errors.New("symlink rejected")
	errNonRegular      = errors.New("non-regular file rejected")
	errLimit           = errors.New("size limit exceeded")
	errHostileYAML     = errors.New("hostile yaml rejected")
	errMultipleDocs    = errors.New("multiple yaml documents rejected")
	errUnsupportedOpen = errors.New("secure open is unavailable on this platform")
	errChanged         = errors.New("file identity changed during read")
	errNotGit          = errors.New("not a git repository")
	errRelativeCE      = errors.New("ce path must be absolute")
	errCEDigest        = errors.New("ce digest must be sha256 lowercase hex")
	errTimeoutRange    = errors.New("ce timeout must be between 1ns and 30s")
)
