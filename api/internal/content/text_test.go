package content

import (
	"strings"
	"testing"
)

func TestExtractText(t *testing.T) {
	doc := `{
	  "type": "doc",
	  "content": [
	    {"type": "heading", "attrs": {"level": 1}, "content": [{"type": "text", "text": "Title"}]},
	    {"type": "paragraph", "content": [
	      {"type": "text", "text": "Hello"},
	      {"type": "text", "text": "world", "marks": [{"type": "bold"}]},
	      {"type": "hardBreak"},
	      {"type": "text", "text": "next line"}
	    ]},
	    {"type": "bulletList", "content": [
	      {"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "one"}]}]},
	      {"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "two"}]}]}
	    ]},
	    {"type": "paragraph", "content": [
	      {"type": "text", "text": "ping"},
	      {"type": "mention", "attrs": {"id": "u1", "label": "Ada"}}
	    ]},
	    {"type": "codeBlock", "attrs": {"language": "go"}, "content": [{"type": "text", "text": "a := 1\nb := 2"}]},
	    {"type": "table", "content": [
	      {"type": "tableRow", "content": [
	        {"type": "tableCell", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "c1"}]}]},
	        {"type": "tableCell", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "c2"}]}]}
	      ]}
	    ]},
	    {"type": "paragraph"}
	  ]
	}`
	got := ExtractText([]byte(doc))
	want := "Title\nHello world\nnext line\none\ntwo\nping @Ada\na := 1\nb := 2\nc1\nc2"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestExtractTextEmptyAndInvalid(t *testing.T) {
	for _, in := range []string{"", "null", "   ", "{not json", `{"type":"doc"}`, `[]`} {
		if got := ExtractText([]byte(in)); got != "" {
			t.Errorf("ExtractText(%q) = %q, want empty", in, got)
		}
	}
}

func TestExtractTextFromHTML(t *testing.T) {
	in := `<h1>Title</h1><p>Hello <strong>bold</strong> &amp; more</p><ul><li>one</li><li>two</li></ul>` +
		`<script>alert(1)</script><style>p{}</style><table><tr><td>a</td><td>b</td></tr></table><p>end<br>tail</p>`
	got := ExtractTextFromHTML(in)
	want := "Title\nHello bold & more\none\ntwo\na\nb\nend\ntail"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
	if ExtractTextFromHTML("") != "" || ExtractTextFromHTML("   ") != "" {
		t.Fatal("empty html should give empty text")
	}
	if strings.Contains(ExtractTextFromHTML(`<p>x</p><script>secret()</script>`), "secret") {
		t.Fatal("script content leaked")
	}
}

func TestMentionedUserIDs(t *testing.T) {
	doc := `{"type":"doc","content":[
	  {"type":"paragraph","content":[
	    {"type":"mention","attrs":{"id":"` + testUserID + `","label":"A"}},
	    {"type":"mention","attrs":{"id":"` + testUserID + `","label":"A"}},
	    {"type":"mention","attrs":{"id":"not-a-uuid"}}
	  ]},
	  {"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[
	    {"type":"mention","attrs":{"id":"0194f0a0-0000-7000-8000-0000000000aa"}}
	  ]}]}]}
	]}`
	ids := MentionedUserIDs([]byte(doc))
	if len(ids) != 2 || ids[0].String() != testUserID || ids[1].String() != "0194f0a0-0000-7000-8000-0000000000aa" {
		t.Fatalf("ids = %v", ids)
	}
	for _, in := range []string{"", "null", "{bad"} {
		got := MentionedUserIDs([]byte(in))
		if got == nil || len(got) != 0 {
			t.Errorf("MentionedUserIDs(%q) = %v, want empty non-nil", in, got)
		}
	}
}
