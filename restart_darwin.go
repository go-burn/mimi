package main

import "syscall"

func restartProcessAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
