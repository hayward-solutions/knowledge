package content

import (
	"bytes"
	"html"
	"path"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var (
	mdImporter = goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(util.Prioritized(mdDetailsRenderer{}, 100)),
		),
	)
	mdDetailsOpen  = regexp.MustCompile(`(?is)^\s*<details(?:\s[^>]*)?>\s*<summary(?:\s[^>]*)?>(.*?)</summary>\s*$`)
	mdDetailsClose = regexp.MustCompile(`(?is)^\s*</details>\s*$`)
	mdHighlight    = regexp.MustCompile(`==(\S(?:[^=\n]*?\S)?)==`)
	mdBlockAtoms   = map[atom.Atom]bool{
		atom.P: true, atom.Ul: true, atom.Ol: true, atom.Blockquote: true, atom.Pre: true,
		atom.Table: true, atom.Div: true, atom.Details: true, atom.Hr: true,
		atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true,
	}
)

type mdDetailsRenderer struct{}

func (mdDetailsRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindHTMLBlock, mdRenderHTMLBlock)
}

func mdRenderHTMLBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.HTMLBlock)
	var raw bytes.Buffer
	for i := 0; i < n.Lines().Len(); i++ {
		seg := n.Lines().At(i)
		raw.Write(seg.Value(source))
	}
	if n.HasClosure() {
		raw.Write(n.ClosureLine.Value(source))
	}
	block := raw.String()
	switch {
	case mdDetailsOpen.MatchString(block):
		title := mdDetailsOpen.FindStringSubmatch(block)[1]
		title = html.EscapeString(strings.TrimSpace(html.UnescapeString(title)))
		_, _ = w.WriteString(`<details data-type="expand"><summary>` + title + `</summary><div>` + "\n")
	case mdDetailsClose.MatchString(block):
		_, _ = w.WriteString("</div></details>\n")
	}
	return ast.WalkContinue, nil
}

func FromMarkdown(md string) (string, string, error) {
	md = strings.TrimPrefix(md, "\ufeff")
	md = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(md)
	src := []byte(md)
	doc := mdImporter.Parser().Parse(text.NewReader(src))
	title := mdTakeTitle(doc, src)
	var buf bytes.Buffer
	if err := mdImporter.Renderer().Render(&buf, src, doc); err != nil {
		return "", "", err
	}
	out, err := mdContractHTML(buf.String())
	if err != nil {
		return "", "", err
	}
	return SanitizeHTML(out), title, nil
}

func TitleFromFilename(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	base := path.Base(name)
	if base == "." || base == "/" {
		base = ""
	}
	lower := strings.ToLower(base)
	for _, ext := range []string{".markdown", ".md"} {
		if strings.HasSuffix(lower, ext) {
			base = base[:len(base)-len(ext)]
			break
		}
	}
	return TitleFromFolder(base)
}

func TitleFromFolder(name string) string {
	name = strings.NewReplacer("_", " ", "-", " ").Replace(name)
	name = strings.TrimSpace(mdHTMLSpace.ReplaceAllString(name, " "))
	if name == "" {
		return ""
	}
	r, size := utf8.DecodeRuneInString(name)
	return string(unicode.ToUpper(r)) + name[size:]
}

func mdTakeTitle(doc ast.Node, src []byte) string {
	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		h, ok := c.(*ast.Heading)
		if !ok {
			continue
		}
		if h.Level != 1 {
			return ""
		}
		title := mdHeadingText(h, src)
		doc.RemoveChild(doc, h)
		return title
	}
	return ""
}

