// Package runtime abstracts the container engine used to evaluate tasks.
package runtime

import (
	"context"
	"time"
)

// ExecResult is the outcome of a command executed inside a container.
type ExecResult struct {
	Output   string
	ExitCode int
	Duration time.Duration
}

// Runtime manages the lifecycle of task containers.
type Runtime interface {
	// Start creates and boots a container from image with /task as the
	// working directory, returning its id.
	Start(ctx context.Context, image string) (string, error)
	// CopyDir copies the directory at src into the container at dst.
	CopyDir(ctx context.Context, id, src, dst string) error
	// Exec runs cmd with the container shell. A non-zero exit status is
	// reported in ExecResult.ExitCode; error is reserved for
	// infrastructure failures (daemon gone, ctx cancelled, ...).
	Exec(ctx context.Context, id, cmd string) (ExecResult, error)
	// Stop kills and removes the container.
	Stop(ctx context.Context, id string) error
	// Close releases runtime-level resources.
	Close() error
}
