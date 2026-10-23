package typst

import (
	"os/exec"
	"sync"
	"testing"

	"github.com/flexprice/flexprice/internal/config"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/stretchr/testify/require"
)

func TestCompileUsesUniqueOutputFiles(t *testing.T) {
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Skip("true binary not available")
	}
	log, err := logger.NewLogger(config.GetDefaultConfig())
	require.NoError(t, err)

	c := NewCompiler(log, truePath, "", "", t.TempDir())

	const renders = 50
	paths := make([]string, renders)
	var wg sync.WaitGroup
	for i := range paths {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, cerr := c.Compile(CompileOpts{InputFile: "in.typ"})
			if cerr == nil {
				paths[i] = p
			}
		}()
	}
	wg.Wait()

	seen := make(map[string]struct{}, renders)
	for _, p := range paths {
		require.NotEmpty(t, p)
		_, dup := seen[p]
		require.False(t, dup, "duplicate output path %s", p)
		seen[p] = struct{}{}
	}
}
