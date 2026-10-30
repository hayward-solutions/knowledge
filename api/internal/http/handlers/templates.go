package handlers

import (
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/content"
	"github.com/hayward-solutions/knowledge/api/internal/domain"
	mw "github.com/hayward-solutions/knowledge/api/internal/http/middleware"
	"github.com/hayward-solutions/knowledge/api/internal/store"
)

const (
	templateMaxName        = 120
	templateMaxDescription = 2000
)

type TemplateHandler struct {
	Deps
}

func (h *TemplateHandler) Routes(r chi.Router) {
	r.With(mw.RequireAuth(h.Store)).Get("/templates", h.List)
	r.With(mw.SpaceByKey(h.Store, domain.PermViewSpace)).Get("/spaces/{key}/templates", h.ListSpace)
	r.With(mw.SpaceByKey(h.Store, domain.PermManageTemplates)).Post("/spaces/{key}/templates", h.CreateSpace)
	r.With(mw.RequireAuth(h.Store), mw.RequireSystemAdmin).Post("/admin/templates", h.CreateGlobal)
	r.With(mw.RequireAuth(h.Store)).Get("/templates/{templateID}", h.Get)
	r.With(mw.RequireAuth(h.Store)).Patch("/templates/{templateID}", h.Update)
	r.With(mw.RequireAuth(h.Store)).Delete("/templates/{templateID}", h.Delete)
}

func (h *TemplateHandler) List(w http.ResponseWriter, r *http.Request) {
	u := mw.UserFrom(r.Context())
	if u.IsSystemAdmin {
		if _, err := h.Store.SeedTemplatesIfEmpty(r.Context(), u.ID, templateStarters()); err != nil {
			internalErr(w, r, err, "failed to seed templates")
			return
		}
	}
	out, err := h.Store.ListVisibleTemplates(r.Context(), u.ID, u.IsSystemAdmin)
	if err != nil {
		internalErr(w, r, err, "failed to list templates")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *TemplateHandler) ListSpace(w http.ResponseWriter, r *http.Request) {
	sc := mw.SpaceFrom(r.Context())
	out, err := h.Store.ListSpaceTemplates(r.Context(), sc.ID)
	if err != nil {
		internalErr(w, r, err, "failed to list templates")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type templateInputBody struct {
	Name        *string         `json:"name"`
	Description *string         `json:"description"`
	ContentJSON json.RawMessage `json:"content_json"`
	ContentHTML *string         `json:"content_html"`
}

func templateDecode(w http.ResponseWriter, r *http.Request) (templateInputBody, bool) {
	var body templateInputBody
	if err := readJSON(r, &body); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpErr(w, http.StatusRequestEntityTooLarge, "request body too large")
			return body, false
		}
		httpErr(w, http.StatusBadRequest, "bad body")
		return body, false
	}
	return body, true
}

func templateValidName(name string) bool {
	n := utf8.RuneCountInString(name)
	return n >= 1 && n <= templateMaxName
}

func templateValidDescription(desc string) bool {
	return utf8.RuneCountInString(desc) <= templateMaxDescription
}

func templateNormalizeJSON(raw json.RawMessage) (json.RawMessage, bool) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, true
	}
	if trimmed[0] != '{' || !json.Valid([]byte(trimmed)) {
		return nil, false
	}
	return json.RawMessage(trimmed), true
}

func (h *TemplateHandler) templateCreate(w http.ResponseWriter, r *http.Request, spaceID *uuid.UUID) {
	u := mw.UserFrom(r.Context())
	body, ok := templateDecode(w, r)
	if !ok {
		return
	}
	name := ""
	if body.Name != nil {
		name = strings.TrimSpace(*body.Name)
	}
	if !templateValidName(name) {
		httpErr(w, http.StatusBadRequest, "name must be 1-"+strconv.Itoa(templateMaxName)+" characters")
		return
	}
	desc := ""
	if body.Description != nil {
		desc = strings.TrimSpace(*body.Description)
	}
	if !templateValidDescription(desc) {
		httpErr(w, http.StatusBadRequest, "description too long")
		return
	}
	raw, ok := templateNormalizeJSON(body.ContentJSON)
	if !ok {
		httpErr(w, http.StatusBadRequest, "content_json must be an object or null")
		return
	}
	htmlIn := ""
	if body.ContentHTML != nil {
		htmlIn = content.SanitizeHTML(*body.ContentHTML)
	}
	tpl, err := h.Store.CreateTemplate(r.Context(), store.NewTemplate{
		SpaceID:     spaceID,
		Name:        name,
		Description: desc,
		ContentJSON: raw,
		ContentHTML: htmlIn,
		CreatedBy:   u.ID,
	})
	if err != nil {
		internalErr(w, r, err, "failed to create template")
		return
	}
	writeJSON(w, http.StatusCreated, tpl)
}

