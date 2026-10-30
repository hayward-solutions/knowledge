package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	mw "github.com/hayward-solutions/knowledge/api/internal/http/middleware"
	"github.com/hayward-solutions/knowledge/api/internal/storage"
	"github.com/hayward-solutions/knowledge/api/internal/store"
)

const (
	attachmentDefaultMaxBytes = 25 << 20
	attachmentMultipartSlack  = 1 << 20
	attachmentMaxFilename     = 255
	attachmentSniffLen        = 512
)

var attachmentInlineTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
	"image/avif": true,
}

type AttachmentHandler struct {
	Deps
}

func (h *AttachmentHandler) Routes(r chi.Router) {
	r.With(mw.SpaceByPage(h.Store, domain.PermManageAttachments)).Post("/pages/{pageID}/attachments", h.Upload)
	r.With(mw.SpaceByPage(h.Store, domain.PermViewSpace)).Get("/pages/{pageID}/attachments", h.List)
	r.With(mw.SpaceByResolver(h.Store, h.resolveAttachmentSpace, domain.PermViewSpace)).Get("/attachments/{attachmentID}", h.Download)
	r.With(mw.SpaceByResolver(h.Store, h.resolveAttachmentSpace, domain.PermViewSpace)).Get("/attachments/{attachmentID}/content", h.Content)
	r.With(mw.SpaceByResolver(h.Store, h.resolveAttachmentSpace, domain.PermManageAttachments)).Delete("/attachments/{attachmentID}", h.Delete)
}

func (h *AttachmentHandler) resolveAttachmentSpace(r *http.Request) (uuid.UUID, error) {
	id, err := mw.URLUUID(r, "attachmentID")
	if err != nil {
		return uuid.Nil, err
	}
	return h.Store.GetAttachmentSpaceID(r.Context(), id)
}

func (h *AttachmentHandler) maxBytes() int64 {
	if h.Config != nil && h.Config.AttachmentMaxBytes > 0 {
		return h.Config.AttachmentMaxBytes
	}
	return attachmentDefaultMaxBytes
}

func attachmentPageVisible(status domain.PageStatus, createdBy uuid.UUID, user *domain.User, sc *mw.SpaceCtx) bool {
	switch status {
	case domain.PageStatusDraft:
		return sc.Role == domain.RoleAdmin || createdBy == user.ID
	case domain.PageStatusTrashed:
		return sc.Role.AtLeast(domain.RoleEditor)
	}
	return true
}

func attachmentPageTrashed(w http.ResponseWriter) {
	httpErrCode(w, http.StatusConflict, "page_trashed", "page is trashed")
}

func attachmentPurgeObjects(ctx context.Context, st storage.ObjectStore, keys []string) {
	if st == nil {
		return
	}
	for _, key := range keys {
		if err := st.Delete(ctx, key); err != nil {
			log.Printf("attachment purge %s: %v", key, err)
		}
	}
}

