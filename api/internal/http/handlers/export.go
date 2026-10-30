package handlers

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/config"
	"github.com/hayward-solutions/knowledge/api/internal/content"
	"github.com/hayward-solutions/knowledge/api/internal/domain"
	mw "github.com/hayward-solutions/knowledge/api/internal/http/middleware"
	"github.com/hayward-solutions/knowledge/api/internal/store"
)

const (
	exportMaxImportFiles   = 200
	exportImportTotalScale = 8
	exportMultipartSlack   = 1 << 20
)

type ExportHandler struct {
	Deps
}

func (h *ExportHandler) Routes(r chi.Router) {
	r.With(mw.SpaceByPage(h.Store, domain.PermViewSpace)).Get("/pages/{pageID}/export", h.ExportPage)
	r.With(mw.SpaceByKey(h.Store, domain.PermViewSpace)).Get("/spaces/{key}/export", h.ExportSpace)
	r.With(mw.SpaceByKey(h.Store, domain.PermEditPages)).Post("/spaces/{key}/import", h.Import)
}

func exportDisposition(name string) string {
	ascii := true
	var b strings.Builder
	for _, r := range name {
		switch {
		case r > unicode.MaxASCII:
			ascii = false
			b.WriteByte('_')
		case r < 0x20 || r == 0x7f || r == '"' || r == '\\' || r == '/':
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	out := `attachment; filename="` + b.String() + `"`
	if !ascii {
		out += "; filename*=UTF-8''" + url.PathEscape(name)
	}
	return out
}

func exportFileBase(slug, title string) string {
	slug = strings.TrimSpace(slug)
	clean := slug != ""
	for _, r := range slug {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			clean = false
			break
		}
	}
	if clean {
		return slug
	}
	if slug == "" {
		slug = title
	}
	return store.SlugFromTitle(slug)
}

func exportPageMarkdown(title string, contentJSON []byte, contentHTML string) string {
	body, err := content.ToMarkdown(contentJSON)
	if err != nil || (strings.TrimSpace(body) == "" && strings.TrimSpace(contentHTML) != "") {
		if alt, aerr := content.ToMarkdownFromHTML(contentHTML); aerr == nil {
			body = alt
		}
	}
	out := content.ToMarkdownTitle(title) + "\n"
	if body != "" {
		out += "\n" + body + "\n"
	}
	return out
}

func (h *ExportHandler) ExportPage(w http.ResponseWriter, r *http.Request) {
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "md"
	}
	if format != "md" && format != "html" {
		httpErr(w, http.StatusBadRequest, "format must be md or html")
		return
	}
	req, ok := pageLoad(w, r, h.Store, false)
	if !ok {
		return
	}
	page := req.page
	base := exportFileBase(page.Slug, page.Title)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	switch format {
	case "html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'unsafe-inline'; img-src * data:")
		w.Header().Set("Content-Disposition", exportDisposition(base+".html"))
		_, _ = io.WriteString(w, content.ToStandaloneHTML(page.Title, page.ContentHTML))
	default:
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", exportDisposition(base+".md"))
		_, _ = io.WriteString(w, exportPageMarkdown(page.Title, page.ContentJSON, page.ContentHTML))
	}
}

type exportEntry struct {
	path  string
	depth int
	page  store.ExportPage
}

func exportDedupe(used map[string]bool, name string) string {
	candidate := name
	for n := 2; used[strings.ToLower(candidate)]; n++ {
		candidate = fmt.Sprintf("%s-%d", name, n)
	}
	used[strings.ToLower(candidate)] = true
	return candidate
}

