package runtime

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// Docker drives tasks through the Docker Engine API.
type Docker struct {
	cli *client.Client
}

// Compile-time check that Docker satisfies Runtime.
var _ Runtime = (*Docker)(nil)

// NewDocker connects to the local Docker daemon and pings it so
// failures surface early with a clear message.
func NewDocker(ctx context.Context) (*Docker, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	if _, err := cli.Ping(ctx); err != nil {
		cli.Close()
		return nil, fmt.Errorf("docker daemon unavailable: %w", err)
	}
	return &Docker{cli: cli}, nil
}

// Close implements Runtime.
func (d *Docker) Close() error { return d.cli.Close() }

// Start implements Runtime. Images must provide a POSIX shell: the
// container is booted as `sh -c 'sleep ...'` and every interaction
// happens through execs.
func (d *Docker) Start(ctx context.Context, img string) (string, error) {
	if err := d.ensureImage(ctx, img); err != nil {
		return "", err
	}
	resp, err := d.cli.ContainerCreate(ctx,
		&container.Config{
			Image:      img,
			Entrypoint: []string{"sh", "-c"},
			Cmd:        []string{"sleep 86400"},
			WorkingDir: "/task",
		},
		&container.HostConfig{},
		nil, nil, "")
	if err != nil {
		return "", fmt.Errorf("create container from %s: %w", img, err)
	}
	if err := d.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		d.remove(context.WithoutCancel(ctx), resp.ID)
		return "", fmt.Errorf("start container from %s: %w", img, err)
	}
	insp, err := d.cli.ContainerInspect(ctx, resp.ID)
	if err != nil {
		d.remove(context.WithoutCancel(ctx), resp.ID)
		return "", fmt.Errorf("inspect container: %w", err)
	}
	if !insp.State.Running {
		d.remove(context.WithoutCancel(ctx), resp.ID)
		return "", fmt.Errorf("container from %s exited immediately: an image with a POSIX shell is required", img)
	}
	return resp.ID, nil
}

// CopyDir implements Runtime.
func (d *Docker) CopyDir(ctx context.Context, id, src, dst string) error {
	buf, err := tarDirectory(src)
	if err != nil {
		return fmt.Errorf("pack %s: %w", src, err)
	}
	if err := d.cli.CopyToContainer(ctx, id, dst, buf, container.CopyToContainerOptions{}); err != nil {
		return fmt.Errorf("copy %s to %s: %w", src, dst, err)
	}
	return nil
}

// Exec implements Runtime.
func (d *Docker) Exec(ctx context.Context, id, cmd string) (ExecResult, error) {
	start := time.Now()
	create, err := d.cli.ContainerExecCreate(ctx, id, container.ExecOptions{
		Cmd:          []string{"sh", "-c", cmd},
		WorkingDir:   "/task",
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return ExecResult{}, fmt.Errorf("exec create: %w", err)
	}
	attach, err := d.cli.ContainerExecAttach(ctx, create.ID, container.ExecStartOptions{})
	if err != nil {
		return ExecResult{}, fmt.Errorf("exec attach: %w", err)
	}
	defer attach.Close()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, attach.Reader); err != nil {
		return ExecResult{}, fmt.Errorf("exec output: %w", err)
	}
	insp, err := d.cli.ContainerExecInspect(ctx, create.ID)
	if err != nil {
		return ExecResult{}, fmt.Errorf("exec inspect: %w", err)
	}
	return ExecResult{
		Output:   stdout.String() + stderr.String(),
		ExitCode: insp.ExitCode,
		Duration: time.Since(start),
	}, nil
}

// Stop implements Runtime.
func (d *Docker) Stop(ctx context.Context, id string) error {
	return d.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true, RemoveVolumes: true})
}

func (d *Docker) remove(ctx context.Context, id string) {
	_ = d.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true, RemoveVolumes: true})
}

// ensureImage uses the local image when present and pulls otherwise.
func (d *Docker) ensureImage(ctx context.Context, ref string) error {
	if _, _, err := d.cli.ImageInspectWithRaw(ctx, ref); err == nil {
		return nil
	}
	rc, err := d.cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull %s: %w", ref, err)
	}
	defer rc.Close()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("pull %s: %w", ref, err)
	}
	return nil
}

// tarDirectory packs src as a tar stream with slash-separated relative
// paths, ready for CopyToContainer.
func tarDirectory(src string) (*bytes.Buffer, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if info.IsDir() {
			hdr.Name += "/"
		} else if !info.Mode().IsRegular() {
			return nil // skip symlinks and specials
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return &buf, nil
}
