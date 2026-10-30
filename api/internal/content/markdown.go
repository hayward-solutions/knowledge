package content

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type mdMark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs"`
}

type mdNode struct {
	Type    string         `json:"type"`
	Text    string         `json:"text"`
	Attrs   map[string]any `json:"attrs"`
	Content []mdNode       `json:"content"`
	Marks   []mdMark       `json:"marks"`
}

type mdBlock struct {
	typ  string
	text string
}

type mdPiece struct {
	text  string
	raw   bool
	brk   bool
	marks []mdMark
}

var (
	mdPanelLabels = map[string]string{
		"info":    "Info",
		"note":    "Note",
		"success": "Success",
		"warning": "Warning",
		"error":   "Error",
	}
	mdMarkPriority = map[string]int{"link": 0, "bold": 1, "italic": 2, "strike": 3, "highlight": 4, "code": 5}
	mdMarkAliases  = map[string]string{
		"bold": "bold", "strong": "bold",
		"italic": "italic", "em": "italic",
		"strike": "strike", "s": "strike", "del": "strike",
		"highlight": "highlight", "mark": "highlight",
		"code": "code", "link": "link",
	}
	mdInlineTypes = map[string]bool{
		"text": true, "hardBreak": true, "mention": true, "pageLink": true,
		"status": true, "emoji": true,
	}
	mdListTypes     = map[string]bool{"bulletList": true, "orderedList": true, "taskList": true}
	mdHTMLSpace     = regexp.MustCompile(`[ \t\r\n\f]+`)
	mdLineStartRule = []struct {
		re   *regexp.Regexp
		repl string
	}{
		{regexp.MustCompile(`(?m)^(\s*)(#{1,6})(\s|$)`), `$1\$2$3`},
		{regexp.MustCompile(`(?m)^(\s*)>`), `$1\>`},
		{regexp.MustCompile(`(?m)^(\s*)([-+])(\s|$)`), `$1\$2$3`},
		{regexp.MustCompile(`(?m)^(\s*)(\d+)([.)])(\s|$)`), `$1$2\$3$4`},
		{regexp.MustCompile(`(?m)^(\s*)([-_=]{3,})\s*$`), `$1\$2`},
	}
	mdBlockElements = map[atom.Atom]bool{
		atom.P: true, atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true,
		atom.Ul: true, atom.Ol: true, atom.Li: true, atom.Blockquote: true, atom.Pre: true, atom.Hr: true,
		atom.Table: true, atom.Thead: true, atom.Tbody: true, atom.Tfoot: true, atom.Tr: true,
		atom.Td: true, atom.Th: true, atom.Div: true, atom.Details: true, atom.Summary: true,
		atom.Figure: true, atom.Section: true, atom.Article: true, atom.Main: true, atom.Header: true,
		atom.Footer: true, atom.Nav: true, atom.Aside: true, atom.Dl: true, atom.Dt: true, atom.Dd: true,
	}
)

func mdAttrStr(a map[string]any, k string) string {
	switch v := a[k].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}

func mdAttrInt(a map[string]any, k string, def int) int {
	switch v := a[k].(type) {
	case float64:
		return int(v)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}

func mdAttrBool(a map[string]any, k string) bool {
	switch v := a[k].(type) {
	case bool:
		return v
	case string:
		return v == "true"
	}
	return false
}

func ToMarkdown(contentJSON []byte) (string, error) {
	trimmed := bytes.TrimSpace(contentJSON)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return "", nil
	}
	var root mdNode
	if err := json.Unmarshal(trimmed, &root); err != nil {
		return "", fmt.Errorf("invalid content json: %w", err)
	}
	if root.Type == "doc" {
		return mdJoinBlocks(mdRenderBlocks(root.Content)), nil
	}
	return mdJoinBlocks(mdRenderBlocks([]mdNode{root})), nil
}

func ToMarkdownTitle(title string) string {
	title = strings.TrimSpace(mdHTMLSpace.ReplaceAllString(title, " "))
	return "# " + mdEscape(title)
}

func mdRenderBlocks(nodes []mdNode) []mdBlock {
	out := make([]mdBlock, 0, len(nodes))
	for _, n := range nodes {
		if s := mdRenderBlock(n); s != "" {
			out = append(out, mdBlock{typ: n.Type, text: s})
		}
	}
	return out
}

func mdJoinBlocks(bs []mdBlock) string {
	texts := make([]string, len(bs))
	for i, b := range bs {
		texts[i] = b.text
	}
	return strings.Join(texts, "\n\n")
}

