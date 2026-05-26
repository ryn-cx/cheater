package placeholder

import (
	"reflect"
	"testing"
)

func TestExtract_DedupesAndPreservesOrder(t *testing.T) {
	got := Extract("ffmpeg -i <input> -c:v <codec> <output> <input>")
	want := []string{"input", "codec", "output"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExtract_NoPlaceholders(t *testing.T) {
	if got := Extract("ls -la"); len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestExtract_IgnoresShellRedirectsAndComparisons(t *testing.T) {
	for _, input := range []string{
		"cmd > /dev/null",
		"cmd 2>&1",
		"cat <<EOF",
		"cmp <(diff a b) <(diff c d)",
		"test 1 < 2",
	} {
		if got := Extract(input); len(got) != 0 {
			t.Errorf("input %q: expected empty, got %v", input, got)
		}
	}
}

func TestAssemble_SubstitutesValues(t *testing.T) {
	values := map[string]string{"input": "a.mp4", "codec": "libx264", "output": "b.mp4"}
	got := Assemble("ffmpeg -i <input> -c:v <codec> <output>", values)
	want := "ffmpeg -i a.mp4 -c:v libx264 b.mp4"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAssemble_PassesThroughWithoutPlaceholders(t *testing.T) {
	if got := Assemble("ls -la", nil); got != "ls -la" {
		t.Errorf("got %q, want %q", got, "ls -la")
	}
}

func TestAssemble_MissingValueExpandsToEmpty(t *testing.T) {
	if got := Assemble("echo <x>", map[string]string{}); got != "echo " {
		t.Errorf("got %q, want %q", got, "echo ")
	}
}