func (h *TemplateHandler) CreateSpace(w http.ResponseWriter, r *http.Request) {
	sc := mw.SpaceFrom(r.Context())
	id := sc.ID
	h.templateCreate(w, r, &id)
}

func (h *TemplateHandler) CreateGlobal(w http.ResponseWriter, r *http.Request) {
	h.templateCreate(w, r, nil)
}

func templateEffectiveRole(u *domain.User, a store.SpaceAccess) (domain.Role, bool) {
	switch {
	case u.IsSystemAdmin:
		return domain.RoleAdmin, true
	case a.MemberRole != nil && a.MemberRole.Valid():
		return *a.MemberRole, true
	case a.Visibility == domain.VisibilityInternal:
		return domain.RoleViewer, true
	}
	return "", false
}

type templateAccess struct {
	tpl      store.TemplateView
	role     domain.Role
	readOnly bool
}

func (h *TemplateHandler) templateLoad(w http.ResponseWriter, r *http.Request) (templateAccess, bool) {
	u := mw.UserFrom(r.Context())
	id, err := urlUUID(r, "templateID")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "invalid id")
		return templateAccess{}, false
	}
	tpl, err := h.Store.GetTemplate(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpErr(w, http.StatusNotFound, "not found")
			return templateAccess{}, false
		}
		internalErr(w, r, err, "failed to load template")
		return templateAccess{}, false
	}
	if tpl.SpaceID == nil {
		role := domain.Role("")
		if u.IsSystemAdmin {
			role = domain.RoleAdmin
		}
		return templateAccess{tpl: tpl, role: role}, true
	}
	access, err := h.Store.GetSpaceAccessByID(r.Context(), *tpl.SpaceID, u.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpErr(w, http.StatusNotFound, "not found")
			return templateAccess{}, false
		}
		internalErr(w, r, err, "failed to load template")
		return templateAccess{}, false
	}
	if access.DeletedAt != nil && !u.IsSystemAdmin {
		httpErr(w, http.StatusNotFound, "not found")
		return templateAccess{}, false
	}
	role, ok := templateEffectiveRole(u, access)
	if !ok {
		httpErr(w, http.StatusNotFound, "not found")
		return templateAccess{}, false
	}
	return templateAccess{tpl: tpl, role: role, readOnly: access.ArchivedAt != nil || access.DeletedAt != nil}, true
}

func (h *TemplateHandler) templateLoadForManage(w http.ResponseWriter, r *http.Request) (templateAccess, bool) {
	a, ok := h.templateLoad(w, r)
	if !ok {
		return a, false
	}
	if a.tpl.SpaceID == nil {
		if !mw.UserFrom(r.Context()).IsSystemAdmin {
			httpErr(w, http.StatusForbidden, "forbidden")
			return a, false
		}
		return a, true
	}
	if !domain.RoleAllows(a.role, domain.PermManageTemplates) {
		httpErr(w, http.StatusForbidden, "forbidden")
		return a, false
	}
	if a.readOnly && a.role != domain.RoleAdmin {
		httpErr(w, http.StatusForbidden, "space is read-only")
		return a, false
	}
	return a, true
}

func (h *TemplateHandler) Get(w http.ResponseWriter, r *http.Request) {
	a, ok := h.templateLoad(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, a.tpl)
}

func (h *TemplateHandler) Update(w http.ResponseWriter, r *http.Request) {
	a, ok := h.templateLoadForManage(w, r)
	if !ok {
		return
	}
	body, ok := templateDecode(w, r)
	if !ok {
		return
	}
	patch := store.TemplatePatch{}
	if body.Name != nil {
		name := strings.TrimSpace(*body.Name)
		if !templateValidName(name) {
			httpErr(w, http.StatusBadRequest, "name must be 1-"+strconv.Itoa(templateMaxName)+" characters")
			return
		}
		patch.Name = &name
	}
	if body.Description != nil {
		desc := strings.TrimSpace(*body.Description)
		if !templateValidDescription(desc) {
			httpErr(w, http.StatusBadRequest, "description too long")
			return
		}
		patch.Description = &desc
	}
	if len(body.ContentJSON) > 0 {
		raw, ok := templateNormalizeJSON(body.ContentJSON)
		if !ok {
			httpErr(w, http.StatusBadRequest, "content_json must be an object or null")
			return
		}
		patch.ContentJSON = raw
		patch.SetContentJSON = true
	}
	if body.ContentHTML != nil {
		clean := content.SanitizeHTML(*body.ContentHTML)
		patch.ContentHTML = &clean
	}
	tpl, err := h.Store.UpdateTemplate(r.Context(), a.tpl.ID, patch)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpErr(w, http.StatusNotFound, "not found")
			return
		}
		internalErr(w, r, err, "failed to update template")
		return
	}
	writeJSON(w, http.StatusOK, tpl)
}

