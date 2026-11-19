package frontmatter

import "testing"

func TestParseAcceptsLFAndCRLF(t *testing.T) {
	for _, tc := range []struct {
		name, content string
	}{
		{"LF", "---\nlayout: kanban\ntags:\n  - a\n---\n\n# Title\n"},
		{"CRLF", "---\r\nlayout: kanban\r\ntags:\r\n  - a\r\n---\r\n\r\n# Title\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !HasFrontmatter(tc.content) {
				t.Fatal("HasFrontmatter = false, want true")
			}
			meta, body, ok := Parse(tc.content)
			if !ok || meta.Layout != "kanban" {
				t.Fatalf("Parse = %+v, %v, want layout kanban", meta, ok)
			}
			if body != "# Title\n" && body != "# Title\r\n" {
				t.Errorf("body = %q, want the heading line", body)
			}
			if got := Extract(tc.content); got != "layout: kanban\ntags:\n  - a" && got != "layout: kanban\r\ntags:\r\n  - a" {
				t.Errorf("Extract = %q", got)
			}
		})
	}
}

func TestParseWithoutFrontmatter(t *testing.T) {
	for _, content := range []string{"# Title\n", "---", "---\nno closing line\n", "--- \nx\n---\n"} {
		if _, body, ok := Parse(content); ok || body != content {
			t.Errorf("Parse(%q) = %q, %v, want the content unchanged and false", content, body, ok)
		}
	}
}
