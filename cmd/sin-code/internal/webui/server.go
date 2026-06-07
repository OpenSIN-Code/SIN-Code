// SPDX-License-Identifier: MIT
// Purpose: sin-code web UI — stdlib HTTP server that exposes the
// orchestrator, todo store, notifications, and EFM stacks through a
// browser. All templates and static assets are embedded.
package webui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/notifications"
	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/orchestrator"
	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/todo"
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
	}
	s.routes()
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
}

func loadTemplates() (*template.Template, error) {
	sub, err := fs.Sub(templateFS, "templates")
	if err != nil {
		return nil, err
	}
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
	}).ParseFS(sub, "*.html")
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

	Todos  []*todo.Todo
	Total  int
	Open   int
	Done   int
	Added  *todo.Todo

	Todo          *todo.Todo
	Deps          []todo.Dependency
	Audit         []*todo.AuditEntry

	Notifications []*notifications.Notification
	Unread        int

	Stacks []efmStack
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
	cloned, err := s.templates.Clone()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := cloned.Parse(string(bodyRaw)); err != nil {
		http.Error(w, "parse "+name+": "+err.Error(), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := cloned.ExecuteTemplate(&buf, "base", data); err != nil {
		http.Error(w, "render: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	s.render(w, "index.html", pageData{Title: "Home", Active: "home"})
}

func (s *Server) handleOrchestratorPage(w http.ResponseWriter, r *http.Request) {
	agents := defaultAgentConfigs()
	s.render(w, "orchestrator.html", pageData{
		Title:       "Orchestrator",
		Active:      "orchestrator",
		Agents:      agents,
		Prompt:      r.URL.Query().Get("prompt"),
	})
}

func (s *Server) handleOrchestratorRun(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	prompt := strings.TrimSpace(r.FormValue("prompt"))
	if prompt == "" {
		s.render(w, "orchestrator.html", pageData{
			Title:  "Orchestrator",
			Active: "orchestrator",
			Agents: defaultAgentConfigs(),
			Error:  "Prompt is required",
			Prompt: prompt,
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	o := orchestrator.New()
	res, err := o.Run(ctx, prompt)

	agents := defaultAgentConfigs()
	data := pageData{
		Title:  "Orchestrator",
		Active: "orchestrator",
		Prompt: prompt,
		Agents: agents,
		Result: res,
	}
	if err != nil {
		data.Error = err.Error()
	}
	if res != nil {
		data.Result = res
	}
	s.render(w, "orchestrator.html", data)
}

func (s *Server) handleTodosPage(w http.ResponseWriter, r *http.Request) {
	store, err := todo.Open(s.todoDB)
	if err != nil {
		s.render(w, "todos.html", pageData{
			Title:  "Todos",
			Active: "todos",
			Error:  err.Error(),
		})
		return
	}
	defer store.Close()
	ts, err := store.List()
	if err != nil {
		s.render(w, "todos.html", pageData{
			Title:  "Todos",
			Active: "todos",
			Error:  err.Error(),
		})
		return
	}
	total, openN, doneN := 0, 0, 0
	for _, t := range ts {
		total++
		if t.Status == todo.StatusOpen || t.Status == todo.StatusInProgress || t.Status == todo.StatusBlocked {
			openN++
		}
		if t.Status == todo.StatusDone {
			doneN++
		}
	}
	addedID := r.URL.Query().Get("added")
	var added *todo.Todo
	if addedID != "" {
		if a, err := store.Get(addedID); err == nil {
			added = a
		}
	}
	s.render(w, "todos.html", pageData{
		Title:  "Todos",
		Active: "todos",
		Todos:  ts,
		Total:  total,
		Open:   openN,
		Done:   doneN,
		Added:  added,
	})
}

func (s *Server) handleTodosAdd(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Redirect(w, r, "/todos?err=title_required", http.StatusSeeOther)
		return
	}
	store, err := todo.Open(s.todoDB)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer store.Close()
	t := &todo.Todo{
		Title:       title,
		Description: strings.TrimSpace(r.FormValue("description")),
		Priority:    todo.Priority(strings.TrimSpace(r.FormValue("priority"))),
		Type:        todo.TodoType(strings.TrimSpace(r.FormValue("type"))),
		Assignee:    strings.TrimSpace(r.FormValue("assignee")),
	}
	if t.Priority == "" {
		t.Priority = todo.PriorityP2
	}
	if t.Type == "" {
		t.Type = todo.TypeTask
	}
	if err := store.Add(t); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/todos?added="+url.QueryEscape(t.ID), http.StatusSeeOther)
}

func (s *Server) handleTodoDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	store, err := todo.Open(s.todoDB)
	if err != nil {
		s.render(w, "todo_detail.html", pageData{
			Title:  "Todo " + id,
			Active: "todos",
			Error:  err.Error(),
		})
		return
	}
	defer store.Close()
	t, err := store.Get(id)
	if err != nil {
		s.render(w, "todo_detail.html", pageData{
			Title:  "Todo " + id,
			Active: "todos",
			Error:  "Todo not found: " + id,
		})
		return
	}
	deps, _ := store.GetDeps(id)
	audit, _ := store.ListAudit(id)
	s.render(w, "todo_detail.html", pageData{
		Title:  "Todo " + id,
		Active: "todos",
		Todo:   t,
		Deps:   deps,
		Audit:  audit,
	})
}

func (s *Server) handleNotificationsPage(w http.ResponseWriter, r *http.Request) {
	store, err := notifications.Open(s.notifDB)
	if err != nil {
		s.render(w, "notifications.html", pageData{
			Title:  "Notifications",
			Active: "notifications",
			Error:  err.Error(),
		})
		return
	}
	defer store.Close()
	ns, err := store.List(notifications.ListFilter{NotDismissed: true}, 100)
	if err != nil {
		s.render(w, "notifications.html", pageData{
			Title:  "Notifications",
			Active: "notifications",
			Error:  err.Error(),
		})
		return
	}
	unread := 0
	for _, n := range ns {
		if !n.Read {
			unread++
		}
	}
	s.render(w, "notifications.html", pageData{
		Title:         "Notifications",
		Active:        "notifications",
		Notifications: ns,
		Total:         len(ns),
		Unread:        unread,
	})
}

type efmStack struct {
	Name        string
	Path        string
	Status      string
	StatusBadge string
	Started     *time.Time
	Expires     *time.Time
	Runtime     string
}

func (s *Server) handleEfmPage(w http.ResponseWriter, r *http.Request) {
	stacks, runtime, err := discoverEfmStacks()
	data := pageData{
		Title:   "EFM Stacks",
		Active:  "efm",
		Stacks:  stacks,
		Runtime: runtime,
	}
	if err != nil {
		data.Error = err.Error()
	}
	s.render(w, "efm.html", data)
}

func (s *Server) handleEfmDetail(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	stacks, runtime, err := discoverEfmStacks()
	data := pageData{
		Title:   "EFM " + name,
		Active:  "efm",
		Stacks:  stacks,
		Runtime: runtime,
	}
	if err != nil {
		data.Error = err.Error()
	}
	for _, st := range stacks {
		if st.Name == name {
			data.Title = "EFM " + name
			break
		}
	}
	s.render(w, "efm.html", data)
}

func (s *Server) handleAgentsJSON(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, defaultAgentConfigs())
}

func (s *Server) handleNotificationsJSON(w http.ResponseWriter, r *http.Request) {
	store, err := notifications.Open(s.notifDB)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	defer store.Close()
	ns, err := store.List(notifications.ListFilter{}, 0)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if ns == nil {
		ns = []*notifications.Notification{}
	}
	writeJSON(w, ns)
}

func (s *Server) handleTodosJSON(w http.ResponseWriter, r *http.Request) {
	store, err := todo.Open(s.todoDB)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	defer store.Close()
	ts, err := store.List()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	if ts == nil {
		ts = []*todo.Todo{}
	}
	writeJSON(w, ts)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeJSONError(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func defaultAgentConfigs() []orchestrator.AgentConfig {
	return orchestrator.DefaultAgents()
}

func defaultTodoDB() string {
	if env := os.Getenv("SIN_CODE_TODO_DB"); env != "" {
		return env
	}
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "todo.db"
	}
	return filepath.Join(cfg, "sin-code", "todo.db")
}

func defaultNotifDB() string {
	if env := os.Getenv("SIN_CODE_NOTIF_DB"); env != "" {
		return env
	}
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "notifications.db"
	}
	return filepath.Join(cfg, "sin-code", "notifications.db")
}

func efmMetaDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".local", "state", "sin-code", "efm")
	}
	cfg, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "sin-code-efm")
	}
	return filepath.Join(cfg, "sin-code", "efm")
}