func mdHeadingText(h ast.Node, src []byte) string {
	var b strings.Builder
	_ = ast.Walk(h, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := c.(type) {
		case *ast.Text:
			b.Write(t.Segment.Value(src))
			if t.SoftLineBreak() || t.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(t.Value)
		case *ast.AutoLink:
			b.Write(t.URL(src))
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(mdHTMLSpace.ReplaceAllString(html.UnescapeString(b.String()), " "))
}

func mdContractHTML(rendered string) (string, error) {
	ctx := &xhtml.Node{Type: xhtml.ElementNode, DataAtom: atom.Body, Data: "body"}
	nodes, err := xhtml.ParseFragment(strings.NewReader(rendered), ctx)
	if err != nil {
		return "", err
	}
	holder := &xhtml.Node{Type: xhtml.ElementNode, DataAtom: atom.Body, Data: "body"}
	for _, n := range nodes {
		holder.AppendChild(n)
	}
	mdTransform(holder, false)
	var buf bytes.Buffer
	for c := holder.FirstChild; c != nil; c = c.NextSibling {
		if err := xhtml.Render(&buf, c); err != nil {
			return "", err
		}
	}
	return buf.String(), nil
}

func mdSetAttr(n *xhtml.Node, key, val string) {
	for i := range n.Attr {
		if n.Attr[i].Key == key {
			n.Attr[i].Val = val
			return
		}
	}
	n.Attr = append(n.Attr, xhtml.Attribute{Key: key, Val: val})
}

func mdTransform(n *xhtml.Node, inCode bool) {
	if n.Type == xhtml.ElementNode && (n.DataAtom == atom.Code || n.DataAtom == atom.Pre) {
		inCode = true
	}
	var kids []*xhtml.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		kids = append(kids, c)
	}
	for _, c := range kids {
		if c.Type == xhtml.TextNode && !inCode {
			mdHighlightText(c)
			continue
		}
		mdTransform(c, inCode)
	}
	if n.Type != xhtml.ElementNode {
		return
	}
	switch n.DataAtom {
	case atom.Del:
		n.DataAtom = atom.S
		n.Data = "s"
	case atom.Ul:
		mdTaskList(n)
	case atom.Td, atom.Th:
		mdWrapInline(n)
	}
}

func mdHighlightText(t *xhtml.Node) {
	matches := mdHighlight.FindAllStringSubmatchIndex(t.Data, -1)
	if len(matches) == 0 || t.Parent == nil {
		return
	}
	parent, data, last := t.Parent, t.Data, 0
	add := func(n *xhtml.Node) { parent.InsertBefore(n, t) }
	for _, m := range matches {
		if m[0] > last {
			add(&xhtml.Node{Type: xhtml.TextNode, Data: data[last:m[0]]})
		}
		mark := &xhtml.Node{Type: xhtml.ElementNode, DataAtom: atom.Mark, Data: "mark"}
		mark.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: data[m[2]:m[3]]})
		add(mark)
		last = m[1]
	}
	if last < len(data) {
		add(&xhtml.Node{Type: xhtml.TextNode, Data: data[last:]})
	}
	parent.RemoveChild(t)
}

func mdIsSpaceText(n *xhtml.Node) bool {
	return n.Type == xhtml.TextNode && strings.TrimSpace(n.Data) == ""
}

func mdTaskCheckbox(li *xhtml.Node) (input *xhtml.Node, ok bool) {
	var first *xhtml.Node
	for c := li.FirstChild; c != nil; c = c.NextSibling {
		if mdIsSpaceText(c) {
			continue
		}
		first = c
		break
	}
	if first == nil || first.Type != xhtml.ElementNode {
		return nil, false
	}
	if first.DataAtom == atom.P {
		var inner *xhtml.Node
		for c := first.FirstChild; c != nil; c = c.NextSibling {
			if mdIsSpaceText(c) {
				continue
			}
			inner = c
			break
		}
		first = inner
	}
	if first == nil || first.Type != xhtml.ElementNode || first.DataAtom != atom.Input {
		return nil, false
	}
	for _, a := range first.Attr {
		if a.Key == "type" && a.Val == "checkbox" {
			return first, true
		}
	}
	return nil, false
}

func mdTaskList(ul *xhtml.Node) {
	var items []*xhtml.Node
	hasTask := false
	for c := ul.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == xhtml.ElementNode && c.DataAtom == atom.Li {
			items = append(items, c)
			if _, ok := mdTaskCheckbox(c); ok {
				hasTask = true
			}
		}
	}
	if !hasTask {
		return
	}
	mdSetAttr(ul, "data-type", "taskList")
	for _, li := range items {
		checked := false
		if input, ok := mdTaskCheckbox(li); ok {
			for _, a := range input.Attr {
				if a.Key == "checked" {
					checked = true
				}
			}
			next := input.NextSibling
			input.Parent.RemoveChild(input)
			if next != nil && next.Type == xhtml.TextNode {
				next.Data = strings.TrimLeftFunc(next.Data, unicode.IsSpace)
			}
		}
		mdSetAttr(li, "data-type", "taskItem")
		if checked {
			mdSetAttr(li, "data-checked", "true")
		} else {
			mdSetAttr(li, "data-checked", "false")
		}
		mdWrapInline(li)
	}
}

func mdWrapInline(parent *xhtml.Node) {
	var kids []*xhtml.Node
	for c := parent.FirstChild; c != nil; c = c.NextSibling {
		kids = append(kids, c)
	}
	var run []*xhtml.Node
	flush := func() {
		if len(run) == 0 {
			return
		}
		blank := true
		for _, r := range run {
			if !mdIsSpaceText(r) {
				blank = false
			}
		}
		if !blank {
			p := &xhtml.Node{Type: xhtml.ElementNode, DataAtom: atom.P, Data: "p"}
			parent.InsertBefore(p, run[0])
			for _, r := range run {
				parent.RemoveChild(r)
				p.AppendChild(r)
			}
		}
		run = nil
	}
	for _, c := range kids {
		if c.Type == xhtml.ElementNode && mdBlockAtoms[c.DataAtom] {
			flush()
			continue
		}
		run = append(run, c)
	}
	flush()
}
