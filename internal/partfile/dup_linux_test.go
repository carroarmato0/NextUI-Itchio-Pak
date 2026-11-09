package partfile

import "syscall"

// dupOnto points newfd at oldfd's file. linux/arm64 has no dup2 syscall, so
// Linux goes through dup3, which every Linux architecture provides.
func dupOnto(oldfd, newfd int) error { return syscall.Dup3(oldfd, newfd, 0) }