func exportLayout(pages []store.ExportPage) []exportEntry {
	byID := make(map[uuid.UUID]bool, len(pages))
	for _, p := range pages {
		byID[p.ID] = true
	}
	children := map[uuid.UUID][]store.ExportPage{}
	var roots []store.ExportPage
	for _, p := range pages {
		if p.ParentID != nil && byID[*p.ParentID] {
			children[*p.ParentID] = append(children[*p.ParentID], p)
		} else {
			roots = append(roots, p)
		}
	}
	var entries []exportEntry
	visited := map[uuid.UUID]bool{}
	var walk func(list []store.ExportPage, dir string, used map[string]bool, depth int)
	walk = func(list []store.ExportPage, dir string, used map[string]bool, depth int) {
		for _, p := range list {
			if visited[p.ID] {
				continue
			}
			visited[p.ID] = true
			name := exportDedupe(used, exportFileBase(p.Slug, p.Title))
			kids := children[p.ID]
			if len(kids) == 0 {
				entries = append(entries, exportEntry{path: dir + name + ".md", depth: depth, page: p})
				continue
			}
			folder := dir + name + "/"
			entries = append(entries, exportEntry{path: folder + "index.md", depth: depth, page: p})
			walk(kids, folder, map[string]bool{"index": true}, depth+1)
		}
	}
	walk(roots, "", map[string]bool{"readme": true}, 0)
	return entries
}

func exportREADME(spaceName string, entries []exportEntry) string {
	var b strings.Builder
	b.WriteString(content.ToMarkdownTitle(spaceName + " export"))
	b.WriteString("\n\n")
	esc := strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]")
	for _, e := range entries {
		segs := strings.Split(e.path, "/")
		for i := range segs {
			segs[i] = url.PathEscape(segs[i])
		}
		b.WriteString(strings.Repeat("  ", e.depth))
		b.WriteString("- [" + esc.Replace(e.page.Title) + "](" + strings.Join(segs, "/") + ")\n")
	}
	return b.String()
}

func (h *ExportHandler) ExportSpace(w http.ResponseWriter, r *http.Request) {
	sc := mw.SpaceFrom(r.Context())
	if sc == nil {
		httpErr(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	ctx := r.Context()
	view, err := h.Store.GetSpaceView(ctx, sc.ID)
	if err != nil {
		internalErr(w, r, err, "failed to export space")
		return
	}
	pages, err := h.Store.ListSpacePagesForExport(ctx, sc.ID)
	if err != nil {
		internalErr(w, r, err, "failed to export space")
		return
	}
	entries := exportLayout(pages)

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", exportDisposition(sc.Key+"-export.zip"))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	zw := zip.NewWriter(w)
	write := func(hdr *zip.FileHeader, body string) bool {
		hdr.Method = zip.Deflate
		fw, err := zw.CreateHeader(hdr)
		if err == nil {
			_, err = io.WriteString(fw, body)
		}
		if err != nil {
			log.Printf("export space %s: %v", sc.Key, err)
			return false
		}
		return true
	}
	if !write(&zip.FileHeader{Name: "README.md"}, exportREADME(view.Name, entries)) {
		return
	}
	for _, e := range entries {
		body := exportPageMarkdown(e.page.Title, e.page.ContentJSON, e.page.ContentHTML)
		if !write(&zip.FileHeader{Name: e.path, Modified: e.page.UpdatedAt}, body) {
			return
		}
	}
	if err := zw.Close(); err != nil {
		log.Printf("export space %s: %v", sc.Key, err)
	}
}

type exportUpload struct {
	name string
	data []byte
}

func exportPartFilename(header map[string][]string) string {
	cd := ""
	if v := header["Content-Disposition"]; len(v) > 0 {
		cd = v[0]
	}
	_, params, err := mime.ParseMediaType(cd)
	if err != nil {
		return ""
	}
	return params["filename"]
}

func exportReadErr(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		httpErr(w, http.StatusRequestEntityTooLarge, "upload too large")
		return
	}
	httpErr(w, http.StatusBadRequest, "malformed multipart body")
}

func exportSplitPath(name string) (dirs []string, base string, ok bool) {
	name = strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(name, "/") || (len(name) > 1 && name[1] == ':') {
		return nil, "", false
	}
	var segs []string
	for _, s := range strings.Split(name, "/") {
		s = strings.TrimSpace(s)
		switch s {
		case "", ".":
			continue
		case "..":
			return nil, "", false
		}
		segs = append(segs, s)
	}
	if len(segs) == 0 {
		return nil, "", false
	}
	return segs[:len(segs)-1], segs[len(segs)-1], true
}

func exportIsMarkdown(base string) bool {
	lower := strings.ToLower(base)
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown")
}

func exportIsIndex(base string) bool {
	lower := strings.ToLower(base)
	return lower == "index.md" || lower == "index.markdown"
}

func exportClampTitle(title, fallback string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = fallback
	}
	if title == "" {
		title = pageDefaultTitle
	}
	if utf8.RuneCountInString(title) > pageMaxTitleRunes {
		title = string([]rune(title)[:pageMaxTitleRunes])
	}
	return title
}

