// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

//go:build unix

package contextref

import (
	"errors"
	"io"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

func bindWorktreeRoot(path string) (*wtRoot, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, errSymlink
	}
	if !info.IsDir() {
		return nil, errNonRegular
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, unix.ELOOP) {
			return nil, errSymlink
		}
		return nil, err
	}
	return &wtRoot{file: os.NewFile(uintptr(fd), path)}, nil
}

func worktreePresent(root *wtRoot, rel string) (nodeInfo, bool, error) {
	parts := strings.Split(rel, "/")
	parent := rootFD(root)
	opened := make([]*os.File, 0, len(parts))
	defer func() {
		for i := len(opened) - 1; i >= 0; i-- {
			_ = opened[i].Close()
		}
	}()
	for i, part := range parts {
		st, err := fstatatFD(parent, part)
		if err != nil {
			if errors.Is(err, unix.ENOENT) {
				return nodeInfo{}, false, nil
			}
			return nodeInfo{}, false, err
		}
		info := nodeFromStat(st)
		if info.symlink || i == len(parts)-1 || !info.dir {
			return info, true, nil
		}
		next, err := openatFD(parent, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC)
		if err != nil {
			return nodeInfo{}, false, err
		}
		opened = append(opened, next)
		parent = int(next.Fd())
	}
	return nodeInfo{}, false, nil
}

func readRelativeBounded(root *wtRoot, rel string, limit int) ([]byte, error) {
	file, before, err := openRelativeNoFollow(root, rel)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, errLimit
	}
	after, err := identFromFile(file)
	if err != nil {
		return nil, err
	}
	if after != before {
		return nil, errChanged
	}
	return data, nil
}

func openRelativeNoFollow(root *wtRoot, rel string) (*os.File, fileIdent, error) {
	parts := strings.Split(rel, "/")
	parent := rootFD(root)
	opened := make([]*os.File, 0, len(parts))
	defer func() {
		for i := len(opened) - 1; i >= 0; i-- {
			_ = opened[i].Close()
		}
	}()
	for i, part := range parts {
		st, err := fstatatFD(parent, part)
		if err != nil {
			return nil, fileIdent{}, err
		}
		info := nodeFromStat(st)
		if info.symlink {
			return nil, fileIdent{}, errSymlink
		}
		if i == len(parts)-1 {
			if !info.regular {
				return nil, fileIdent{}, errNonRegular
			}
			f, err := openatFD(parent, part, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC)
			if err != nil {
				return nil, fileIdent{}, err
			}
			ident, err := identFromFile(f)
			if err != nil {
				_ = f.Close()
				return nil, fileIdent{}, err
			}
			if ident != info.ident {
				_ = f.Close()
				return nil, fileIdent{}, errChanged
			}
			return f, ident, nil
		}
		if !info.dir {
			return nil, fileIdent{}, errNonRegular
		}
		next, err := openatFD(parent, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC)
		if err != nil {
			return nil, fileIdent{}, err
		}
		opened = append(opened, next)
		parent = int(next.Fd())
	}
	return nil, fileIdent{}, errNonRegular
}

func fstatatFD(fd int, name string) (unix.Stat_t, error) {
	var st unix.Stat_t
	if err := unix.Fstatat(fd, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return unix.Stat_t{}, err
	}
	return st, nil
}

func openatFD(fd int, name string, flags int) (*os.File, error) {
	nfd, err := unix.Openat(fd, name, flags, 0)
	if err != nil {
		if errors.Is(err, unix.ELOOP) {
			return nil, errSymlink
		}
		return nil, err
	}
	return os.NewFile(uintptr(nfd), name), nil
}

func rootFD(root *wtRoot) int {
	f, ok := root.file.(*os.File)
	if !ok {
		return -1
	}
	return int(f.Fd())
}

func nodeFromStat(st unix.Stat_t) nodeInfo {
	kind := st.Mode & unix.S_IFMT
	return nodeInfo{
		symlink: kind == unix.S_IFLNK,
		regular: kind == unix.S_IFREG,
		dir:     kind == unix.S_IFDIR,
		ident:   identFromStat(st),
	}
}

func identFromStat(st unix.Stat_t) fileIdent {
	return fileIdent{
		size:  st.Size,
		mtime: st.Mtim.Nano(),
		ctime: st.Ctim.Nano(),
		dev:   uint64(st.Dev), //nolint:gosec // G115: device ID is an opaque kernel identifier
		ino:   st.Ino,
	}
}

func lstatNoFollow(path string) (os.FileInfo, error) {
	return os.Lstat(path)
}

func openExecNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW, 0) //nolint:gosec // G304: path is a trusted CE descriptor
}

func identFromInfo(info os.FileInfo) fileIdent {
	id := fileIdent{size: info.Size(), mtime: info.ModTime().UnixNano()}
	if st, ok := info.Sys().(*unix.Stat_t); ok {
		return identFromStat(*st)
	}
	return id
}

func identFromFile(f *os.File) (fileIdent, error) {
	var st unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		return fileIdent{}, err
	}
	return identFromStat(st), nil
}
