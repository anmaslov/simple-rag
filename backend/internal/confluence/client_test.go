package confluence

import "testing"

func TestParsePageID(t *testing.T) {
	for input, want := range map[string]string{
		"12345": "12345",
		"https://wiki.local/pages/viewpage.action?pageId=12345": "12345",
		"https://wiki.local/spaces/HR/pages/12345/Title":        "12345",
	} {
		got, err := ParsePageID(input)
		if err != nil || got != want {
			t.Fatalf("ParsePageID(%q)=%q,%v want %q", input, got, err, want)
		}
	}
}
