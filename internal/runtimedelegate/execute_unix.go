//go:build !windows

package runtimedelegate

import "syscall"

func executeDelegate(file string, args, env []string, _ string) (int, error) {
	return 0, syscall.Exec(file, append([]string{file}, args...), env)
}
