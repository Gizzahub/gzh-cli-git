// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

package contextref

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

var windowsReserved = map[string]struct{}{
	"CON": {}, "PRN": {}, "AUX": {}, "NUL": {},
	"COM1": {}, "COM2": {}, "COM3": {}, "COM4": {}, "COM5": {},
	"COM6": {}, "COM7": {}, "COM8": {}, "COM9": {},
	"LPT1": {}, "LPT2": {}, "LPT3": {}, "LPT4": {}, "LPT5": {},
	"LPT6": {}, "LPT7": {}, "LPT8": {}, "LPT9": {},
}

func validateEntrypointPath(p string) error {
	if p == "" || !utf8.ValidString(p) {
		return errPath
	}
	if strings.HasPrefix(p, "/") || strings.HasSuffix(p, "/") {
		return errPath
	}
	if strings.ContainsAny(p, "\\\x00") {
		return errPath
	}
	for _, r := range p {
		if r < 0x20 {
			return errPath
		}
	}
	if looksLikeDriveOrUNC(p) {
		return errPath
	}
	parts := strings.Split(p, "/")
	if len(parts) == 0 {
		return errPath
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return errPath
		}
		if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return errPath
		}
		stem := part
		if i := strings.IndexByte(part, '.'); i >= 0 {
			stem = part[:i]
		}
		if _, reserved := windowsReserved[strings.ToUpper(stem)]; reserved {
			return errPath
		}
		for _, r := range part {
			if unicode.Is(unicode.C, r) {
				return errPath
			}
		}
	}
	return nil
}

func looksLikeDriveOrUNC(p string) bool {
	if len(p) >= 2 && p[1] == ':' {
		return true
	}
	return strings.HasPrefix(p, "//")
}