type exportPlanner struct {
	uploads []exportUpload
	dirs    [][]string
	bases   []string
	index   map[string]int
	folders map[string]int
	plan    []store.ImportPage
}

func (p *exportPlanner) convert(i int) (html, title string, err error) {
	md := strings.ToValidUTF8(string(p.uploads[i].data), "�")
	return content.FromMarkdown(md)
}

func (p *exportPlanner) page(i, parent int, fallback, message string) error {
	html, title, err := p.convert(i)
	if err != nil {
		return err
	}
	p.plan = append(p.plan, store.ImportPage{
		Parent:      parent,
		Title:       exportClampTitle(title, fallback),
		ContentHTML: html,
		ContentText: content.ExtractTextFromHTML(html),
		Message:     message,
	})
	return nil
}

func (p *exportPlanner) folder(dirs []string) (int, error) {
	if len(dirs) == 0 {
		return -1, nil
	}
	key := strings.Join(dirs, "/")
	if idx, ok := p.folders[key]; ok {
		return idx, nil
	}
	parent, err := p.folder(dirs[:len(dirs)-1])
	if err != nil {
		return 0, err
	}
	name := content.TitleFromFolder(dirs[len(dirs)-1])
	if i, ok := p.index[key]; ok {
		if err := p.page(i, parent, name, "Imported from "+p.uploads[i].name); err != nil {
			return 0, err
		}
	} else {
		p.plan = append(p.plan, store.ImportPage{
			Parent:  parent,
			Title:   exportClampTitle(name, ""),
			Message: "Imported from folder " + key,
		})
	}
	idx := len(p.plan) - 1
	p.folders[key] = idx
	return idx, nil
}

