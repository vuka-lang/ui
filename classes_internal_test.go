package ui

import "testing"

// BenchmarkMergeUncached merges without the cache: a list's first merge.
func BenchmarkMergeUncached(b *testing.B) {
	s := "flex items-start gap-2 rounded-lg border px-4 py-3 text-sm border-border bg-gray-50 hover:bg-gray-100 dark:bg-gray-900 px-2 bg-white md:px-6"
	b.ReportAllocs()
	for b.Loop() {
		mergeClasses(s)
	}
}

// Every group a utility can have is numbered: one that isn't would merge
// by name, as an arbitrary property does.
func TestClassGroupsNumbered(t *testing.T) {
	values := []string{"", "0", "2", "px", "sm", "lg", "2xl", "none", "auto", "full", "red-500", "red-500/50", "[3px]", "[#fff]",
		"left", "center", "wrap", "ellipsis", "bold", "sans", "fixed", "cover", "repeat", "dashed", "wavy", "inside", "contain",
		"row", "col", "nowrap", "1", "between", "[url(x.png)]", "50%", "x", "y", "t-2", "x-red-500"}
	for p := range classPrefixes {
		for _, v := range values {
			c := p
			if v != "" {
				c += "-" + v
			}
			if g := utilityGroup(c); g != "" && g[0] != '[' {
				if _, ok := groupIDs[g]; !ok {
					t.Errorf("%s: group %s has no number", c, g)
				}
			}
		}
	}
}
