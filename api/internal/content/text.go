package content

import (
	"encoding/json"
	"html"
	"regexp"
	"strings"

	xhtml "golang.org/x/net/html"
)

type pmNode struct {
	Type    string         `json:"type"`
	Text    string         `json:"text"`
	Attrs   map[string]any `json:"attrs"`
	Content []pmNode       `json:"content"`
}

var (
	reInlineSpace = regexp.MustCompile(`[ \t\r\f\v]+`)
	inlineLeaves  = map[string]bool{
		"mention":   true,
		"status":    true,
		"image":     true,
		"emoji":     true,
		"hardBreak": true,
	}
)

type textBuilder struct {
	lines []string
	parts []string
}

func (b *textBuilder) addInline(s string) {
	if s = strings.TrimSpace(s); s != "" {
		b.parts = append(b.parts, s)
	}
}

func (b *textBuilder) flush() {
	if len(b.parts) == 0 {
		return
	}
	line := strings.Join(b.parts, " ")
	b.parts = b.parts[:0]
	b.lines = append(b.lines, normalizeLine(line))
}

func normalizeLine(s string) string {
	s = reInlineSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func (b *textBuilder) walk(n pmNode) {
	switch {
	case n.Type == "text":
		b.addInline(n.Text)
		return
	case n.Type == "hardBreak":
		b.flush()
		return
	case inlineLeaves[n.Type]:
		b.addInline(leafText(n))
		return
	}
	b.flush()
	if n.Type == "codeBlock" {
		var sb strings.Builder
		for _, c := range n.Content {
			sb.WriteString(c.Text)
		}
		for _, l := range strings.Split(sb.String(), "\n") {
			if l = normalizeLine(l); l != "" {
				b.lines = append(b.lines, l)
			}
		}
		return
	}
	for _, c := range n.Content {
		b.walk(c)
	}
	b.flush()
}

func leafText(n pmNode) string {
	for _, k := range []string{"label", "text", "alt"} {
		if v, ok := n.Attrs[k].(string); ok && v != "" {
			if n.Type == "mention" && !strings.HasPrefix(v, "@") {
				return "@" + v
			}
			return v
		}
	}
	return ""
}

func ExtractText(contentJSON []byte) string {
	trimmed := strings.TrimSpace(string(contentJSON))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	var root pmNode
	if err := json.Unmarshal([]byte(trimmed), &root); err != nil {
		return ""
	}
	b := &textBuilder{}
	b.walk(root)
	b.flush()
	return strings.Join(b.lines, "\n")
}

var htmlBlockTags = map[string]bool{
	"p": true, "div": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"li": true, "ul": true, "ol": true, "br": true, "hr": true, "tr": true, "table": true,
	"blockquote": true, "pre": true, "details": true, "summary": true, "thead": true, "tbody": true,
	"td": true, "th": true,
}

func ExtractTextFromHTML(src string) string {
	if strings.TrimSpace(src) == "" {
		return ""
	}
	z := xhtml.NewTokenizer(strings.NewReader(src))
	var sb strings.Builder
	skip := 0
	for {
		tt := z.Next()
		switch tt {
		case xhtml.ErrorToken:
			return finishHTMLText(sb.String())
		case xhtml.TextToken:
			if skip == 0 {
				sb.WriteString(html.UnescapeString(string(z.Raw())))
			}
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			name, _ := z.TagName()
			tag := string(name)
			if tag == "script" || tag == "style" {
				if tt == xhtml.StartTagToken {
					skip++
				}
				continue
			}
			if htmlBlockTags[tag] {
				sb.WriteByte('\n')
			}
		case xhtml.EndTagToken:
			name, _ := z.TagName()
			tag := string(name)
			if tag == "script" || tag == "style" {
				if skip > 0 {
					skip--
				}
				continue
			}
			if htmlBlockTags[tag] {
				sb.WriteByte('\n')
			}
		}
	}
}

func finishHTMLText(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if l = normalizeLine(l); l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
