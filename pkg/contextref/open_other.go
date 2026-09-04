// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

//go:build !unix

package contextref

import "os"

func bindWorktreeRoot(string) (*wtRoot, error) {
	return nil, errUnsupportedOpen
}

func worktreePresent(*wtRoot, string) (nodeInfo, bool, error) {
	return nodeInfo{}, false, errUnsupportedOpen
}

func readRelativeBounded(*wtRoot, string, int) ([]byte, error) {
	return nil, errUnsupportedOpen
}

func lstatNoFollow(path string) (os.FileInfo, error) {
	return os.Lstat(path)
}

func openExecNoFollow(string) (*os.File, error) {
	return nil, errUnsupportedOpen
}

func identFromInfo(info os.FileInfo) fileIdent {
	return fileIdent{size: info.Size(), mtime: info.ModTime().UnixNano()}
}

func identFromFile(f *os.File) (fileIdent, error) {
	info, err := f.Stat()
	if err != nil {
		return fileIdent{}, err
	}
	return identFromInfo(info), nil
}
