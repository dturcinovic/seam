package git

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// TestSessionsThatStartWhileOthersEndAllOpen runs sessions of one
// repository back to back in two places, each ending as Close does. One
// that ends removes the repository's directory under the other as it
// starts.
func TestSessionsThatStartWhileOthersEndAllOpen(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	c := &Checkouts{base: filepath.Join(t.TempDir(), "repo"), log: zap.New(core)}
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			for range 500 {
				s, err := openSession(c.base, "root")
				if !assert.NoError(t, err) {
					return
				}
				// With no checkout in it, no git runs.
				c.removeSession(t.Context(), s.dir)
				assert.NoError(t, s.unlock())
				c.removeBase()
			}
		})
	}
	wg.Wait()
	assert.Empty(t, logs.All())
	assert.NoDirExists(t, c.base)
}