func (h *AttachmentHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if h.Storage == nil {
		httpErr(w, http.StatusServiceUnavailable, "object storage not configured")
		return
	}
	pageID, err := mw.URLUUID(r, "pageID")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	user := mw.UserFrom(r.Context())
	sc := mw.SpaceFrom(r.Context())
	meta, err := h.Store.GetAttachmentPageMeta(r.Context(), pageID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpErr(w, http.StatusNotFound, "not found")
			return
		}
		internalErr(w, r, err, "failed to load page")
		return
	}
	if !attachmentPageVisible(meta.Status, meta.CreatedBy, user, sc) {
		httpErr(w, http.StatusNotFound, "not found")
		return
	}
	if meta.Status == domain.PageStatusTrashed {
		attachmentPageTrashed(w)
		return
	}

	limit := h.maxBytes()
	r.Body = http.MaxBytesReader(w, r.Body, limit+attachmentMultipartSlack)
	mr, err := r.MultipartReader()
	if err != nil {
		httpErr(w, http.StatusBadRequest, "expected multipart/form-data")
		return
	}
	var part io.Reader
	var declared, rawName string
	for {
		p, err := mr.NextPart()
		if err != nil {
			if attachmentTooLarge(err) {
				httpErr(w, http.StatusRequestEntityTooLarge, "file too large")
				return
			}
			if errors.Is(err, io.EOF) {
				httpErr(w, http.StatusBadRequest, "file is required")
				return
			}
			httpErr(w, http.StatusBadRequest, "invalid multipart body")
			return
		}
		if p.FormName() == "file" && p.FileName() != "" {
			part = p
			declared = p.Header.Get("Content-Type")
			rawName = p.FileName()
			break
		}
		_, _ = io.Copy(io.Discard, p)
	}

	head := make([]byte, attachmentSniffLen)
	n, err := io.ReadFull(part, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		h.attachmentReadError(w, err)
		return
	}
	head = head[:n]
	if n == 0 {
		httpErr(w, http.StatusBadRequest, "file is empty")
		return
	}

	tmp, err := os.CreateTemp("", "knowledge-attachment-*")
	if err != nil {
		internalErr(w, r, err, "failed to buffer upload")
		return
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()
	size, err := io.Copy(tmp, io.LimitReader(io.MultiReader(bytes.NewReader(head), part), limit+1))
	if err != nil {
		h.attachmentReadError(w, err)
		return
	}
	if size > limit {
		httpErr(w, http.StatusRequestEntityTooLarge, "file too large")
		return
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		internalErr(w, r, err, "failed to buffer upload")
		return
	}

	id, err := uuid.NewV7()
	if err != nil {
		internalErr(w, r, err, "")
		return
	}
	filename := attachmentSanitizeFilename(rawName)
	contentType := attachmentContentType(head, declared)
	key := fmt.Sprintf("spaces/%s/pages/%s/%s/%s", meta.SpaceID, pageID, id, filename)

	if err := h.Storage.Put(r.Context(), key, contentType, tmp, size); err != nil {
		internalErr(w, r, err, "failed to store file")
		return
	}
	a := domain.Attachment{
		ID:          id,
		PageID:      pageID,
		SpaceID:     meta.SpaceID,
		UploaderID:  user.ID,
		Filename:    filename,
		ContentType: contentType,
		Size:        size,
		S3Key:       key,
	}
	if err := h.Store.CreateAttachment(r.Context(), &a); err != nil {
		if delErr := h.Storage.Delete(r.Context(), key); delErr != nil {
			log.Printf("attachment cleanup after failed insert %s: %v", key, delErr)
		}
		internalErr(w, r, err, "failed to save attachment")
		return
	}
	writeJSON(w, http.StatusCreated, store.AttachmentWithUploader{Attachment: a, Uploader: user.Ref()})
}

func attachmentTooLarge(err error) bool {
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}

func (h *AttachmentHandler) attachmentReadError(w http.ResponseWriter, err error) {
	if attachmentTooLarge(err) {
		httpErr(w, http.StatusRequestEntityTooLarge, "file too large")
		return
	}
	httpErr(w, http.StatusBadRequest, "failed to read upload")
}

func (h *AttachmentHandler) List(w http.ResponseWriter, r *http.Request) {
	pageID, err := mw.URLUUID(r, "pageID")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	meta, err := h.Store.GetAttachmentPageMeta(r.Context(), pageID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpErr(w, http.StatusNotFound, "not found")
			return
		}
		internalErr(w, r, err, "failed to load page")
		return
	}
	if !attachmentPageVisible(meta.Status, meta.CreatedBy, mw.UserFrom(r.Context()), mw.SpaceFrom(r.Context())) {
		httpErr(w, http.StatusNotFound, "not found")
		return
	}
	list, err := h.Store.ListPageAttachments(r.Context(), pageID)
	if err != nil {
		internalErr(w, r, err, "failed to list attachments")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *AttachmentHandler) Download(w http.ResponseWriter, r *http.Request) {
	h.attachmentServe(w, r, false)
}

func (h *AttachmentHandler) Content(w http.ResponseWriter, r *http.Request) {
	h.attachmentServe(w, r, true)
}

