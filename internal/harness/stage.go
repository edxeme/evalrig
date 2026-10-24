package harness

import (
	"io"
	"os"
	"path/filepath"
)

// copyInto copies the file or directory at src into dstParent,
// creating dstParent first.
func copyInto(dstParent, src string, isDir bool) error {
	if err := os.MkdirAll(dstParent, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(dstParent, filepath.Base(src))
	if !isDir {
		return copyFile(dst, src)
	}
	return copyTree(dst, src)
}

// copyTree copies the directory tree at src to dst.
func copyTree(dst, src string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyFile(target, p)
	})
}

func copyFile(dst, src string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