func (h *TemplateHandler) Delete(w http.ResponseWriter, r *http.Request) {
	a, ok := h.templateLoadForManage(w, r)
	if !ok {
		return
	}
	if err := h.Store.DeleteTemplate(r.Context(), a.tpl.ID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpErr(w, http.StatusNotFound, "not found")
			return
		}
		internalErr(w, r, err, "failed to delete template")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type templateNode struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content []templateNode `json:"content,omitempty"`
	Text    string         `json:"text,omitempty"`
}

func templateText(s string) templateNode { return templateNode{Type: "text", Text: s} }

func templatePara(s string) templateNode {
	if s == "" {
		return templateNode{Type: "paragraph"}
	}
	return templateNode{Type: "paragraph", Content: []templateNode{templateText(s)}}
}

func templateHeading(level int, s string) templateNode {
	return templateNode{Type: "heading", Attrs: map[string]any{"level": level}, Content: []templateNode{templateText(s)}}
}

func templatePanel(color string, paras ...string) templateNode {
	n := templateNode{Type: "panel", Attrs: map[string]any{"color": color}}
	for _, p := range paras {
		n.Content = append(n.Content, templatePara(p))
	}
	return n
}

func templateList(kind string, items ...string) templateNode {
	n := templateNode{Type: kind}
	for _, it := range items {
		n.Content = append(n.Content, templateNode{Type: "listItem", Content: []templateNode{templatePara(it)}})
	}
	return n
}

func templateTasks(items ...string) templateNode {
	n := templateNode{Type: "taskList"}
	for _, it := range items {
		n.Content = append(n.Content, templateNode{
			Type:    "taskItem",
			Attrs:   map[string]any{"checked": false},
			Content: []templateNode{templatePara(it)},
		})
	}
	return n
}

func templateTable(header []string, rows ...[]string) templateNode {
	cell := func(kind, s string) templateNode {
		return templateNode{
			Type:    kind,
			Attrs:   map[string]any{"colspan": 1, "rowspan": 1, "colwidth": nil},
			Content: []templateNode{templatePara(s)},
		}
	}
	n := templateNode{Type: "table"}
	head := templateNode{Type: "tableRow"}
	for _, c := range header {
		head.Content = append(head.Content, cell("tableHeader", c))
	}
	n.Content = append(n.Content, head)
	for _, row := range rows {
		tr := templateNode{Type: "tableRow"}
		for _, c := range row {
			tr.Content = append(tr.Content, cell("tableCell", c))
		}
		n.Content = append(n.Content, tr)
	}
	return n
}

func templateDoc(blocks ...templateNode) templateNode {
	return templateNode{Type: "doc", Content: blocks}
}

func templateRender(n templateNode) string {
	var b strings.Builder
	templateWrite(&b, n)
	return b.String()
}

func templateWriteChildren(b *strings.Builder, n templateNode) {
	for _, c := range n.Content {
		templateWrite(b, c)
	}
}

