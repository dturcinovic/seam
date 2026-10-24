package git

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/change"
	"seam/internal/domain/limits"
	"seam/internal/testsupport"
)

// repoWithBlob opens a repository that holds the blob id with "content\n".
func repoWithBlob(t *testing.T) (*Repo, change.ObjectID) {
	t.Helper()
	git := testsupport.NewGitRepo(t)
	git.Write("f", "content\n")
	id := mustID(t, strings.TrimSpace(git.Git("hash-object", "-w", "f")))
	repo, err := Open(git.Dir, nil)
	require.NoError(t, err)
	return repo, id
}

func TestCloseEndsCatFile(t *testing.T) {
	repo, id := repoWithBlob(t)
	_, err := repo.ReadBlob(t.Context(), id, limits.DefaultFileSize)
	require.NoError(t, err)
	cat := repo.blobs

	require.NoError(t, repo.Close(t.Context()))

	assert.NotNil(t, cat.cmd.ProcessState, "the process has been waited for")
	_, err = repo.ReadBlob(t.Context(), id, limits.DefaultFileSize)
	assert.ErrorIs(t, err, ErrClosed, "and no read starts another")
	assert.Nil(t, repo.blobs)
}

func TestAReadWhoseContextEndsWhileItWaitsKeepsCatFile(t *testing.T) {
	repo, id := repoWithBlob(t)
	t.Cleanup(func() { require.NoError(t, repo.Close(context.Background())) })
	_, err := repo.ReadBlob(t.Context(), id, limits.DefaultFileSize)
	require.NoError(t, err)
	cat := repo.blobs
	ctx, cancel := context.WithCancel(t.Context())

	repo.blobMu.Lock()
	done := readBlob(ctx, repo, id)
	cancel()
	repo.blobMu.Unlock()

	require.ErrorIs(t, testsupport.Await(t, done), context.Canceled)
	assert.Same(t, cat, repo.blobs, "the cat-file in step is kept")
}

// stalledRepo is a repository whose cat-file takes a request and never
// answers, as one fetching a blob of a partial clone from a slow remote:
// the fetch is a child that holds cat-file's stdout. asked exists once the
// request has reached it.
func stalledRepo(t *testing.T) (repo *Repo, id change.ObjectID, stalled *catFile, asked string) {
	t.Helper()
	repo, id = repoWithBlob(t)
	asked = filepath.Join(t.TempDir(), "asked")
	cmd := exec.CommandContext(t.Context(), "sh", "-c", `read -r line && : > "$1" && { sleep 60 & wait; }`, "sh", asked)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	in, err := cmd.StdinPipe()
	require.NoError(t, err)
	out, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	stalled = &catFile{cmd: cmd, in: in, out: bufio.NewReader(out), pipe: out}
	repo.blobs = stalled
	return repo, id, stalled, asked
}

// readBlob reads id in the background.
func readBlob(ctx context.Context, repo *Repo, id change.ObjectID) <-chan error {
	done := make(chan error, 1)
	go func() {
		_, err := repo.ReadBlob(ctx, id, limits.DefaultFileSize)
		done <- err
	}()
	return done
}

// awaitAsked waits until the read has reached the stalled cat-file.
func awaitAsked(t *testing.T, asked string) {
	t.Helper()
	require.Eventually(t, func() bool {
		_, err := os.Stat(asked)
		return err == nil
	}, testsupport.Guard, 5*time.Millisecond, "the read reaches cat-file")
}

func TestAStalledReadEndsWithItsContext(t *testing.T) {
	repo, id, stalled, asked := stalledRepo(t)
	t.Cleanup(func() { require.NoError(t, repo.Close(context.Background())) })
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := readBlob(ctx, repo, id)
	awaitAsked(t, asked)

	cancel()

	require.ErrorIs(t, testsupport.Await(t, done), context.Canceled)
	assert.NotNil(t, stalled.cmd.ProcessState, "the stalled process is killed and waited for")
	data, err := repo.ReadBlob(t.Context(), id, limits.DefaultFileSize)
	require.NoError(t, err, "the next read starts a cat-file of its own")
	assert.Equal(t, "content\n", string(data))
}

func TestCloseEndsAStalledRead(t *testing.T) {
	repo, id, stalled, asked := stalledRepo(t)
	done := readBlob(t.Context(), repo, id)
	awaitAsked(t, asked)

	closed := make(chan error, 1)
	go func() { closed <- repo.Close(t.Context()) }()

	require.ErrorIs(t, testsupport.Await(t, done), ErrClosed)
	require.NoError(t, testsupport.Await(t, closed), "Close does not wait for cat-file to answer")
	assert.NotNil(t, stalled.cmd.ProcessState)
}
