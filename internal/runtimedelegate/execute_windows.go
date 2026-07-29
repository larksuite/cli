//go:build windows

package runtimedelegate

import (
	"errors"
	"os"
	"os/exec"
)

// Windows has no execve equivalent. Running one attached child with inherited
// standard handles and returning its exact exit code is the closest portable
// process behavior available to the CLI entry point.
func executeDelegate(file string, args, env []string, cwd string) (int, error) {
	child := exec.Command(file, args...)
	child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
	child.Env, child.Dir = env, cwd
	if err := child.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return 1, err
	}
	return 0, nil
}