func mdParagraph(nodes []mdNode) string {
	return mdEscapeLineStarts(strings.TrimSpace(mdInline(nodes, "\\\n")))
}

func mdRenderBlock(n mdNode) string {
	switch n.Type {
	case "doc":
		return mdJoinBlocks(mdRenderBlocks(n.Content))
	case "paragraph":
		return mdParagraph(n.Content)
	case "heading":
		text := strings.TrimSpace(mdInline(n.Content, " "))
		if text == "" {
			return ""
		}
		level := mdAttrInt(n.Attrs, "level", 1)
		if level < 1 {
			level = 1
		}
		if level > 6 {
			level = 6
		}
		return strings.Repeat("#", level) + " " + text
	case "bulletList":
		return mdRenderList(n, "bullet")
	case "orderedList":
		return mdRenderList(n, "ordered")
	case "taskList":
		return mdRenderList(n, "task")
	case "blockquote":
		inner := mdJoinBlocks(mdRenderBlocks(n.Content))
		if inner == "" {
			return ""
		}
		return mdQuote(inner)
	case "codeBlock":
		return mdRenderCode(n)
	case "horizontalRule":
		return "---"
	case "table":
		return mdRenderTable(n)
	case "image":
		return mdImage(n)
	case "panel":
		return mdRenderPanel(n)
	case "expand":
		return mdRenderExpand(n)
	case "toc":
		return ""
	}
	if mdInlineTypes[n.Type] {
		return mdParagraph([]mdNode{n})
	}
	if len(n.Content) == 0 {
		return ""
	}
	if mdInlineTypes[n.Content[0].Type] {
		return mdParagraph(n.Content)
	}
	return mdJoinBlocks(mdRenderBlocks(n.Content))
}

func mdQuote(inner string) string {
	lines := strings.Split(inner, "\n")
	for i, l := range lines {
		if l == "" {
			lines[i] = ">"
		} else {
			lines[i] = "> " + l
		}
	}
	return strings.Join(lines, "\n")
}

func mdRenderList(n mdNode, kind string) string {
	start := mdAttrInt(n.Attrs, "start", 1)
	items := make([]string, 0, len(n.Content))
	for i, it := range n.Content {
		marker, indent := "- ", 2
		switch {
		case it.Type == "taskItem" || kind == "task":
			if mdAttrBool(it.Attrs, "checked") {
				marker = "- [x] "
			} else {
				marker = "- [ ] "
			}
		case kind == "ordered":
			marker = strconv.Itoa(start+i) + ". "
			indent = len(marker)
		}
		items = append(items, mdPrefixItem(mdItemBody(it.Content), marker, indent))
	}
	return strings.Join(items, "\n")
}

func mdItemBody(children []mdNode) string {
	var b strings.Builder
	for i, blk := range mdRenderBlocks(children) {
		if i > 0 {
			if mdListTypes[blk.typ] {
				b.WriteString("\n")
			} else {
				b.WriteString("\n\n")
			}
		}
		b.WriteString(blk.text)
	}
	return b.String()
}

