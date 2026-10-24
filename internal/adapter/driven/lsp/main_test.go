package lsp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"go.uber.org/goleak"
	"go.uber.org/zap"

	"seam/internal/domain/hover"
	"seam/internal/domain/lang"
)

// fakelspEnv names the test server's path, built once for the package
// and handed to the tests through the environment rather than a package
// variable.
const fakelspEnv = "SEAM_TEST_FAKELSP"

func fakelsp() string { return os.Getenv(fakelspEnv) }

func TestMain(m *testing.M) {
	if os.Getenv(helperEnv) != "" {
		os.Exit(helper(os.Getenv(helperEnv)))
	}
	dir, err := os.MkdirTemp("", "fakelsp")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	path := filepath.Join(dir, "fakelsp")
	if out, err := exec.CommandContext(context.Background(), "go", "build", "-o", path, "./cmd/fakelsp").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build fakelsp: %v\n%s", err, out)
		os.Exit(1)
	}
	if err := os.Setenv(fakelspEnv, path); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	goleak.VerifyTestMain(m, goleak.Cleanup(func(int) { _ = os.RemoveAll(dir) }))
}

// helperEnv makes the test binary a stand-in for seam: it starts fakelsp
// in the directory the variable names, prints the server's pid and waits
// to be killed.
const helperEnv = "SEAM_LSP_HELPER"

func helper(root string) int {
	table, err := lang.NewTable([]lang.Language{{Lang: lang.Go, Extensions: []string{".go"}, Server: "fake"}},
		[]lang.Server{{Name: "fake", Command: fakelsp()}})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	pool := newPool(Config{Root: root, LogDir: root}, table, newProcesses(root), zap.NewNop(), nil)
	pool.Sync(hover.Document{Path: "a.go", Lang: lang.Go, Version: 1, Text: "package a\n"})
	for {
		<-pool.Signal()
		for _, u := range pool.Drain() {
			if u.Kind == hover.StatusChanged && u.Status == hover.Ready {
				c := pool.instances[lang.Instance{Server: "fake", Root: "."}].client
				c.mu.Lock()
				fmt.Println(c.conn.pid)
				c.mu.Unlock()
			}
		}
	}
}
