// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when webui is refactored
// Purpose: HTTP handler methods for the web UI server — page renders and
// JSON API endpoints for orchestrator, todos, notifications, and EFM stacks.
package webui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/notifications"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/todo"
)

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	s.render(w, "index.html", pageData{Title: "Home", Active: "home"})
}

func (s *Server) handleOrchestratorPage(w http.ResponseWriter, r *http.Request) {
	agents := defaultAgentConfigs()
	s.render(w, "orchestrator.html", pageData{
		Title:  "Orchestrator",
		Active: "orchestrator",
		Agents: agents,
		Prompt: r.URL.Query().Get("prompt"),
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

	res, err := orchestratorRunFunc(ctx, prompt)

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
	store, err := todoOpenHook(s.todoDB)
	if err != nil {
		s.render(w, "todos.html", pageData{
			Title:  "Todos",
			Active: "todos",
			Error:  err.Error(),
		})
		return
	}
	defer store.Close()
	ts, err := todoListHook(store)
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
	store, err := todoOpenHook(s.todoDB)
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
	if err := todoAddHook(store, t); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/todos?added="+url.QueryEscape(t.ID), http.StatusSeeOther)
}

func (s *Server) handleTodoDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	store, err := todoOpenHook(s.todoDB)
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
	store, err := notifOpenHook(s.notifDB)
	if err != nil {
		s.render(w, "notifications.html", pageData{
			Title:  "Notifications",
			Active: "notifications",
			Error:  err.Error(),
		})
		return
	}
	defer store.Close()
	ns, err := notifListHook(store, notifications.ListFilter{NotDismissed: true}, 100)
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
	store, err := notifOpenHook(s.notifDB)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	defer store.Close()
	ns, err := notifListHook(store, notifications.ListFilter{}, 0)
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
	store, err := todoOpenHook(s.todoDB)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err)
		return
	}
	defer store.Close()
	ts, err := todoListHook(store)
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