func mdPrefixItem(body, marker string, indent int) string {
	if body == "" {
		return strings.TrimRight(marker, " ")
	}
	pad := strings.Repeat(" ", indent)
	lines := strings.Split(body, "\n")
	for i := range lines {
		switch {
		case i == 0:
			lines[i] = marker + lines[i]
		case lines[i] != "":
			lines[i] = pad + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

func mdNodeText(nodes []mdNode) string {
	var sb strings.Builder
	for _, c := range nodes {
		switch c.Type {
		case "hardBreak":
			sb.WriteString("\n")
		default:
			sb.WriteString(c.Text)
		}
	}
	return sb.String()
}

func mdRenderCode(n mdNode) string {
	code := strings.TrimSuffix(mdNodeText(n.Content), "\n")
	lang := ""
	if f := strings.Fields(mdAttrStr(n.Attrs, "language")); len(f) > 0 {
		lang = strings.ReplaceAll(f[0], "`", "")
	}
	fence := "```"
	for strings.Contains(code, fence) {
		fence += "`"
	}
	if code == "" {
		return fence + lang + "\n" + fence
	}
	return fence + lang + "\n" + code + "\n" + fence
}

func mdRenderTable(n mdNode) string {
	var rows [][]string
	width := 0
	for _, row := range n.Content {
		var cells []string
		for _, c := range row.Content {
			cells = append(cells, mdCellText(c))
			for k := 1; k < mdAttrInt(c.Attrs, "colspan", 1) && k < 64; k++ {
				cells = append(cells, "")
			}
		}
		if len(cells) > width {
			width = len(cells)
		}
		rows = append(rows, cells)
	}
	if width == 0 {
		return ""
	}
	lines := make([]string, 0, len(rows)+1)
	for i, cells := range rows {
		for len(cells) < width {
			cells = append(cells, "")
		}
		lines = append(lines, "| "+strings.Join(cells, " | ")+" |")
		if i == 0 {
			sep := make([]string, width)
			for j := range sep {
				sep[j] = "---"
			}
			lines = append(lines, "| "+strings.Join(sep, " | ")+" |")
		}
	}
	return strings.Join(lines, "\n")
}

func mdCellText(cell mdNode) string {
	var parts []string
	for _, c := range cell.Content {
		if s := mdFlat(c); s != "" {
			parts = append(parts, s)
		}
	}
	text := strings.Join(parts, " ")
	text = strings.ReplaceAll(text, "\n", " ")
	return strings.ReplaceAll(text, "|", "\\|")
}

func mdFlat(n mdNode) string {
	switch n.Type {
	case "paragraph", "heading":
		return strings.TrimSpace(mdInline(n.Content, "<br>"))
	case "toc":
		return ""
	case "image":
		return mdImage(n)
	case "codeBlock":
		return mdCodeSpan(mdNodeText(n.Content))
	case "expand":
		var parts []string
		for _, c := range n.Content {
			if s := mdFlat(c); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " ")
	}
	if mdInlineTypes[n.Type] {
		return strings.TrimSpace(mdInline([]mdNode{n}, "<br>"))
	}
	var parts []string
	for _, c := range n.Content {
		if s := mdFlat(c); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

func mdImage(n mdNode) string {
	src := mdAttrStr(n.Attrs, "src")
	if src == "" {
		return ""
	}
	alt := strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]", "\n", " ").Replace(mdAttrStr(n.Attrs, "alt"))
	return "![" + alt + "](" + mdDest(src) + ")"
}

func mdRenderPanel(n mdNode) string {
	label := mdPanelLabels[mdAttrStr(n.Attrs, "color")]
	if label == "" {
		label = "Info"
	}
	prefix := "**" + label + ":**"
	bs := mdRenderBlocks(n.Content)
	if len(bs) > 0 && bs[0].typ == "paragraph" {
		bs[0].text = prefix + " " + bs[0].text
	} else {
		bs = append([]mdBlock{{typ: "paragraph", text: prefix}}, bs...)
	}
	return mdQuote(mdJoinBlocks(bs))
}

func mdPlain(nodes []mdNode) string {
	var sb strings.Builder
	for _, c := range nodes {
		switch c.Type {
		case "hardBreak":
			sb.WriteString(" ")
		case "mention":
			sb.WriteString("@" + strings.TrimPrefix(mdMentionLabel(c), "@"))
		case "pageLink":
			sb.WriteString(mdAttrStr(c.Attrs, "title"))
		case "status":
			sb.WriteString(mdAttrStr(c.Attrs, "text"))
		default:
			sb.WriteString(c.Text)
			sb.WriteString(mdPlain(c.Content))
		}
	}
	return strings.TrimSpace(mdHTMLSpace.ReplaceAllString(sb.String(), " "))
}

func mdRenderExpand(n mdNode) string {
	title := ""
	var body []mdNode
	for _, c := range n.Content {
		switch c.Type {
		case "expandSummary":
			title = mdPlain(c.Content)
		case "expandBody":
			body = append(body, c.Content...)
		default:
			body = append(body, c)
		}
	}
	head := "<details><summary>" + html.EscapeString(title) + "</summary>"
	inner := mdJoinBlocks(mdRenderBlocks(body))
	if inner == "" {
		return head + "\n\n</details>"
	}
	return head + "\n\n" + inner + "\n\n</details>"
}

func mdMentionLabel(n mdNode) string {
	for _, k := range []string{"label", "text", "id"} {
		if v := mdAttrStr(n.Attrs, k); v != "" {
			return v
		}
	}
	return n.Text
}

func mdCanonicalMarks(marks []mdMark) []mdMark {
	seen := map[string]bool{}
	out := make([]mdMark, 0, len(marks))
	for _, m := range marks {
		t, ok := mdMarkAliases[m.Type]
		if !ok {
			continue
		}
		m.Type = t
		if t == "link" && strings.TrimSpace(mdAttrStr(m.Attrs, "href")) == "" {
			continue
		}
		key := mdMarkKey(m)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool { return mdMarkPriority[out[i].Type] < mdMarkPriority[out[j].Type] })
	return out
}

func mdMarkKey(m mdMark) string {
	if m.Type == "link" {
		return "link|" + mdAttrStr(m.Attrs, "href")
	}
	return m.Type
}

func mdMarksKey(marks []mdMark) string {
	keys := make([]string, len(marks))
	for i, m := range marks {
		keys[i] = mdMarkKey(m)
	}
	return strings.Join(keys, "\x00")
}

func mdCollectPieces(nodes []mdNode, inherited []mdMark, br string, out *[]mdPiece) {
	for _, n := range nodes {
		all := make([]mdMark, 0, len(inherited)+len(n.Marks))
		all = append(all, inherited...)
		all = append(all, n.Marks...)
		marks := mdCanonicalMarks(all)
		switch n.Type {
		case "text":
			if n.Text != "" {
				*out = append(*out, mdPiece{text: n.Text, marks: marks})
			}
		case "hardBreak":
			*out = append(*out, mdPiece{text: br, brk: true})
		case "mention":
			label := strings.TrimPrefix(mdMentionLabel(n), "@")
			*out = append(*out, mdPiece{text: "@" + label, marks: marks})
		case "pageLink":
			title := mdAttrStr(n.Attrs, "title")
			href := mdAttrStr(n.Attrs, "href")
			switch {
			case href == "":
				*out = append(*out, mdPiece{text: title, marks: marks})
			default:
				if title == "" {
					title = href
				}
				*out = append(*out, mdPiece{text: "[" + mdEscape(title) + "](" + mdDest(href) + ")", raw: true, marks: marks})
			}
		case "status":
			text := mdAttrStr(n.Attrs, "text")
			*out = append(*out, mdPiece{text: "**[" + mdEscape(text) + "]**", raw: true, marks: marks})
		case "image":
			if s := mdImage(n); s != "" {
				*out = append(*out, mdPiece{text: s, raw: true, marks: marks})
			}
		default:
			if n.Text != "" {
				*out = append(*out, mdPiece{text: n.Text, marks: marks})
			}
			mdCollectPieces(n.Content, marks, br, out)
		}
	}
}

func mdInline(nodes []mdNode, br string) string {
	var pieces []mdPiece
	mdCollectPieces(nodes, nil, br, &pieces)
	for len(pieces) > 0 && pieces[0].brk {
		pieces = pieces[1:]
	}
	for len(pieces) > 0 && pieces[len(pieces)-1].brk {
		pieces = pieces[:len(pieces)-1]
	}
	pieces = mdMergePieces(pieces)
	pieces = mdSplitSpace(pieces)
	pieces = mdMergePieces(pieces)
	return mdEmit(pieces)
}

func mdMergePieces(in []mdPiece) []mdPiece {
	out := make([]mdPiece, 0, len(in))
	for _, p := range in {
		if n := len(out); n > 0 {
			last := &out[n-1]
			if !last.raw && !last.brk && !p.raw && !p.brk && mdMarksKey(last.marks) == mdMarksKey(p.marks) {
				last.text += p.text
				continue
			}
		}
		out = append(out, p)
	}
	return out
}

func mdSplitSpace(in []mdPiece) []mdPiece {
	out := make([]mdPiece, 0, len(in))
	for _, p := range in {
		if p.raw || p.brk || len(p.marks) == 0 {
			out = append(out, p)
			continue
		}
		core := strings.TrimFunc(p.text, unicode.IsSpace)
		if core == "" {
			out = append(out, mdPiece{text: p.text})
			continue
		}
		at := strings.Index(p.text, core)
		lead, trail := p.text[:at], p.text[at+len(core):]
		if lead != "" {
			out = append(out, mdPiece{text: lead})
		}
		out = append(out, mdPiece{text: core, marks: p.marks})
		if trail != "" {
			out = append(out, mdPiece{text: trail})
		}
	}
	return out
}

func mdMarkOpen(m mdMark) string {
	switch m.Type {
	case "bold":
		return "**"
	case "italic":
		return "*"
	case "strike":
		return "~~"
	case "highlight":
		return "=="
	case "link":
		return "["
	}
	return ""
}

func mdMarkClose(m mdMark) string {
	if m.Type == "link" {
		return "](" + mdDest(strings.TrimSpace(mdAttrStr(m.Attrs, "href"))) + ")"
	}
	return mdMarkOpen(m)
}

func mdEmit(pieces []mdPiece) string {
	var b strings.Builder
	var open []mdMark
	for _, p := range pieces {
		hasCode := false
		want := make([]mdMark, 0, len(p.marks))
		for _, m := range p.marks {
			if m.Type == "code" {
				hasCode = true
				continue
			}
			want = append(want, m)
		}
		k := 0
		for k < len(open) && k < len(want) && mdMarkKey(open[k]) == mdMarkKey(want[k]) {
			k++
		}
		for i := len(open) - 1; i >= k; i-- {
			b.WriteString(mdMarkClose(open[i]))
		}
		for i := k; i < len(want); i++ {
			b.WriteString(mdMarkOpen(want[i]))
		}
		open = want
		switch {
		case p.raw || p.brk:
			b.WriteString(p.text)
		case hasCode:
			b.WriteString(mdCodeSpan(p.text))
		default:
			b.WriteString(mdEscape(p.text))
		}
	}
	for i := len(open) - 1; i >= 0; i-- {
		b.WriteString(mdMarkClose(open[i]))
	}
	return b.String()
}

func mdEscape(s string) string {
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\\', '`', '*', '_', '[', ']', '<', '~':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func mdEscapeLineStarts(s string) string {
	for _, rule := range mdLineStartRule {
		s = rule.re.ReplaceAllString(s, rule.repl)
	}
	return s
}

func mdCodeSpan(s string) string {
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
	if s == "" {
		return ""
	}
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			if run > longest {
				longest = run
			}
		} else {
			run = 0
		}
	}
	delim := strings.Repeat("`", longest+1)
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") ||
		(strings.HasPrefix(s, " ") && strings.HasSuffix(s, " ") && strings.TrimSpace(s) != "") {
		return delim + " " + s + " " + delim
	}
	return delim + s + delim
}

func mdDest(href string) string {
	if strings.ContainsAny(href, " ()<>\t\n\r") {
		return "<" + strings.NewReplacer("<", "%3C", ">", "%3E", "\n", "%0A", "\r", "%0D", "\t", "%09").Replace(href) + ">"
	}
	return href
}

func ToMarkdownFromHTML(src string) (string, error) {
	if strings.TrimSpace(src) == "" {
		return "", nil
	}
	root, err := xhtml.Parse(strings.NewReader(src))
	if err != nil {
		return "", err
	}
	body := mdFindBody(root)
	if body == nil {
		return "", nil
	}
	return mdJoinBlocks(mdRenderBlocks(mdDOMBlocks(body, 0))), nil
}

func mdFindBody(n *xhtml.Node) *xhtml.Node {
	if n.Type == xhtml.ElementNode && n.DataAtom == atom.Body {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if b := mdFindBody(c); b != nil {
			return b
		}
	}
	return nil
}

func mdDOMAttr(n *xhtml.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func mdDOMHasAttr(n *xhtml.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

func mdDOMText(n *xhtml.Node) string {
	if n.Type == xhtml.TextNode {
		return n.Data
	}
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sb.WriteString(mdDOMText(c))
	}
	return sb.String()
}

func mdDOMBlocks(parent *xhtml.Node, skip atom.Atom) []mdNode {
	var out []mdNode
	var run []mdNode
	flush := func() {
		if p := mdDOMParagraph(run); p != nil {
			out = append(out, *p)
		}
		run = nil
	}
	for c := parent.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == xhtml.ElementNode && skip != 0 && c.DataAtom == skip {
			continue
		}
		if c.Type == xhtml.ElementNode && mdBlockElements[c.DataAtom] {
			flush()
			out = append(out, mdDOMBlock(c)...)
			continue
		}
		run = append(run, mdDOMInline(c, nil)...)
	}
	flush()
	return out
}

func mdDOMParagraph(run []mdNode) *mdNode {
	for len(run) > 0 && run[0].Type == "text" {
		run[0].Text = strings.TrimLeft(run[0].Text, " ")
		if run[0].Text != "" {
			break
		}
		run = run[1:]
	}
	for len(run) > 0 && run[len(run)-1].Type == "text" {
		last := &run[len(run)-1]
		last.Text = strings.TrimRight(last.Text, " ")
		if last.Text != "" {
			break
		}
		run = run[:len(run)-1]
	}
	if len(run) == 0 {
		return nil
	}
	return &mdNode{Type: "paragraph", Content: run}
}

func mdDOMInlineChildren(el *xhtml.Node) []mdNode {
	var run []mdNode
	for c := el.FirstChild; c != nil; c = c.NextSibling {
		run = append(run, mdDOMInline(c, nil)...)
	}
	if p := mdDOMParagraph(run); p != nil {
		return p.Content
	}
	return nil
}

func mdWithMark(marks []mdMark, m mdMark) []mdMark {
	out := make([]mdMark, 0, len(marks)+1)
	out = append(out, marks...)
	return append(out, m)
}

func mdDOMInline(c *xhtml.Node, marks []mdMark) []mdNode {
	switch c.Type {
	case xhtml.TextNode:
		text := mdHTMLSpace.ReplaceAllString(c.Data, " ")
		if text == "" {
			return nil
		}
		return []mdNode{{Type: "text", Text: text, Marks: marks}}
	case xhtml.ElementNode:
	default:
		return nil
	}
	next := marks
	switch c.DataAtom {
	case atom.Script, atom.Style:
		return nil
	case atom.Br:
		return []mdNode{{Type: "hardBreak", Marks: marks}}
	case atom.Img:
		return []mdNode{{Type: "image", Marks: marks, Attrs: map[string]any{
			"src": mdDOMAttr(c, "src"), "alt": mdDOMAttr(c, "alt"),
		}}}
	case atom.Strong, atom.B:
		next = mdWithMark(marks, mdMark{Type: "bold"})
	case atom.Em, atom.I:
		next = mdWithMark(marks, mdMark{Type: "italic"})
	case atom.S, atom.Del, atom.Strike:
		next = mdWithMark(marks, mdMark{Type: "strike"})
	case atom.Mark:
		next = mdWithMark(marks, mdMark{Type: "highlight"})
	case atom.Code:
		next = mdWithMark(marks, mdMark{Type: "code"})
	case atom.A:
		href := mdDOMAttr(c, "href")
		if mdDOMAttr(c, "data-type") == "page-link" {
			return []mdNode{{Type: "pageLink", Marks: marks, Attrs: map[string]any{
				"pageId": mdDOMAttr(c, "data-page-id"),
				"title":  strings.TrimSpace(mdDOMText(c)),
				"href":   href,
			}}}
		}
		if href != "" {
			next = mdWithMark(marks, mdMark{Type: "link", Attrs: map[string]any{"href": href}})
		}
	case atom.Span:
		switch mdDOMAttr(c, "data-type") {
		case "mention":
			return []mdNode{{Type: "mention", Marks: marks, Attrs: map[string]any{
				"id":    mdDOMAttr(c, "data-id"),
				"label": strings.TrimPrefix(strings.TrimSpace(mdDOMText(c)), "@"),
			}}}
		case "status":
			return []mdNode{{Type: "status", Marks: marks, Attrs: map[string]any{
				"text":  strings.TrimSpace(mdDOMText(c)),
				"color": mdDOMAttr(c, "data-color"),
			}}}
		}
	}
	var out []mdNode
	for k := c.FirstChild; k != nil; k = k.NextSibling {
		out = append(out, mdDOMInline(k, next)...)
	}
	return out
}

func mdDOMItems(list *xhtml.Node, itemType string) []mdNode {
	var items []mdNode
	for c := list.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != xhtml.ElementNode || c.DataAtom != atom.Li {
			continue
		}
		typ := itemType
		if mdDOMAttr(c, "data-type") == "taskItem" {
			typ = "taskItem"
		}
		it := mdNode{Type: typ, Content: mdDOMBlocks(c, 0)}
		if typ == "taskItem" {
			it.Attrs = map[string]any{"checked": mdDOMAttr(c, "data-checked") == "true"}
		}
		items = append(items, it)
	}
	return items
}

func mdDOMRows(n *xhtml.Node, rows *[]mdNode) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != xhtml.ElementNode {
			continue
		}
		switch c.DataAtom {
		case atom.Tr:
			row := mdNode{Type: "tableRow"}
			for k := c.FirstChild; k != nil; k = k.NextSibling {
				if k.Type != xhtml.ElementNode || (k.DataAtom != atom.Td && k.DataAtom != atom.Th) {
					continue
				}
				typ := "tableCell"
				if k.DataAtom == atom.Th {
					typ = "tableHeader"
				}
				row.Content = append(row.Content, mdNode{
					Type:    typ,
					Attrs:   map[string]any{"colspan": mdDOMAttr(k, "colspan")},
					Content: mdDOMBlocks(k, 0),
				})
			}
			*rows = append(*rows, row)
		case atom.Thead, atom.Tbody, atom.Tfoot:
			mdDOMRows(c, rows)
		}
	}
}

func mdDOMBlock(el *xhtml.Node) []mdNode {
	switch el.DataAtom {
	case atom.P:
		return []mdNode{{Type: "paragraph", Content: mdDOMInlineChildren(el)}}
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		level := int(el.Data[1] - '0')
		return []mdNode{{Type: "heading", Attrs: map[string]any{"level": float64(level)}, Content: mdDOMInlineChildren(el)}}
	case atom.Blockquote:
		return []mdNode{{Type: "blockquote", Content: mdDOMBlocks(el, 0)}}
	case atom.Ul:
		if mdDOMAttr(el, "data-type") == "taskList" {
			return []mdNode{{Type: "taskList", Content: mdDOMItems(el, "taskItem")}}
		}
		return []mdNode{{Type: "bulletList", Content: mdDOMItems(el, "listItem")}}
	case atom.Ol:
		start := 1
		if v, err := strconv.Atoi(mdDOMAttr(el, "start")); err == nil {
			start = v
		}
		return []mdNode{{Type: "orderedList", Attrs: map[string]any{"start": float64(start)}, Content: mdDOMItems(el, "listItem")}}
	case atom.Li:
		return []mdNode{{Type: "bulletList", Content: []mdNode{{Type: "listItem", Content: mdDOMBlocks(el, 0)}}}}
	case atom.Pre:
		lang := ""
		for c := el.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == xhtml.ElementNode && c.DataAtom == atom.Code {
				for _, class := range strings.Fields(mdDOMAttr(c, "class")) {
					if strings.HasPrefix(class, "language-") {
						lang = strings.TrimPrefix(class, "language-")
					}
				}
				break
			}
		}
		node := mdNode{Type: "codeBlock", Attrs: map[string]any{"language": lang}}
		if text := mdDOMText(el); text != "" {
			node.Content = []mdNode{{Type: "text", Text: text}}
		}
		return []mdNode{node}
	case atom.Hr:
		return []mdNode{{Type: "horizontalRule"}}
	case atom.Table:
		var rows []mdNode
		mdDOMRows(el, &rows)
		return []mdNode{{Type: "table", Content: rows}}
	case atom.Div:
		switch mdDOMAttr(el, "data-type") {
		case "panel":
			return []mdNode{{Type: "panel", Attrs: map[string]any{"color": mdDOMAttr(el, "data-color")}, Content: mdDOMBlocks(el, 0)}}
		case "toc":
			return []mdNode{{Type: "toc"}}
		}
		return mdDOMBlocks(el, 0)
	case atom.Details:
		node := mdNode{Type: "expand"}
		var summary []mdNode
		for c := el.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == xhtml.ElementNode && c.DataAtom == atom.Summary {
				summary = mdDOMInlineChildren(c)
				break
			}
		}
		node.Content = []mdNode{
			{Type: "expandSummary", Content: summary},
			{Type: "expandBody", Content: mdDOMBlocks(el, atom.Summary)},
		}
		return []mdNode{node}
	case atom.Summary:
		if inline := mdDOMInlineChildren(el); len(inline) > 0 {
			return []mdNode{{Type: "paragraph", Content: inline}}
		}
		return nil
	}
	return mdDOMBlocks(el, 0)
}

