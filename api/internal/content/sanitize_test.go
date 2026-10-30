package content

import (
	"strings"
	"testing"
)

const (
	testUserID    = "0194f0a0-0000-7000-8000-000000000001"
	testPageID    = "0194f0a0-0000-7000-8000-000000000002"
	testCommentID = "0194f0a0-0000-7000-8000-000000000003"
	testAttachID  = "0194f0a0-0000-7000-8000-000000000004"
)

func TestSanitizeStripsXSSVectors(t *testing.T) {
	cases := map[string]struct {
		in      string
		banned  []string
		wantAny string
	}{
		"script tag":              {`<p>hi</p><script>alert(1)</script>`, []string{"script", "alert"}, "<p>hi</p>"},
		"inline handler":          {`<p onclick="alert(1)">x</p>`, []string{"onclick", "alert"}, "<p>x</p>"},
		"img onerror":             {`<img src="x" onerror="alert(1)">`, []string{"onerror", "alert"}, `src="x"`},
		"javascript href":         {`<a href="javascript:alert(1)">x</a>`, []string{"javascript", "href"}, "x"},
		"mixed case javascript":   {`<a href="JaVaScRiPt:alert(1)">x</a>`, []string{"javascript", "href"}, "x"},
		"entity javascript":       {`<a href="&#106;avascript:alert(1)">x</a>`, []string{"javascript", "alert", "href"}, "x"},
		"tab javascript":          {"<a href=\"java\tscript:alert(1)\">x</a>", []string{"script:", "href"}, "x"},
		"data html href":          {`<a href="data:text/html;base64,PHNjcmlwdD4=">x</a>`, []string{"data:", "href"}, "x"},
		"vbscript href":           {`<a href="vbscript:msgbox(1)">x</a>`, []string{"vbscript", "href"}, "x"},
		"data img src":            {`<img src="data:image/svg+xml;base64,PHN2Zz4=">`, []string{"data:"}, ""},
		"javascript img src":      {`<img src="javascript:alert(1)">`, []string{"javascript"}, ""},
		"iframe":                  {`<iframe src="https://evil.test"></iframe><p>ok</p>`, []string{"iframe", "evil.test"}, "<p>ok</p>"},
		"object embed":            {`<object data="x"></object><embed src="y">`, []string{"object", "embed"}, ""},
		"svg onload":              {`<svg onload="alert(1)"><circle/></svg>`, []string{"svg", "onload", "alert"}, ""},
		"style block":             {`<style>body{display:none}</style><p>ok</p>`, []string{"style", "display"}, "<p>ok</p>"},
		"style attribute":         {`<p style="background:url(javascript:alert(1))">x</p>`, []string{"style", "javascript"}, "<p>x</p>"},
		"form":                    {`<form action="https://evil.test"><input name="a"></form>`, []string{"form", "input", "evil.test"}, ""},
		"meta refresh":            {`<meta http-equiv="refresh" content="0;url=https://evil.test">`, []string{"meta", "refresh"}, ""},
		"base tag":                {`<base href="https://evil.test/">`, []string{"base", "evil.test"}, ""},
		"unknown data-type":       {`<div data-type="evil">x</div>`, []string{"evil", "data-type"}, "x"},
		"bad data-id":             {`<span data-type="mention" data-id="1 onmouseover=alert(1)">x</span>`, []string{"onmouseover", "data-id"}, `data-type="mention"`},
		"bad data-color":          {`<span data-type="status" data-color="red;background:url(x)">x</span>`, []string{"background", "data-color"}, `data-type="status"`},
		"bad code class":          {`<pre><code class="x\" onmouseover=\"alert(1)">c</code></pre>`, []string{"onmouseover", "class"}, "c"},
		"non-language code class": {`<code class="hljs">c</code>`, []string{"class"}, "<code>c</code>"},
		"heading id with quotes":  {`<h1 id="a&quot; onclick=&quot;alert(1)">t</h1>`, []string{"onclick"}, "t"},
		"bad heading id":          {`<h1 id="1bad">t</h1>`, []string{"id="}, "<h1>t</h1>"},
		"page link bad id":        {`<a data-type="page-link" data-page-id="x&quot; onclick=&quot;1" href="/p">t</a>`, []string{"onclick"}, "t"},
		"img dimension":           {`<img src="/a.png" width="100;onload=1">`, []string{"width", "onload"}, `src="/a.png"`},
		"rel injection":           {`<a href="/x" rel="opener evil">t</a>`, []string{"rel="}, "t"},
		"target top":              {`<a href="/x" target="_top">t</a>`, []string{"target"}, "t"},
		"class attribute":         {`<p class="x">t</p>`, []string{"class"}, "<p>t</p>"},
		"comment":                 {`<p>a</p><!-- <script>alert(1)</script> -->`, []string{"script", "alert", "<!--"}, "<p>a</p>"},
		"unclosed tag":            {`<p>a</p><img src="/x.png" onerror="alert(1)`, []string{"alert", "onerror"}, ""},
		"mutation xss noscript":   {`<noscript><p title="</noscript><img src=x onerror=alert(1)>">`, []string{"onerror"}, ""},
		"input autofocus":         {`<input autofocus onfocus="alert(1)">`, []string{"input", "onfocus"}, ""},
		"button formaction":       {`<button formaction="javascript:alert(1)">x</button>`, []string{"button", "formaction", "javascript"}, ""},
		"details ontoggle":        {`<details open ontoggle="alert(1)"><summary>s</summary>b</details>`, []string{"ontoggle", "open", "alert"}, "<summary>s</summary>"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			out := SanitizeHTML(c.in)
			lower := strings.ToLower(out)
			for _, b := range c.banned {
				if strings.Contains(lower, strings.ToLower(b)) {
					t.Errorf("output %q still contains %q", out, b)
				}
			}
			if c.wantAny != "" && !strings.Contains(out, c.wantAny) {
				t.Errorf("output %q missing %q", out, c.wantAny)
			}
		})
	}
}

