//go:build windows

package cmd

import "os/exec"

func setSysProcAttr(_ *exec.Cmd) {
	// No Setsid on Windows — process is already detached via Start()
}
