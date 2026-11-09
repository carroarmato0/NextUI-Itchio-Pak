//go:build unix && !linux

package partfile

import "syscall"

// dupOnto points newfd at oldfd's file.
func dupOnto(oldfd, newfd int) error { return syscall.Dup2(oldfd, newfd) }
