package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestFlatRegistrationFromMultipleHandlers(t *testing.T) {
	tag := func(name string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(name + ":" + chi.URLParam(r, "key") + chi.URLParam(r, "pageID")))
		}
	}
	pass := func(next http.Handler) http.Handler { return next }

	spaces := func(r chi.Router) {
		r.With(pass).Get("/spaces/{key}", tag("space"))
		r.With(pass).Get("/spaces/{key}/tree", tag("tree"))
		r.With(pass).Put("/spaces/{key}/members/{userID}", tag("member"))
	}
	pages := func(r chi.Router) {
		r.With(pass).Post("/spaces/{key}/pages", tag("create-page"))
		r.With(pass).Get("/pages/{pageID}", tag("page"))
	}
	search := func(r chi.Router) {
		r.With(pass).Get("/spaces/{key}/labels", tag("labels"))
		r.With(pass).Get("/spaces/{key}/export", tag("export"))
	}

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		spaces(r)
		pages(r)
		search(r)
	})

	cases := map[string]string{
		"GET /api/v1/spaces/ENG":        "space:ENG",
		"GET /api/v1/spaces/ENG/tree":   "tree:ENG",
		"POST /api/v1/spaces/ENG/pages": "create-page:ENG",
		"GET /api/v1/spaces/ENG/labels": "labels:ENG",
		"GET /api/v1/spaces/ENG/export": "export:ENG",
		"GET /api/v1/pages/abc":         "page:abc",
	}
	for k, want := range cases {
		var method, path string
		for i := range k {
			if k[i] == ' ' {
				method, path = k[:i], k[i+1:]
				break
			}
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		if rec.Body.String() != want {
			t.Errorf("%s -> %q, want %q", k, rec.Body.String(), want)
		}
	}
}
