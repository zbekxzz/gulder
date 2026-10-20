package httpapi

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §13, DoD: тексты, которые уходят пользователю, не обещают безопасности
// («известных противопоказаний не найдено», а не «безопасно»/«гарантированно»).
func TestNoSafetyPromisesInMessages(t *testing.T) {
	forbidden := regexp.MustCompile(`(?i)"[^"\n]*(безопасн|гарантир)[^"\n]*"`)
	var offenders []string
	err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		src, err := os.ReadFile(path) //nolint:gosec // обход исходников в тесте
		if err != nil {
			return err
		}
		if m := forbidden.FindString(string(src)); m != "" {
			offenders = append(offenders, path+": "+m)
		}
		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, offenders)
}
