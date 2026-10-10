package demo_test

import (
	"os"
	"os/exec"
	"testing"
)

// TestGenerated checks demo_vuka.go is what the compiler makes of demo.vuka.
func TestGenerated(t *testing.T) {
	vuka := os.Getenv("VUKA")
	if vuka == "" {
		var err error
		if vuka, err = exec.LookPath("vuka"); err != nil {
			t.Skip("needs the vuka command: on PATH, or $VUKA")
		}
	}
	cmd := exec.Command(vuka, "gen", "-inplace", "-check", ".")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("demo_vuka.go is stale (vuka gen -inplace ./live/internal/demo):\n%s%v", out, err)
	}
}
