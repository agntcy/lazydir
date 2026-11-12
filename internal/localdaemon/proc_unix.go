// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package localdaemon

import (
	"os"
	"syscall"
)

func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

func killGroup(p *os.Process) error {
	return syscall.Kill(-p.Pid, syscall.SIGKILL)
}

// terminate asks p to shut down gracefully.
func terminate(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}
