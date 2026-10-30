package content

import (
	"strings"
	"testing"
)

const importSample = "# Release Plan\n\nIntro with **bold**, *italic*, ~~gone~~, `code` and ==marked== text.\n\n" +
	"## Steps\n\n- one\n- two\n  - nested\n\n1. first\n2. second\n\n" +
	"- [x] done thing\n- [ ] open thing\n\n" +
	"> quoted\n\n```go\nfmt.Println(\"hi\")\n```\n\n```\nplain\n```\n\n" +
	"| Name | Role |\n| --- | --- |\n| Ada | Engineer |\n\n" +
	"![diagram](/api/v1/attachments/0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5e/content)\n\n" +
	"Visit https://example.com or [docs](https://example.com/docs).\n\n---\n\n" +
	"<details><summary>More &amp; more</summary>\n\nHidden **text**\n\n</details>\n"

func TestFromMarkdownContract(t *testing.T) {
	out, title, err := FromMarkdown(importSample)
	if err != nil {
		t.Fatal(err)
	}
	if title != "Release Plan" {
		t.Fatalf("title = %q", title)
	}
	for _, want := range []string{
		"<strong>bold</strong>", "<em>italic</em>", "<s>gone</s>", "<code>code</code>", "<mark>marked</mark>",
		"<h2>Steps</h2>", "<ol>", "<blockquote>",
		`<ul data-type="taskList">`,
		`<li data-type="taskItem" data-checked="true"><p>done thing</p></li>`,
		`<li data-type="taskItem" data-checked="false"><p>open thing</p></li>`,
		`<pre><code class="language-go">`, "<pre><code>plain",
		"<table>", "<th><p>Name</p></th>", "<td><p>Engineer</p></td>",
		`<img src="/api/v1/attachments/0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5e/content" alt="diagram"/>`,
		`<a href="https://example.com">https://example.com</a>`,
		`<details data-type="expand"><summary>More &amp; more</summary><div>`, "<strong>text</strong>", "</div></details>",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "<h1") || strings.Contains(out, "Release Plan") {
		t.Fatalf("h1 should be removed from body:\n%s", out)
	}
	if strings.Contains(out, "<input") || strings.Contains(out, "<del") {
		t.Fatalf("goldmark-only markup survived:\n%s", out)
	}
}

func TestFromMarkdownSanitiserFixedPoint(t *testing.T) {
	out, _, err := FromMarkdown(importSample)
	if err != nil {
		t.Fatal(err)
	}
	if again := SanitizeHTML(out); again != out {
		t.Fatalf("sanitiser changed output\n--- first ---\n%s\n--- second ---\n%s", out, again)
	}
}

func TestFromMarkdownStripsUnsafe(t *testing.T) {
	md := "Hello\n\n<script>alert(1)</script>\n\n<img src=x onerror=alert(1)>\n\n" +
		"<div onclick=\"x()\">block</div>\n\ninline <b onmouseover=x>raw</b> <iframe src=\"https://evil.test\"></iframe>\n\n" +
		"[bad](javascript:alert(1)) [data](data:text/html;base64,AAAA) ![img](javascript:alert(2))\n\n" +
		"<details onclick=\"x()\"><summary>t<img src=x onerror=y></summary>\n\nbody\n"
	out, _, err := FromMarkdown(md)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"<script", "alert(", "onclick", "onmouseover", "<iframe", "<img src=", "javascript:", "data:text", "evil.test", "<b>"} {
		if strings.Contains(out, bad) {
			t.Fatalf("%q survived in\n%s", bad, out)
		}
	}
	if !strings.Contains(out, "Hello") {
		t.Fatalf("lost safe text:\n%s", out)
	}
}

func TestFromMarkdownTitleRules(t *testing.T) {
	cases := []struct{ name, md, title, bodyHas string }{
		{"first h1", "# Title\n\nbody", "Title", "body"},
		{"emphasis in h1", "# The *real* `title` &amp; more\n\nbody", "The real title & more", "body"},
		{"no h1", "just text", "", "just text"},
		{"h2 first keeps later h1", "## Sub\n\n# Later", "", "<h1>Later</h1>"},
		{"crlf and bom", "\ufeff# Win\r\n\r\ntext\r\n", "Win", "text"},
		{"setext", "Title\n=====\n\ntext", "Title", "text"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, title, err := FromMarkdown(c.md)
			if err != nil {
				t.Fatal(err)
			}
			if title != c.title || !strings.Contains(out, c.bodyHas) {
				t.Fatalf("title %q (want %q), out %s", title, c.title, out)
			}
		})
	}
	if out, title, err := FromMarkdown(""); err != nil || out != "" || title != "" {
		t.Fatalf("empty: %q %q %v", out, title, err)
	}
}

func TestTitleFromFilename(t *testing.T) {
	cases := map[string]string{
		"setup.md":               "Setup",
		"guides/setup-guide.MD":  "Setup guide",
		"release_notes.markdown": "Release notes",
		"README":                 "README",
		".md":                    "",
	}
	for in, want := range cases {
		if got := TitleFromFilename(in); got != want {
			t.Fatalf("TitleFromFilename(%q) = %q want %q", in, got, want)
		}
	}
	if TitleFromFolder("my_docs-v2") != "My docs v2" {
		t.Fatal("folder title")
	}
}

func TestMarkdownRoundTrip(t *testing.T) {
	doc, err := ToMarkdown(mdFixture(t, "blocks.json"))
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := FromMarkdown("# Round trip\n\n" + doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<h2>Intro</h2>", "<strong>bold</strong>", "<mark>marked</mark>", "<ol>",
		`data-checked="true"`, `<pre><code class="language-go">`, "<table>", "<hr/>", `<img src="/api/v1/attachments/`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("round trip lost %q:\n%s", want, out)
		}
	}
	if SanitizeHTML(out) != out {
		t.Fatal("round trip output not sanitiser-stable")
	}
	custom, err := ToMarkdown(mdFixture(t, "custom.json"))
	if err != nil {
		t.Fatal(err)
	}
	out, _, err = FromMarkdown(custom)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `<details data-type="expand"><summary>More &lt;detail&gt; &amp; stuff</summary><div>`) || SanitizeHTML(out) != out {
		t.Fatalf("custom round trip:\n%s", out)
	}
}
