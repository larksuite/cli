//go:build !windows

package runtimedelegate

import (
	"io/fs"
	"os"
	"syscall"
)

func ownedByCurrentUser(_ string, info fs.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Getuid())
}
