package content

import (
	"strings"
	"testing"
)

func TestSanitizeEditorContractSamples(t *testing.T) {
	samples := []struct {
		node string
		html string
	}{
		{"mention", `<p><span data-type="mention" data-id="0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b">@Ada Lovelace</span></p>`},
		{"pageLink", `<p><a data-type="page-link" data-page-id="0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c" href="/spaces/ENG/pages/0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c/getting-started">Getting started</a></p>`},
		{"panel", `<div data-type="panel" data-color="warning"><p>Heads up</p></div>`},
		{"expand", `<details data-type="expand"><summary>More detail</summary><div><p>Hidden text</p></div></details>`},
		{"status", `<p><span data-type="status" data-color="green">DONE</span></p>`},
		{"toc", `<div data-type="toc"></div>`},
		{"comment", `<p>Some <span data-type="comment" data-id="0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5d">anchored words</span> here</p>`},
		{"image", `<img src="/api/v1/attachments/0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5e/content" alt="diagram.png" width="50%" data-attachment-id="0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5e">`},
		{"taskList", `<ul data-type="taskList"><li data-type="taskItem" data-checked="true"><p>Done</p></li><li data-type="taskItem" data-checked="false"><p>Todo</p></li></ul>`},
		{"codeBlock", `<pre><code class="language-go">fmt.Println("hi")</code></pre>`},
		{"heading", `<h2 id="release-notes">Release notes</h2>`},
		{"link", `<p><a target="_blank" rel="noopener noreferrer" href="https://example.com">Example</a></p>`},
		{"table", `<table><tbody><tr><th colspan="1" rowspan="1"><p>Name</p></th><th colspan="1" rowspan="1"><p>Role</p></th></tr><tr><td colspan="1" rowspan="1"><p>Ada</p></td><td colspan="1" rowspan="1"><p>Engineer</p></td></tr></tbody></table>`},
		{"marks", `<p><strong>b</strong><em>i</em><u>u</u><s>s</s><code>c</code><mark>h</mark></p>`},
		{"alignedParagraph", `<p data-align="center">centred</p>`},
		{"alignedHeading", `<h3 data-align="right">right</h3>`},
	}
	for _, c := range samples {
		t.Run(c.node, func(t *testing.T) {
			if got := strings.ReplaceAll(SanitizeHTML(c.html), "&#34;", `"`); got != c.html {
				t.Errorf("sanitiser changed the %s sample\n in: %s\nout: %s", c.node, c.html, got)
			}
		})
	}
}
