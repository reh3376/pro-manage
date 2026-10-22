package secrets

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
)

// File reads KEY=VALUE pairs from a dotenv-style file once at construction.
// Lines starting with # and blank lines are ignored; surrounding single or
// double quotes on the value are stripped; no variable expansion is done.
type File struct {
	path   string
	values map[string]string
}

// NewFile loads the file eagerly so a missing or unreadable file fails at
// startup rather than on first use.
func NewFile(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("secrets: open %s: %w", path, err)
	}
	defer f.Close()

	values := make(map[string]string)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if len(v) >= 2 {
			if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
				v = v[1 : len(v)-1]
			}
		}
		values[k] = v
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("secrets: read %s: %w", path, err)
	}
	return &File{path: path, values: values}, nil
}

func (f *File) Get(_ context.Context, name string) (string, error) {
	if v, ok := f.values[name]; ok && v != "" {
		return v, nil
	}
	return "", ErrNotFound
}

func (f *File) Name() string { return "file(" + f.path + ")" }
