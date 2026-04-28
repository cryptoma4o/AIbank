// Package validate wraps packages/tenant-config-schema/validate.py via
// os/exec so the CLI uses the same JSON Schema validation as CI.
//
// We intentionally do not re-implement validation in Go — keeping a single
// validator avoids drift between CI and developer machines.
package validate

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Result captures the outcome of a single validation run.
type Result struct {
	Path     string
	Stdout   string
	Stderr   string
	ExitCode int
}

// Ok reports whether the run produced exit code 0 (all files validated).
func (r Result) Ok() bool { return r.ExitCode == 0 }

// FindValidator walks up from `start` looking for
// `packages/tenant-config-schema/validate.py`.  The search stops at the
// filesystem root.  Returns an absolute path or ErrValidatorNotFound.
func FindValidator(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, "packages", "tenant-config-schema", "validate.py")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrValidatorNotFound
		}
		dir = parent
	}
}

// ErrValidatorNotFound is returned when the validate.py script cannot be
// found by walking up the directory tree.
var ErrValidatorNotFound = errors.New("validate.py not found in any ancestor (looked for packages/tenant-config-schema/validate.py)")

// Run executes validate.py against tenantPath. python is the executable
// to invoke (default "python3" if empty).
func Run(python, validatorPath, tenantPath string, verbose bool) (Result, error) {
	if python == "" {
		python = "python3"
	}
	args := []string{validatorPath, tenantPath}
	if verbose {
		args = append(args, "--verbose")
	}
	cmd := exec.Command(python, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := Result{
		Path:   tenantPath,
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
	if exit, ok := err.(*exec.ExitError); ok {
		res.ExitCode = exit.ExitCode()
		// Non-zero exit is a validation result, not a CLI failure.
		return res, nil
	}
	if err != nil {
		return res, fmt.Errorf("execute %s: %w", python, err)
	}
	res.ExitCode = 0
	return res, nil
}
