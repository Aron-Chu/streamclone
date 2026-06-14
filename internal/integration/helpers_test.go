package integration_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func repoRootFromFile(file string, ok bool) string {
	if ok {
		return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	}
	wd, _ := os.Getwd()
	return wd
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(1)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return repoRootFromFile(file, ok)
}

func migrationsDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "migrations")
}