func ToStandaloneHTML(title, contentHTML string) string {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	b.WriteString("<title>" + html.EscapeString(title) + "</title>\n<style>")
	b.WriteString(standaloneCSS)
	b.WriteString("</style>\n</head>\n<body>\n<main class=\"kn-content\">\n")
	b.WriteString("<h1 class=\"kn-title\">" + html.EscapeString(title) + "</h1>\n")
	b.WriteString(SanitizeHTML(contentHTML))
	b.WriteString("\n</main>\n</body>\n</html>\n")
	return b.String()
}

const standaloneCSS = `
:root{color-scheme:light dark;--fg:#1f2328;--bg:#ffffff;--muted:#656d76;--line:#d0d7de;--soft:#f6f8fa;--link:#0969da;--code-bg:#0d1117;--code-fg:#e6edf3}
@media (prefers-color-scheme:dark){:root{--fg:#e6edf3;--bg:#0d1117;--muted:#8b949e;--line:#30363d;--soft:#161b22;--link:#58a6ff}}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--fg);font:16px/1.65 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif}
.kn-content{max-width:52rem;margin:0 auto;padding:2.5rem 1.25rem 4rem;overflow-wrap:anywhere}
.kn-title{font-size:2.1rem;line-height:1.25;margin:0 0 1.5rem;padding-bottom:.6rem;border-bottom:1px solid var(--line)}
.kn-content h1,.kn-content h2,.kn-content h3,.kn-content h4,.kn-content h5,.kn-content h6{line-height:1.3;margin:1.8em 0 .6em}
.kn-content h1{font-size:1.9rem}.kn-content h2{font-size:1.5rem}.kn-content h3{font-size:1.25rem}.kn-content h4{font-size:1.05rem}
.kn-content p{margin:.8em 0}
.kn-content a{color:var(--link)}
.kn-content img{max-width:100%;height:auto}
.kn-content hr{border:0;border-top:1px solid var(--line);margin:2em 0}
.kn-content blockquote{margin:1em 0;padding:.2em 1em;border-left:4px solid var(--line);color:var(--muted)}
.kn-content ul,.kn-content ol{padding-left:1.6em}
.kn-content li>p{margin:.25em 0}
.kn-content code{font:.875em ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;background:var(--soft);border-radius:4px;padding:.15em .35em}
.kn-content pre{background:var(--code-bg);color:var(--code-fg);border-radius:8px;padding:1em 1.2em;overflow:auto;line-height:1.5}
.kn-content pre code{background:none;padding:0;color:inherit;font-size:.85em}
.kn-content mark{background:#fff3a3;color:#1f2328;padding:0 .15em;border-radius:2px}
.kn-content table{border-collapse:collapse;width:100%;margin:1.2em 0;display:block;overflow-x:auto}
.kn-content th,.kn-content td{border:1px solid var(--line);padding:.45em .75em;text-align:left;vertical-align:top}
.kn-content th{background:var(--soft);font-weight:600}
.kn-content td>p,.kn-content th>p{margin:.2em 0}
.kn-content [data-align=center]{text-align:center}
.kn-content [data-align=right]{text-align:right}
.kn-content [data-align=justify]{text-align:justify}
.kn-content ul[data-type=taskList]{list-style:none;padding-left:.4em}
.kn-content li[data-type=taskItem]{position:relative;padding-left:1.8em}
.kn-content li[data-type=taskItem]::before{content:"\2610";position:absolute;left:0;top:0;font-size:1.1em;line-height:1.5}
.kn-content li[data-type=taskItem][data-checked=true]::before{content:"\2611"}
.kn-content li[data-type=taskItem][data-checked=true]>p{color:var(--muted);text-decoration:line-through}
.kn-content div[data-type=panel]{--panel:#0969da;--panel-bg:#ddf4ff;margin:1.2em 0;padding:.4em 1em;border-left:4px solid var(--panel);border-radius:6px;background:var(--panel-bg);color:#1f2328}
.kn-content div[data-type=panel][data-color=note]{--panel:#8250df;--panel-bg:#fbefff}
.kn-content div[data-type=panel][data-color=success]{--panel:#1a7f37;--panel-bg:#dafbe1}
.kn-content div[data-type=panel][data-color=warning]{--panel:#bf8700;--panel-bg:#fff8c5}
.kn-content div[data-type=panel][data-color=error]{--panel:#cf222e;--panel-bg:#ffebe9}
.kn-content details[data-type=expand]{margin:1em 0;border:1px solid var(--line);border-radius:6px;padding:.2em 1em}
.kn-content details[data-type=expand]>summary{cursor:pointer;font-weight:600;padding:.4em 0}
.kn-content span[data-type=status]{display:inline-block;padding:.05em .55em;border-radius:4px;font-size:.75em;font-weight:700;letter-spacing:.03em;text-transform:uppercase;vertical-align:middle;background:#e6e8eb;color:#3b4350}
.kn-content span[data-type=status][data-color=blue]{background:#cfe3ff;color:#0a3d91}
.kn-content span[data-type=status][data-color=green]{background:#c9f0d3;color:#14532d}
.kn-content span[data-type=status][data-color=yellow]{background:#fdeeb0;color:#6b4e00}
.kn-content span[data-type=status][data-color=red]{background:#fcd5d5;color:#8a1313}
.kn-content span[data-type=status][data-color=purple]{background:#e6d9fb;color:#4a2a8c}
.kn-content span[data-type=mention]{background:var(--soft);border-radius:4px;padding:0 .3em;color:var(--link)}
.kn-content span[data-type=comment]{background:#fff3a3;color:#1f2328}
.kn-content div[data-type=toc]{display:none}
@media print{.kn-content{max-width:none;padding:0}.kn-content pre{white-space:pre-wrap}}
`
