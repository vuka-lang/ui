package ui

import (
	"reflect"
	"testing"
)

// TestLiveMethods: liveMethods, written out, are *Live's exported methods.
func TestLiveMethods(t *testing.T) {
	pt := reflect.TypeFor[*Live]()
	want := map[string]bool{}
	for i := range pt.NumMethod() {
		want[pt.Method(i).Name] = true
	}
	if !reflect.DeepEqual(liveMethods, want) {
		t.Errorf("liveMethods = %v; *Live's methods are %v", liveMethods, want)
	}
}
