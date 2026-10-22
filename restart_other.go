//go:build !darwin

package main

import "syscall"

func restartProcessAttributes() *syscall.SysProcAttr {
	return nil
}