func templateWrite(b *strings.Builder, n templateNode) {
	switch n.Type {
	case "doc":
		templateWriteChildren(b, n)
	case "text":
		b.WriteString(html.EscapeString(n.Text))
	case "paragraph":
		b.WriteString("<p>")
		templateWriteChildren(b, n)
		b.WriteString("</p>")
	case "heading":
		tag := "h" + strconv.Itoa(n.Attrs["level"].(int))
		b.WriteString("<" + tag + ">")
		templateWriteChildren(b, n)
		b.WriteString("</" + tag + ">")
	case "bulletList":
		b.WriteString("<ul>")
		templateWriteChildren(b, n)
		b.WriteString("</ul>")
	case "orderedList":
		b.WriteString("<ol>")
		templateWriteChildren(b, n)
		b.WriteString("</ol>")
	case "listItem":
		b.WriteString("<li>")
		templateWriteChildren(b, n)
		b.WriteString("</li>")
	case "taskList":
		b.WriteString(`<ul data-type="taskList">`)
		templateWriteChildren(b, n)
		b.WriteString("</ul>")
	case "taskItem":
		checked := "false"
		if v, _ := n.Attrs["checked"].(bool); v {
			checked = "true"
		}
		b.WriteString(`<li data-type="taskItem" data-checked="` + checked + `">`)
		templateWriteChildren(b, n)
		b.WriteString("</li>")
	case "panel":
		b.WriteString(`<div data-type="panel" data-color="` + html.EscapeString(n.Attrs["color"].(string)) + `">`)
		templateWriteChildren(b, n)
		b.WriteString("</div>")
	case "toc":
		b.WriteString(`<div data-type="toc"></div>`)
	case "table":
		b.WriteString("<table><tbody>")
		templateWriteChildren(b, n)
		b.WriteString("</tbody></table>")
	case "tableRow":
		b.WriteString("<tr>")
		templateWriteChildren(b, n)
		b.WriteString("</tr>")
	case "tableHeader":
		b.WriteString(`<th colspan="1" rowspan="1">`)
		templateWriteChildren(b, n)
		b.WriteString("</th>")
	case "tableCell":
		b.WriteString(`<td colspan="1" rowspan="1">`)
		templateWriteChildren(b, n)
		b.WriteString("</td>")
	}
}

func templateSeed(name, description string, doc templateNode) store.TemplateSeed {
	raw, err := json.Marshal(doc)
	if err != nil {
		panic(err)
	}
	return store.TemplateSeed{
		Name:        name,
		Description: description,
		ContentJSON: raw,
		ContentHTML: content.SanitizeHTML(templateRender(doc)),
	}
}

func templateStarters() []store.TemplateSeed {
	return []store.TemplateSeed{
		templateSeed("Meeting notes", "Agenda, discussion, decisions and action items for a meeting.", templateDoc(
			templatePanel("info", "Date: ", "Attendees: ", "Facilitator: "),
			templateHeading(2, "Agenda"),
			templateList("orderedList", "Topic one", "Topic two", "Topic three"),
			templateHeading(2, "Discussion notes"),
			templatePara(""),
			templateHeading(2, "Decisions"),
			templateList("bulletList", "Decision"),
			templateHeading(2, "Action items"),
			templateTable([]string{"Action", "Owner", "Due"}, []string{"", "", ""}, []string{"", "", ""}),
			templateHeading(2, "Follow-ups"),
			templateTasks("Send the notes to attendees", "Schedule the next meeting"),
		)),
		templateSeed("Decision record", "Capture a decision, the options considered and its consequences.", templateDoc(
			templatePanel("note", "Status: Proposed", "Deciders: ", "Date: "),
			templateHeading(2, "Context"),
			templatePara("What problem are we solving and what constraints apply?"),
			templateHeading(2, "Decision"),
			templatePara("We will..."),
			templateHeading(2, "Options considered"),
			templateTable([]string{"Option", "Pros", "Cons"},
				[]string{"Option A", "", ""},
				[]string{"Option B", "", ""},
				[]string{"Do nothing", "", ""}),
			templateHeading(2, "Consequences"),
			templateList("bulletList", "What becomes easier", "What becomes harder"),
			templatePanel("warning", "Revisit this decision if the assumptions above stop holding."),
		)),
		templateSeed("How-to guide", "Step-by-step instructions with prerequisites and troubleshooting.", templateDoc(
			templateNode{Type: "toc"},
			templatePanel("info", "Who this guide is for and what they will be able to do afterwards."),
			templateHeading(2, "Prerequisites"),
			templateTasks("Access to the system", "Required tools installed"),
			templateHeading(2, "Steps"),
			templateList("orderedList", "First step", "Second step", "Third step"),
			templateHeading(2, "Troubleshooting"),
			templateTable([]string{"Symptom", "Fix"}, []string{"", ""}, []string{"", ""}),
			templateHeading(2, "Related pages"),
			templateList("bulletList", "Link to a related page"),
		)),
		templateSeed("Retrospective", "Look back on a sprint or project: what went well, what did not, what to change.", templateDoc(
			templatePanel("info", "Period: ", "Participants: "),
			templateHeading(2, "What went well"),
			templateList("bulletList", "Something that worked"),
			templateHeading(2, "What did not go well"),
			templateList("bulletList", "Something that got in the way"),
			templateHeading(2, "What we will try next"),
			templateTable([]string{"Experiment", "Owner", "Success measure"}, []string{"", "", ""}),
			templateHeading(2, "Action items"),
			templateTasks("Action item"),
			templatePanel("success", "Shout-outs and thanks."),
		)),
	}
}