func TestSanitizeKeepsContractElements(t *testing.T) {
	cases := map[string]string{
		"paragraph":       `<p>text</p>`,
		"heading with id": `<h2 id="intro">Intro</h2>`,
		"heading levels":  `<h1>a</h1><h3>b</h3><h6>c</h6>`,
		"bullet list":     `<ul><li><p>a</p></li></ul>`,
		"ordered list":    `<ol><li><p>a</p></li></ol>`,
		"task list":       `<ul data-type="taskList"><li data-type="taskItem" data-checked="true"><p>done</p></li><li data-type="taskItem" data-checked="false"><p>todo</p></li></ul>`,
		"blockquote":      `<blockquote><p>q</p></blockquote>`,
		"code block":      `<pre><code class="language-go">fmt.Println()</code></pre>`,
		"inline marks":    `<p><strong>b</strong><em>i</em><u>u</u><s>s</s><mark>m</mark><code>c</code></p>`,
		"link":            `<a href="https://example.com/a?b=c" rel="noopener noreferrer" target="_blank">x</a>`,
		"mailto":          `<a href="mailto:a@example.com">mail</a>`,
		"relative link":   `<a href="/spaces/ENG/pages/1/slug">rel</a>`,
		"page link":       `<a data-type="page-link" data-page-id="` + testPageID + `" href="/spaces/ENG/pages/` + testPageID + `/title">Title</a>`,
		"image":           `<img src="/api/v1/attachments/` + testAttachID + `/content" data-attachment-id="` + testAttachID + `" alt="a" title="t" width="320" height="200">`,
		"remote image":    `<img src="https://example.com/a.png" alt="x">`,
		"table":           `<table><thead><tr><th colspan="2" colwidth="100,200">h</th></tr></thead><tbody><tr><td rowspan="2">c</td><td>d</td></tr></tbody></table>`,
		"hr and br":       `<p>a<br>b</p><hr>`,
		"mention":         `<span data-type="mention" data-id="` + testUserID + `">@Name</span>`,
		"status":          `<span data-type="status" data-color="green">DONE</span>`,
		"inline comment":  `<span data-type="comment" data-id="` + testCommentID + `">anchored</span>`,
		"panel":           `<div data-type="panel" data-color="warning"><p>careful</p></div>`,
		"toc":             `<div data-type="toc"></div>`,
		"expand":          `<details data-type="expand"><summary>Title</summary><div><p>inner</p></div></details>`,
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			out := SanitizeHTML(in)
			norm := func(s string) string {
				s = strings.ReplaceAll(s, "<br/>", "<br>")
				s = strings.ReplaceAll(s, "<hr/>", "<hr>")
				s = strings.ReplaceAll(s, " />", ">")
				s = strings.ReplaceAll(s, "/>", ">")
				return s
			}
			if norm(out) != norm(in) {
				t.Errorf("contract element altered\n in: %s\nout: %s", in, out)
			}
		})
	}
}

func TestSanitizeLinkSchemes(t *testing.T) {
	for _, href := range []string{"http://a.test/x", "https://a.test/x", "mailto:a@b.test", "/relative/path", "relative/path", "#anchor"} {
		out := SanitizeHTML(`<a href="` + href + `">x</a>`)
		if !strings.Contains(out, `href="`+href+`"`) {
			t.Errorf("href %q was stripped: %s", href, out)
		}
	}
	for _, href := range []string{"ftp://a.test/x", "tel:123", "file:///etc/passwd", "javascript:alert(1)", "data:text/plain,hi"} {
		out := SanitizeHTML(`<a href="` + href + `">x</a>`)
		if strings.Contains(out, "href") {
			t.Errorf("href %q was kept: %s", href, out)
		}
	}
}

func TestSanitizeIdempotent(t *testing.T) {
	in := `<h1 id="a">t</h1><p><strong>x</strong> <a href="/p" rel="noopener">l</a></p><script>1</script>`
	once := SanitizeHTML(in)
	if twice := SanitizeHTML(once); twice != once {
		t.Errorf("not idempotent:\n%s\n%s", once, twice)
	}
}
