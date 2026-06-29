// SPDX-License-Identifier: MIT
// Purpose: sin-code web UI — stdlib HTTP server that exposes the
// orchestrator, todo store, notifications, and EFM stacks through a
// browser. All templates and static assets are embedded.
package webui

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"os"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/health"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/notifications"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/todo"
)

type Server struct {
	addr        string
	host        string
	port        int
	mux         *http.ServeMux
	templates   *template.Template
	staticFS    fs.FS
	todoDB      string
	notifDB     string
	openBrowser bool
	httpServer  *http.Server
	ln          net.Listener
	addr_       string
	health      *health.Checker
}

type Config struct {
	Host        string
	Port        int
	TodoDB      string
	NotifDB     string
	OpenBrowser bool
}

func Start(port int) error {
	host := os.Getenv("SIN_CODE_WEBUI_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	cfg := Config{Host: host, Port: port}
	return StartWith(cfg)
}

func StartWith(cfg Config) error {
	s, err := NewServer(cfg)
	if err != nil {
		return err
	}
	return s.ListenAndServe()
}

func NewServer(cfg Config) (*Server, error) {
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = 27402
	}
	if cfg.TodoDB == "" {
		cfg.TodoDB = defaultTodoDB()
	}
	if cfg.NotifDB == "" {
		cfg.NotifDB = defaultNotifDB()
	}

	tmpl, err := loadTemplates()
	if err != nil {
		return nil, fmt.Errorf("load templates: %w", err)
	}

	s := &Server{
		host:        cfg.Host,
		port:        cfg.Port,
		mux:         http.NewServeMux(),
		templates:   tmpl,
		staticFS:    staticSub(),
		todoDB:      cfg.TodoDB,
		notifDB:     cfg.NotifDB,
		openBrowser: cfg.OpenBrowser,
		health:      health.NewChecker("webui"),
	}
	s.routes()
	s.setupHealthChecks()
	return s, nil
}

func (s *Server) Addr() string {
	if s.addr_ != "" {
		return s.addr_
	}
	return fmt.Sprintf("%s:%d", s.host, s.port)
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /{$}", s.handleIndex)
	s.mux.HandleFunc("GET /orchestrator", s.handleOrchestratorPage)
	s.mux.HandleFunc("POST /orchestrator/run", s.handleOrchestratorRun)
	s.mux.HandleFunc("GET /todos", s.handleTodosPage)
	s.mux.HandleFunc("POST /todos/add", s.handleTodosAdd)
	s.mux.HandleFunc("GET /todos/{id}", s.handleTodoDetail)
	s.mux.HandleFunc("GET /notifications", s.handleNotificationsPage)
	s.mux.HandleFunc("GET /efm", s.handleEfmPage)
	s.mux.HandleFunc("GET /efm/{name}", s.handleEfmDetail)

	s.mux.HandleFunc("GET /api/orchestrator/agents.json", s.handleAgentsJSON)
	s.mux.HandleFunc("GET /api/notifications.json", s.handleNotificationsJSON)
	s.mux.HandleFunc("GET /api/todos.json", s.handleTodosJSON)

	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(s.staticFS))))

	// Health check endpoints
	s.mux.Handle("GET /health", s.health.Handler())
	s.mux.Handle("GET /live", health.LivenessHandler())
	s.mux.Handle("GET /ready", health.ReadinessHandler(s.health))
	s.mux.Handle("GET /info", health.InfoHandler(s.health.Version()))
}

func (s *Server) setupHealthChecks() {
	// Add custom health checks for webui
	s.health.RegisterCheck("templates", func(ctx context.Context) health.Check {
		if s.templates == nil {
			return health.Check{
				Status:  health.StatusUnhealthy,
				Message: "templates not loaded",
			}
		}
		return health.Check{
			Status:  health.StatusHealthy,
			Message: "templates loaded",
		}
	})

	s.health.RegisterCheck("todo_db", func(ctx context.Context) health.Check {
		if s.todoDB == "" {
			return health.Check{
				Status:  health.StatusDegraded,
				Message: "todo database not configured",
			}
		}
		return health.Check{
			Status:  health.StatusHealthy,
			Message: "todo database configured",
		}
	})
}

func loadTemplates() (*template.Template, error) {
	sub, err := templateFSSubHook()
	if err != nil {
		return nil, err
	}
	tmpl, err := parseFSHook(template.New("").Funcs(template.FuncMap{
		"safeHTML": func(s string) template.HTML { return template.HTML(s) }, // #nosec G203
	}), sub)
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return tmpl, nil
}

type pageData struct {
	Title  string
	Active string
	Addr   string
	Prompt string
	Agents []orchestrator.AgentConfig
	Result *orchestrator.Result
	Error  string

	Todos []*todo.Todo
	Total int
	Open  int
	Done  int
	Added *todo.Todo

	Todo  *todo.Todo
	Deps  []todo.Dependency
	Audit []*todo.AuditEntry

	Notifications []*notifications.Notification
	Unread        int

	Stacks  []efmStack
	Runtime string
}

func (s *Server) render(w http.ResponseWriter, name string, data pageData) {
	if data.Title == "" {
		data.Title = "Home"
	}
	data.Addr = s.Addr()
	bodyRaw, err := fs.ReadFile(templateSub(), name)
	if err != nil {
		http.Error(w, "template not found: "+name, http.StatusInternalServerError)
		return
	}
	cloned, err := templateCloneHook(s.templates)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := templateParseHook(cloned, string(bodyRaw)); err != nil {
		http.Error(w, "parse "+name+": "+err.Error(), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := templateExecHook(cloned, &buf, "base", data); err != nil {
		http.Error(w, "render: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}
