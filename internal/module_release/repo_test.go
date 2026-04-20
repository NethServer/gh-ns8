package module_release

import (
	"reflect"
	"strings"
	"testing"
)

func TestGetLinkedIssuesPreservesBodyOrderAcrossFormats(t *testing.T) {
	body := strings.Join([]string{
		"Refs NethServer/dev#42",
		"https://github.com/NethServer/dev/issues/7",
		"NethServer/issues/9",
	}, " then ")

	got := GetLinkedIssues(body, "NethServer/dev")
	want := []int{42, 7, 9}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetLinkedIssues() = %v, want %v", got, want)
	}
}
