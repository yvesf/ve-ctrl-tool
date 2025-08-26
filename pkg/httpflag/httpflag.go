package httpflag

import (
	_ "embed"
	"encoding/json"
	"flag"
	"html/template"
	"net/http"
	"strings"
)

//go:embed template.html
var templateHTML string

func NewHandler(visitor func(fn func(*flag.Flag))) http.HandlerFunc {
	mux := http.NewServeMux()

	indexHTML := template.Must(template.New("template.html").Parse(templateHTML))

	mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flags := make(map[string]*flag.Flag)
		visitor(func(f *flag.Flag) {
			flags[f.Name] = f
		})

		// Render HTML for browsers.
		if strings.HasPrefix(r.Header.Get("accept"), "text/html") {
			err := indexHTML.ExecuteTemplate(w, "table", map[string]any{
				"flags": flags,
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		// Write JSON for all others.
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(flags)
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))

	mux.Handle("POST /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		patch := map[string]string{}
		for field := range r.Form {
			patch[field] = r.Form.Get(field)
		}
		visitor(func(f *flag.Flag) {
			if value, found := patch[f.Name]; found {
				err = f.Value.Set(value)
			}
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Render HTML for browsers, otherwise JSON.
		if strings.HasPrefix(r.Header.Get("accept"), "text/html") {
			err = indexHTML.ExecuteTemplate(w, "update", map[string]any{
				"patch": patch,
				"url":   r.URL,
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		} else {
			w.Header().Set("content-type", "application/json")
			_ = json.NewEncoder(w).Encode(patch)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
	}))

	return mux.ServeHTTP
}
