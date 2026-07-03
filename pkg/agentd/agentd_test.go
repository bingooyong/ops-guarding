package agentd

import (
	"path/filepath"
	"testing"
)

func TestDefaultConfigFile(t *testing.T) {
	want := filepath.Join("etc", "service", "agentd.conf")
	if got := DefaultConfigFile(); got != want {
		t.Fatalf("DefaultConfigFile() = %q, want %q", got, want)
	}
}
