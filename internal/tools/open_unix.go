//go:build unix

package tools

import "syscall"

const openNoFollowFlag = syscall.O_NOFOLLOW
