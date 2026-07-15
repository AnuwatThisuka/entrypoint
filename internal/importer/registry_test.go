package importer

import (
	"sort"
	"testing"
)

func TestDefaultRegistersBothImporters(t *testing.T) {
	names := Default().Names()
	sort.Strings(names)
	want := []string{"entire", "entrypoint"}
	if len(names) != len(want) || names[0] != want[0] || names[1] != want[1] {
		t.Fatalf("registered importers = %v, want %v", names, want)
	}
}