func (h *AttachmentHandler) attachmentLoad(w http.ResponseWriter, r *http.Request) (domain.Attachment, store.AttachmentPageMeta, bool) {
	id, err := mw.URLUUID(r, "attachmentID")
	if err != nil {
		httpErr(w, http.StatusBadRequest, "invalid id")
		return domain.Attachment{}, store.AttachmentPageMeta{}, false
	}
	var meta store.AttachmentPageMeta
	a, err := h.Store.GetAttachment(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpErr(w, http.StatusNotFound, "not found")
			return a, meta, false
		}
		internalErr(w, r, err, "failed to load attachment")
		return a, meta, false
	}
	meta, err = h.Store.GetAttachmentPageMeta(r.Context(), a.PageID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpErr(w, http.StatusNotFound, "not found")
			return a, meta, false
		}
		internalErr(w, r, err, "failed to load page")
		return a, meta, false
	}
	if !attachmentPageVisible(meta.Status, meta.CreatedBy, mw.UserFrom(r.Context()), mw.SpaceFrom(r.Context())) {
		httpErr(w, http.StatusNotFound, "not found")
		return a, meta, false
	}
	return a, meta, true
}

func (h *AttachmentHandler) attachmentServe(w http.ResponseWriter, r *http.Request, allowInline bool) {
	a, _, ok := h.attachmentLoad(w, r)
	if !ok {
		return
	}
	if h.Storage == nil {
		httpErr(w, http.StatusServiceUnavailable, "object storage not configured")
		return
	}
	body, _, size, err := h.Storage.Get(r.Context(), a.S3Key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			httpErr(w, http.StatusNotFound, "not found")
			return
		}
		internalErr(w, r, err, "failed to read file")
		return
	}
	defer body.Close()
	if size < 0 {
		size = a.Size
	}

	mediaType := attachmentMediaType(a.ContentType)
	disposition := "attachment"
	if allowInline && attachmentInlineTypes[mediaType] {
		disposition = "inline"
	}
	hdr := w.Header()
	hdr.Set("Content-Type", a.ContentType)
	hdr.Set("Content-Length", strconv.FormatInt(size, 10))
	hdr.Set("Content-Disposition", attachmentDisposition(disposition, a.Filename))
	hdr.Set("X-Content-Type-Options", "nosniff")
	hdr.Set("Cache-Control", "private, max-age=3600")
	hdr.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, body); err != nil {
		log.Printf("attachment stream %s: %v", a.ID, err)
	}
}

func (h *AttachmentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	a, meta, ok := h.attachmentLoad(w, r)
	if !ok {
		return
	}
	if meta.Status == domain.PageStatusTrashed {
		attachmentPageTrashed(w)
		return
	}
	deleted, err := h.Store.SoftDeleteAttachment(r.Context(), a.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpErr(w, http.StatusNotFound, "not found")
			return
		}
		internalErr(w, r, err, "failed to delete attachment")
		return
	}
	if h.Storage != nil {
		if err := h.Storage.Delete(r.Context(), deleted.S3Key); err != nil {
			log.Printf("attachment object delete %s: %v", deleted.S3Key, err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func attachmentMediaType(contentType string) string {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	}
	return mt
}

func attachmentContentType(head []byte, declared string) string {
	sniffed := attachmentMediaType(http.DetectContentType(head))
	if sniffed != "application/octet-stream" {
		return sniffed
	}
	d, _, err := mime.ParseMediaType(declared)
	if err != nil || d == "" || d == "application/octet-stream" || strings.ContainsAny(d, " \t\r\n") {
		return sniffed
	}
	return d
}

func attachmentSanitizeFilename(raw string) string {
	name := strings.ToValidUTF8(raw, "")
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(name)
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '/' || r == '\\' {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || strings.Trim(name, ".") == "" {
		return "file"
	}
	if len(name) > attachmentMaxFilename {
		ext := path.Ext(name)
		if len(ext) > 32 {
			ext = ""
		}
		stem := attachmentTruncateUTF8(strings.TrimSuffix(name, ext), attachmentMaxFilename-len(ext))
		name = stem + ext
	}
	return name
}

func attachmentTruncateUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	s = s[:max]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

func attachmentDisposition(kind, filename string) string {
	return fmt.Sprintf(`%s; filename="%s"; filename*=UTF-8''%s`, kind, attachmentASCIIName(filename), attachmentEncodeRFC5987(filename))
}

func attachmentASCIIName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r < 0x20 || r > 0x7e || r == '"' || r == '\\' || r == '%':
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "file"
	}
	return b.String()
}

func attachmentEncodeRFC5987(name string) string {
	const unreserved = "!#$&+-.^_`|~"
	var b strings.Builder
	for _, c := range []byte(name) {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', strings.IndexByte(unreserved, c) >= 0:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
