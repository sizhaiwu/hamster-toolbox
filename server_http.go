package main

import (
	"net/http"
	"strings"
)

// One handler tree is used by the loopback listener and the authenticated remote adapter.
func (s *server) httpHandler() *http.ServeMux {
	browser := s.browser
	mux := http.NewServeMux()
	mux.HandleFunc("/files/", browser.local)
	mux.HandleFunc("/notes/", s.notesPage)
	mux.HandleFunc("/api/", s.guard(s.api))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'")
		p := r.URL.Path
		if p == "/" {
			if r.URL.Query().Get("session") != s.token {
				http.Error(w, "请使用程序启动时提供的链接", 403)
				return
			}
			p = "/index.html"
		}
		brandType := brandContentType(strings.TrimPrefix(p, "/"))
		if p != "/desktop-media.js" && p != "/desktop-media.css" && p != "/desktop-experience.js" && p != "/desktop-experience.css" && p != "/schedule-controls.js" && p != "/schedule-library.js" && p != "/schedule-status.js" && p != "/schedule-status.css" && p != "/schedules.html" && p != "/schedules.js" && p != "/schedules.css" && p != "/index.html" && p != "/app.js" && p != "/version-info.js" && p != "/task-feedback.js" && p != "/task-history.js" && p != "/network-settings.js" && p != "/network-settings.css" && p != "/general-settings.css" && p != "/style.css" && p != "/theme.css" && p != "/toolbox-layout.css" && brandType == "" {
			http.NotFound(w, r)
			return
		}
		if (p == "/index.html" || p == "/schedules.html") && r.URL.Query().Get("session") != s.token {
			http.Error(w, "无效会话", 403)
			return
		}
		b, e := assets.ReadFile("web" + p)
		if e != nil {
			http.NotFound(w, r)
			return
		}
		if p == "/schedules.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		} else if p == "/index.html" {
			b = sharedSettingsHTML(b)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		} else if strings.HasSuffix(p, ".js") {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		} else if brandType != "" {
			w.Header().Set("Content-Type", brandType)
		} else {
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		}
		w.Write(b)
	})
	return mux
}
