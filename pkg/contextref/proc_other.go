// Copyright (c) 2026 Gizzahub
// SPDX-License-Identifier: MIT

//go:build !unix

package contextref

import "os/exec"

func setProcGroup(*exec.Cmd) {}

func killProcTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
