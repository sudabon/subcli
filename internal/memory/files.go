package memory

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// openDirBelow pins each directory before descending into it. Repository-local
// paths may originate in a checkout, including each ancestor of a projection.
func openDirBelow(base, dir string, create bool) (*os.Root, error) {
	rel, err := filepath.Rel(base, dir)
	if err != nil || !filepath.IsLocal(rel) {
		return nil, fmt.Errorf("ディレクトリ %s は %s の配下にありません", dir, base)
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		return nil, err
	}
	if rel == "." {
		return root, nil
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		next, err := openChildDir(root, part, create)
		root.Close()
		if err != nil {
			return nil, err
		}
		root = next
	}
	return root, nil
}

func openChildDir(parent *os.Root, name string, create bool) (*os.Root, error) {
	if create {
		if err := parent.Mkdir(name, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
	}
	info, err := parent.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("通常のディレクトリではありません: %s", filepath.Join(parent.Name(), name))
	}
	child, err := parent.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	current, err := child.Stat(".")
	if err != nil || !os.SameFile(info, current) {
		child.Close()
		return nil, fmt.Errorf("ディレクトリが変更されました: %s", filepath.Join(parent.Name(), name))
	}
	if create && current.Mode().Perm() != 0o700 {
		if err := child.Chmod(".", 0o700); err != nil {
			child.Close()
			return nil, err
		}
	}
	return child, nil
}

func regularFile(dir *os.Root, name string) (fs.FileInfo, error) {
	info, err := dir.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("通常のファイルではありません: %s", filepath.Join(dir.Name(), name))
	}
	return info, nil
}

func (s *Store) statFile(path string) (fs.FileInfo, error) {
	dir, err := openDirBelow(s.RepoRoot, filepath.Dir(path), false)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	return regularFile(dir, filepath.Base(path))
}

func readFileBelow(base, path string) ([]byte, error) {
	dir, err := openDirBelow(base, filepath.Dir(path), false)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	name := filepath.Base(path)
	info, err := regularFile(dir, name)
	if err != nil {
		return nil, err
	}
	f, err := dir.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	current, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(info, current) {
		return nil, fmt.Errorf("ファイルが変更されました: %s", path)
	}
	return io.ReadAll(f)
}

func (s *Store) writePrivateFile(path string, data []byte) error {
	return s.replaceFile(path, data, 0o600)
}

func (s *Store) replaceFile(path string, data []byte, mode fs.FileMode) error {
	dir, err := openDirBelow(s.RepoRoot, filepath.Dir(path), true)
	if err != nil {
		return err
	}
	defer dir.Close()
	name := filepath.Base(path)
	if _, err := regularFile(dir, name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	tmp := ".subcli-tmp-" + rand.Text()
	f, err := dir.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer dir.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if mode != 0o600 {
		if err := f.Chmod(mode); err != nil {
			f.Close()
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return dir.Rename(tmp, name)
}
