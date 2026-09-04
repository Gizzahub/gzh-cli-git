// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package contextref

type wtRoot struct {
	file interface{ Close() error }
}

func (r *wtRoot) Close() error {
	if r == nil || r.file == nil {
		return nil
	}
	return r.file.Close()
}

type nodeInfo struct {
	symlink bool
	regular bool
	dir     bool
	ident   fileIdent
}
