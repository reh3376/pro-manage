package secrets

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFileProviderParsesDotenv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# comment\n\nDATABASE_URL=postgres://pm:pw@localhost/pm_core\nexport MDEMG_API_KEY='k-1'\nQUOTED=\"a=b\"\nEMPTY=\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	fp, err := NewFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	cases := map[string]string{
		"DATABASE_URL":  "postgres://pm:pw@localhost/pm_core",
		"MDEMG_API_KEY": "k-1",
		"QUOTED":        "a=b",
	}
	for k, want := range cases {
		got, err := fp.Get(ctx, k)
		if err != nil || got != want {
			t.Errorf("%s: got %q, %v; want %q", k, got, err, want)
		}
	}
	if _, err := fp.Get(ctx, "EMPTY"); !errors.Is(err, ErrNotFound) {
		t.Errorf("empty value should be not found, got %v", err)
	}
	if _, err := fp.Get(ctx, "MISSING"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing key should be not found, got %v", err)
	}
}

func TestChainFallsThroughToEnv(t *testing.T) {
	t.Setenv("PM_TEST_SECRET", "from-env")
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("OTHER=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fp, err := NewFile(path)
	if err != nil {
		t.Fatal(err)
	}
	chain := Chain{fp, Env{}}
	got, err := chain.Get(context.Background(), "PM_TEST_SECRET")
	if err != nil || got != "from-env" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := chain.Get(context.Background(), "NOPE"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestNewFileMissingFails(t *testing.T) {
	if _, err := NewFile(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("expected error for missing file")
	}
}