func detectContainerRuntime() string {
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("orb"); err == nil {
			return "orb"
		}
	}
	if _, err := exec.LookPath("docker"); err == nil {
		return "docker"
	}
	return ""
}

func discoverEfmStacks() ([]efmStack, string, error) {
	rt := detectContainerRuntime()
	dir := efmMetaDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []efmStack{}, rt, nil
		}
		return nil, rt, err
	}
	var out []efmStack
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".meta") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var meta map[string]string
		if err := json.Unmarshal(raw, &meta); err != nil {
			continue
		}
		stackPath := meta["stack"]
		name := strings.TrimSuffix(filepath.Base(stackPath), filepath.Ext(stackPath))
		status := "unknown"
		if rt != "" {
			cmd := exec.Command(rt, "ps", "-a", "--filter", "label=com.docker.compose.project="+name, "--format", "{{.Status}}")
			outBytes, _ := cmd.Output()
			running := strings.Contains(string(outBytes), "Up")
			if running {
				status = "running"
			} else {
				status = "stopped"
			}
		}
		var started, expires *time.Time
		if t, err := time.Parse(time.RFC3339, meta["started"]); err == nil {
			started = &t
		}
		if t, err := time.Parse(time.RFC3339, meta["expires"]); err == nil {
			expires = &t
		}
		st := efmStack{
			Name:        name,
			Path:        stackPath,
			Status:      status,
			StatusBadge: status,
			Started:     started,
			Expires:     expires,
			Runtime:     meta["runtime"],
		}
		out = append(out, st)
	}
	if out == nil {
		out = []efmStack{}
	}
	return out, rt, nil
}

func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", s.host, s.port))
	if err != nil {
		return err
	}
	s.ln = ln
	s.addr_ = ln.Addr().String()

	s.httpServer = &http.Server{
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	if s.openBrowser {
		go func() {
			time.Sleep(200 * time.Millisecond)
			_ = openInBrowser("http://" + s.addr_)
		}()
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.httpServer.Serve(ln)
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-stop:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(ctx)
	}
}

func openInBrowser(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Start()
}

func listenOn(host string, port int) (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
}
