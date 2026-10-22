package demo_test

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// minVuka is the first vuka whose JSX lowers to github.com/vuka-lang/ui/v2.
var minVuka = [3]int{0, 10, 0}

// TestGenerated checks demo_vuka.go is what the compiler makes of demo.vuka,
// with the vuka command from $VUKA or PATH.
func TestGenerated(t *testing.T) {
	vuka := os.Getenv("VUKA")
	if vuka == "" {
		var err error
		if vuka, err = exec.LookPath("vuka"); err != nil {
			needs(t, "needs the vuka command: on PATH, or $VUKA")
		}
	}
	out, _ := exec.Command(vuka, "version").Output()
	if v := version(string(out)); v[0] < minVuka[0] || v[0] == minVuka[0] && v[1] < minVuka[1] {
		needs(t, "%s is %s; the demo needs vuka v%d.%d or later ($VUKA)", vuka, strings.TrimSpace(string(out)), minVuka[0], minVuka[1])
	}
	cmd := exec.Command(vuka, "gen", "-inplace", "-check", ".")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("demo_vuka.go is stale (vuka gen -inplace ./live/internal/demo):\n%s%v", out, err)
	}
}

// version reads "vuka v0.10.0" as {0, 10, 0}.
func version(s string) [3]int {
	var v [3]int
	f := strings.Fields(s)
	if len(f) < 2 {
		return v
	}
	for i, p := range strings.SplitN(strings.TrimPrefix(f[1], "v"), ".", 3) {
		v[i], _ = strconv.Atoi(p)
	}
	return v
}

// needs skips the test without the vuka command; under CI (CI set) it fails
// instead, so a missing command never passes as a skip.
func needs(t *testing.T, format string, args ...any) {
	t.Helper()
	if os.Getenv("CI") != "" {
		t.Fatalf(format+" (CI is set, so this fails instead of skipping)", args...)
	}
	t.Skipf(format, args...)
}
