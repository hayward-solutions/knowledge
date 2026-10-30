package content

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var mdUpdate = flag.Bool("update", false, "rewrite golden files")

func mdGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *mdUpdate {
		if err := os.WriteFile(path, []byte(got+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got+"\n" != string(want) {
		t.Fatalf("%s mismatch\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func mdFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestToMarkdownGoldenBlocks(t *testing.T) {
	got, err := ToMarkdown(mdFixture(t, "blocks.json"))
	if err != nil {
		t.Fatal(err)
	}
	mdGolden(t, "blocks.md", got)
}

func TestToMarkdownGoldenCustomNodes(t *testing.T) {
	got, err := ToMarkdown(mdFixture(t, "custom.json"))
	if err != nil {
		t.Fatal(err)
	}
	mdGolden(t, "custom.md", got)
}

func TestToMarkdownFromHTMLGolden(t *testing.T) {
	got, err := ToMarkdownFromHTML(string(mdFixture(t, "contract.html")))
	if err != nil {
		t.Fatal(err)
	}
	mdGolden(t, "contract.md", got)
}

func TestToMarkdownEmptyAndInvalid(t *testing.T) {
	for _, in := range []string{"", "null", "  ", `{"type":"doc"}`, `{"type":"doc","content":[{"type":"paragraph"}]}`} {
		got, err := ToMarkdown([]byte(in))
		if err != nil || got != "" {
			t.Fatalf("ToMarkdown(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := ToMarkdown([]byte(`{"type":`)); err == nil {
		t.Fatal("expected error for invalid json")
	}
	if got, err := ToMarkdownFromHTML(""); err != nil || got != "" {
		t.Fatalf("empty html: %q %v", got, err)
	}
}

func TestToMarkdownInlineCases(t *testing.T) {
	cases := []struct{ name, json, want string }{
		{
			"whitespace moves outside emphasis",
			`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"a "},{"type":"text","text":"bold ","marks":[{"type":"bold"}]},{"type":"text","text":"b"}]}]}`,
			"a **bold** b",
		},
		{
			"adjacent runs with same marks merge",
			`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"ab","marks":[{"type":"bold"}]},{"type":"text","text":"cd","marks":[{"type":"bold"},{"type":"comment","attrs":{"id":"x"}}]}]}]}`,
			"**abcd**",
		},
		{
			"code span with backticks",
			`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"a` + "`" + `b","marks":[{"type":"code"}]}]}]}`,
			"``a`b``",
		},
		{
			"heading hard break becomes space",
			`{"type":"doc","content":[{"type":"heading","attrs":{"level":3},"content":[{"type":"text","text":"a"},{"type":"hardBreak"},{"type":"text","text":"b"}]}]}`,
			"### a b",
		},
		{
			"list-like text is escaped",
			`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"- item"}]},{"type":"paragraph","content":[{"type":"text","text":"# not heading"}]}]}`,
			"\\- item\n\n\\# not heading",
		},
		{
			"table cell flattens hard break",
			`{"type":"doc","content":[{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"a"},{"type":"hardBreak"},{"type":"text","text":"b"}]}]}]}]}]}`,
			"| a<br>b |\n| --- |",
		},
		{
			"link without href is plain",
			`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":""}}]}]}]}`,
			"x",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ToMarkdown([]byte(c.json))
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestToStandaloneHTML(t *testing.T) {
	doc := ToStandaloneHTML(`A <b> & "c"`, `<p>hi</p><script>alert(1)</script><div data-type="panel" data-color="info"><p>x</p></div>`)
	for _, want := range []string{
		"<!doctype html>", "<title>A &lt;b&gt; &amp; &#34;c&#34;</title>", "<p>hi</p>",
		`data-type="panel"`, "<style>", "data-type=taskList", "data-type=status",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("missing %q in\n%s", want, doc)
		}
	}
	if strings.Contains(doc, "<script") || strings.Contains(doc, "alert(1)") {
		t.Fatalf("script survived:\n%s", doc)
	}
}

func TestToMarkdownTitle(t *testing.T) {
	if got := ToMarkdownTitle("  C# *notes*\nsecond  "); got != `# C# \*notes\* second` {
		t.Fatalf("got %q", got)
	}
}