func (h *ExportHandler) Import(w http.ResponseWriter, r *http.Request) {
	u := mw.UserFrom(r.Context())
	sc := mw.SpaceFrom(r.Context())
	if u == nil || sc == nil {
		httpErr(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	maxFile := config.DefaultAttachmentMaxBytes
	if h.Config != nil && h.Config.AttachmentMaxBytes > 0 {
		maxFile = h.Config.AttachmentMaxBytes
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFile*exportImportTotalScale+exportMultipartSlack)
	mr, err := r.MultipartReader()
	if err != nil {
		httpErr(w, http.StatusBadRequest, "expected multipart/form-data")
		return
	}
	var uploads []exportUpload
	parentRaw := ""
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			exportReadErr(w, err)
			return
		}
		switch part.FormName() {
		case "files":
			name := exportPartFilename(part.Header)
			if strings.TrimSpace(name) == "" {
				httpErr(w, http.StatusBadRequest, "each files part needs a filename")
				return
			}
			if len(uploads) >= exportMaxImportFiles {
				httpErr(w, http.StatusBadRequest, fmt.Sprintf("too many files (max %d)", exportMaxImportFiles))
				return
			}
			data, err := io.ReadAll(io.LimitReader(part, maxFile+1))
			if err != nil {
				exportReadErr(w, err)
				return
			}
			if int64(len(data)) > maxFile {
				httpErr(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("file too large: %s", name))
				return
			}
			uploads = append(uploads, exportUpload{name: name, data: data})
		case "parent_id":
			b, err := io.ReadAll(io.LimitReader(part, 128))
			if err != nil {
				exportReadErr(w, err)
				return
			}
			parentRaw = strings.TrimSpace(string(b))
		}
	}
	if len(uploads) == 0 {
		httpErr(w, http.StatusBadRequest, "no files provided")
		return
	}

	planner := &exportPlanner{
		uploads: uploads,
		dirs:    make([][]string, len(uploads)),
		bases:   make([]string, len(uploads)),
		index:   map[string]int{},
		folders: map[string]int{},
	}
	for i, up := range uploads {
		dirs, base, ok := exportSplitPath(up.name)
		if !ok {
			httpErr(w, http.StatusBadRequest, "invalid file path: "+up.name)
			return
		}
		if !exportIsMarkdown(base) {
			httpErr(w, http.StatusBadRequest, "only Markdown (.md) files can be imported: "+up.name)
			return
		}
		planner.dirs[i], planner.bases[i] = dirs, base
		if len(dirs) > 0 && exportIsIndex(base) {
			key := strings.Join(dirs, "/")
			if _, exists := planner.index[key]; !exists {
				planner.index[key] = i
			}
		}
	}

	ctx := r.Context()
	var baseParent *uuid.UUID
	if parentRaw != "" {
		pid, err := uuid.Parse(parentRaw)
		if err != nil {
			httpErr(w, http.StatusBadRequest, "invalid parent_id")
			return
		}
		parent, err := h.Store.GetPage(ctx, pid)
		if err != nil {
			pageStoreErr(w, r, err, "failed to load parent page")
			return
		}
		if parent.SpaceID != sc.ID || parent.Status == domain.PageStatusTrashed || !store.IsPageVisible(parent, u.ID, sc.Role) {
			httpErr(w, http.StatusNotFound, "parent page not found")
			return
		}
		baseParent = &pid
	} else {
		home, err := h.Store.PageSpaceHomeID(ctx, sc.ID)
		if err != nil {
			pageStoreErr(w, r, err, "failed to load space")
			return
		}
		baseParent = home
	}

	for i := range uploads {
		dirs, base := planner.dirs[i], planner.bases[i]
		if len(dirs) > 0 && exportIsIndex(base) {
			if idx, ok := planner.index[strings.Join(dirs, "/")]; ok && idx == i {
				if _, err := planner.folder(dirs); err != nil {
					internalErr(w, r, err, "failed to convert markdown")
					return
				}
				continue
			}
		}
		parent, err := planner.folder(dirs)
		if err != nil {
			internalErr(w, r, err, "failed to convert markdown")
			return
		}
		if err := planner.page(i, parent, content.TitleFromFilename(base), "Imported from "+uploads[i].name); err != nil {
			internalErr(w, r, err, "failed to convert markdown")
			return
		}
	}

	created, err := h.Store.ImportPages(ctx, sc.ID, baseParent, u.ID, planner.plan)
	if err != nil {
		pageStoreErr(w, r, err, "failed to import pages")
		return
	}

	hasChildren := map[uuid.UUID]bool{}
	for _, p := range created {
		if p.ParentID != nil {
			hasChildren[*p.ParentID] = true
		}
	}
	summaries := make([]store.PageSummary, 0, len(created))
	for _, p := range created {
		summaries = append(summaries, store.PageSummary{
			ID:          p.ID,
			Title:       p.Title,
			Slug:        p.Slug,
			ParentID:    p.ParentID,
			Position:    p.Position,
			Status:      p.Status,
			HasChildren: hasChildren[p.ID],
		})
		h.Bus.Publish(domain.PagePublished{
			PageID:           p.ID,
			SpaceID:          p.SpaceID,
			ActorID:          u.ID,
			Version:          p.Version,
			MentionedUserIDs: []uuid.UUID{},
		})
	}
	writeJSON(w, http.StatusCreated, map[string]any{"pages": summaries})
}
