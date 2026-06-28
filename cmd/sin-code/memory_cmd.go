// SPDX-License-Identifier: MIT
// Code extracted from commands.go — Memory section.

package main

// sin-debt: shrink, upgrade: when a second memory-related command is added, merge into a shared file

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/auto_mem"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/memory"
)

// ============================================================================
// Memory command (sin-code memory)
// ============================================================================

var (
	autoMemProject string
	autoMemSource  string
	autoMemMax     int
	autoMemFormat  string

	autoMemDefaultHome = auto_mem.DefaultHome
	autoMemOpen        = auto_mem.Open

	autoMemIndex     = func(s *auto_mem.Store) ([]string, error) { return s.Index() }
	autoMemReadTopic = func(s *auto_mem.Store, heading string) ([]byte, error) { return s.ReadTopic(heading) }
	autoMemAppend    = func(s *auto_mem.Store, e auto_mem.Entry) error { return s.Append(e) }
	autoMemRemove    = func(s *auto_mem.Store, heading string) error { return s.Remove(heading) }
	autoMemRotate    = func(s *auto_mem.Store, max int) (int, error) { return s.Rotate(max) }
)

func openAutoMem() (*auto_mem.Store, string, error) {
	home, err := autoMemDefaultHome()
	if err != nil {
		return nil, "", err
	}
	proj := autoMemProject
	if proj == "" {
		proj = "global"
	}
	s, err := autoMemOpen(home, proj)
	if err != nil {
		return nil, "", err
	}
	return s, proj, nil
}

var memAutoListCmd = &cobra.Command{
	Use:   "auto-list",
	Short: "List topics in MEMORY.md for the active project (issue #192 parity).",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, proj, err := openAutoMem()
		if err != nil {
			return err
		}
		idx, err := autoMemIndex(s)
		if err != nil {
			return err
		}
		if autoMemFormat == "json" {
			return json.NewEncoder(os.Stdout).Encode(struct {
				Project  string   `json:"project"`
				Path     string   `json:"path"`
				Headings []string `json:"headings"`
			}{proj, s.Path(), idx})
		}
		if len(idx) == 0 {
			fmt.Printf("(no entries) — %s\n", s.Path())
			return nil
		}
		fmt.Printf("MEMORY.md for %s (%d topics, %s):\n", proj, len(idx), s.Path())
		for _, h := range idx {
			fmt.Printf("  - %s\n", h)
		}
		return nil
	},
}

var memAutoShowCmd = &cobra.Command{
	Use:   "auto-show <heading>",
	Short: "Show the body of a single MEMORY.md topic.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, _, err := openAutoMem()
		if err != nil {
			return err
		}
		body, err := autoMemReadTopic(s, args[0])
		if err != nil {
			return err
		}
		fmt.Println(string(body))
		return nil
	},
}

var memAutoAppendCmd = &cobra.Command{
	Use:   "auto-append <heading> <body>",
	Short: "Append or replace a MEMORY.md topic. Re-issues replace the prior body.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, _, err := openAutoMem()
		if err != nil {
			return err
		}
		src := autoMemSource
		if src == "" {
			src = "manual"
		}
		if err := autoMemAppend(s, auto_mem.Entry{
			Heading:   args[0],
			Body:      args[1],
			SourceTag: src,
			AddedAt:   time.Now().UTC(),
		}); err != nil {
			return err
		}
		fmt.Printf("updated MEMORY.md topic %q (%s)\n", args[0], s.Path())
		return nil
	},
}

var memAutoRmCmd = &cobra.Command{
	Use:   "auto-rm <heading>",
	Short: "Remove a MEMORY.md topic by heading.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, _, err := openAutoMem()
		if err != nil {
			return err
		}
		if err := autoMemRemove(s, args[0]); err != nil {
			return err
		}
		fmt.Printf("removed topic %q from %s\n", args[0], s.Path())
		return nil
	},
}

var memAutoGcCmd = &cobra.Command{
	Use:   "auto-gc",
	Short: "Rotate MEMORY.md down to the most recent N topics (default 32).",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, _, err := openAutoMem()
		if err != nil {
			return err
		}
		n := autoMemMax
		if n <= 0 {
			n = 32
		}
		kept, err := autoMemRotate(s, n)
		if err != nil {
			return err
		}
		fmt.Printf("rotated MEMORY.md to %d most-recent topics (%s)\n", kept, s.Path())
		return nil
	},
}

func init() {
	MemoryCmd.AddCommand(memAutoListCmd, memAutoShowCmd, memAutoAppendCmd, memAutoRmCmd, memAutoGcCmd)

	// Reuse --project and --format where possible; the auto_mem layer
	// does not use --as (persistence is per-project, not per-actor).
	memAutoListCmd.Flags().StringVar(&autoMemProject, "project", "global", "Project key (default 'global')")
	memAutoListCmd.Flags().StringVar(&autoMemFormat, "format", "text", "Output: text|json")

	memAutoShowCmd.Flags().StringVar(&autoMemProject, "project", "global", "Project key")

	memAutoAppendCmd.Flags().StringVar(&autoMemProject, "project", "global", "Project key")
	memAutoAppendCmd.Flags().StringVar(&autoMemSource, "source", "manual", "Provenance tag for this entry")

	memAutoRmCmd.Flags().StringVar(&autoMemProject, "project", "global", "Project key")

	memAutoGcCmd.Flags().StringVar(&autoMemProject, "project", "global", "Project key")
	memAutoGcCmd.Flags().IntVar(&autoMemMax, "max", 32, "Max topics to keep (rotates down to most-recent N)")
}

var (
	memDBPath   string
	memInsight  string
	memProject  string
	memTags     string
	memActor    string
	memLimit    int
	memTopK     int
	memDepth    int
	memForgetID string
	memForget   bool
	memFormat   string

	openMemoryStoreFn = openMemoryStore

	memAddFn    = func(s *memory.Store, m *memory.Memory) error { return s.Add(m) }
	memListFn   = func(s *memory.Store, f memory.ListFilter) ([]*memory.Memory, error) { return s.List(f) }
	memGetFn    = func(s *memory.Store, id string) (*memory.Memory, error) { return s.Get(id) }
	memSearchFn = func(s *memory.Store, q, project string, k int) ([]memory.ScoredMemory, error) {
		return s.Search(q, project, k)
	}
	memAddLinkFn    = func(s *memory.Store, l memory.Link) error { return s.AddLink(l) }
	memRemoveLinkFn = func(s *memory.Store, from, to string) error { return s.RemoveLink(from, to) }
	memGraphFn      = func(s *memory.Store, id string, depth int) (map[string][]memory.Link, error) {
		return s.Graph(id, depth)
	}
	memPrimeFn  = func(s *memory.Store, q, project string, k int) (string, error) { return s.Prime(q, project, k) }
	memDeleteFn = func(s *memory.Store, id string, hard bool) error { return s.Delete(id, hard) }
	memStatsFn  = func(s *memory.Store) (map[string]int, error) { return s.Stats() }
)

var MemoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Long-term project memory with semantic search",
	Long: `Memory is a bd-style project knowledge store backed by bbolt.

  add <insight>            Add a memory
  list                      List memories (filter by --project, --tag, --actor)
  search <query>            Semantic search (uses NIM embeddings if SIN_NIM_API_KEY is set)
  link <from> <to> --rel    Add a knowledge-graph link
  unlink <from> <to>        Remove a link
  graph <id>                Show knowledge-graph neighborhood
  prime <query>             Print top-K relevant memories for an LLM prompt
  forget <id>               Soft-delete (--hard for permanent)
  show <id>                 Show one memory
  stats                     Memory statistics

Storage: ~/.config/sin-code/memory.db (override with --db).
Embeddings: NIM nv-embed-v1 (set SIN_NIM_API_KEY).`,
	SilenceUsage: true,
}

func init() {
	MemoryCmd.PersistentFlags().StringVar(&memDBPath, "db", "", "Path to bbolt DB (default ~/.config/sin-code/memory.db)")
	MemoryCmd.PersistentFlags().StringVar(&memFormat, "format", "text", "Output format: text|json")
	MemoryCmd.PersistentFlags().StringVar(&memActor, "as", "", "Actor identity (default: git user.name or 'unknown')")

	MemoryCmd.AddCommand(memAddCmd)
	MemoryCmd.AddCommand(memListCmd)
	MemoryCmd.AddCommand(memShowCmd)
	MemoryCmd.AddCommand(memSearchCmd)
	MemoryCmd.AddCommand(memLinkCmd)
	MemoryCmd.AddCommand(memUnlinkCmd)
	MemoryCmd.AddCommand(memGraphCmd)
	MemoryCmd.AddCommand(memPrimeCmd)
	MemoryCmd.AddCommand(memForgetCmd)
	MemoryCmd.AddCommand(memStatsCmd)

	memAddCmd.Flags().StringVar(&memProject, "project", "", "Project namespace")
	memAddCmd.Flags().StringVar(&memTags, "tags", "", "Comma-separated tags")

	memListCmd.Flags().StringVar(&memProject, "project", "", "Filter by project")
	memListCmd.Flags().StringVar(&memTags, "tags", "", "Filter by tag")
	memListCmd.Flags().IntVar(&memLimit, "limit", 50, "Max items (0 = all)")

	memSearchCmd.Flags().StringVar(&memProject, "project", "", "Filter by project")
	memSearchCmd.Flags().IntVar(&memTopK, "top", 10, "Top-K results")

	memLinkCmd.Flags().StringVar(&memRel, "rel", "references", "Link type: references|supports|contradicts|extends|causes")

	memGraphCmd.Flags().IntVar(&memDepth, "depth", 3, "Max traversal depth")

	memPrimeCmd.Flags().StringVar(&memProject, "project", "", "Filter by project")
	memPrimeCmd.Flags().IntVar(&memTopK, "top", 10, "Top-K results")

	memForgetCmd.Flags().BoolVar(&memForget, "hard", false, "Permanent delete (default: soft)")
}

var memAddCmd = &cobra.Command{
	Use:   "add <insight>",
	Short: "Add a memory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		memInsight = args[0]
		store, err := openMemoryStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		m := &memory.Memory{
			Insight: memInsight,
			Project: memProject,
			Tags:    splitList(memTags),
			Actor:   memActor,
		}
		if err := memAddFn(store, m); err != nil {
			return err
		}
		fmt.Printf("Stored %s: %s\n", m.ID, memTruncate(m.Insight, 80))
		return nil
	},
}

var memListCmd = &cobra.Command{
	Use:   "list",
	Short: "List memories",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openMemoryStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		results, err := memListFn(store, memory.ListFilter{
			Project: memProject,
			Tag:     memTags,
			Limit:   memLimit,
		})
		if err != nil {
			return err
		}
		if memFormat == "json" {
			return json.NewEncoder(os.Stdout).Encode(results)
		}
		if len(results) == 0 {
			fmt.Println("(no memories)")
			return nil
		}
		for _, m := range results {
			project := m.Project
			if project == "" {
				project = "-"
			}
			fmt.Printf("%s  [%-12s]  %s\n", m.ID, project, memTruncate(m.Insight, 80))
		}
		return nil
	},
}

var memShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show a single memory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openMemoryStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		m, err := memGetFn(store, args[0])
		if err != nil {
			return err
		}
		if memFormat == "json" {
			return json.NewEncoder(os.Stdout).Encode(m)
		}
		fmt.Printf("ID:      %s\n", m.ID)
		fmt.Printf("Insight: %s\n", m.Insight)
		if m.Project != "" {
			fmt.Printf("Project: %s\n", m.Project)
		}
		if len(m.Tags) > 0 {
			fmt.Printf("Tags:    %s\n", strings.Join(m.Tags, ", "))
		}
		if m.Actor != "" {
			fmt.Printf("Actor:   %s\n", m.Actor)
		}
		fmt.Printf("Created: %s\n", m.Created.Format("2006-01-02 15:04:05"))
		fmt.Printf("Updated: %s\n", m.Updated.Format("2006-01-02 15:04:05"))
		return nil
	},
}

var memSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Semantic search",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openMemoryStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		results, err := memSearchFn(store, args[0], memProject, memTopK)
		if err != nil {
			return err
		}
		if memFormat == "json" {
			return json.NewEncoder(os.Stdout).Encode(results)
		}
		if len(results) == 0 {
			fmt.Println("(no results)")
			return nil
		}
		fmt.Printf("Top %d for %q:\n", len(results), args[0])
		for _, r := range results {
			project := r.Project
			if project == "" {
				project = "-"
			}
			fmt.Printf("  %.4f  %s  [%-12s]  %s\n", r.Score, r.ID, project, memTruncate(r.Insight, 70))
		}
		return nil
	},
}

var memRel string

var memLinkCmd = &cobra.Command{
	Use:   "link <from> <to>",
	Short: "Add a knowledge-graph link",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openMemoryStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		l := memory.Link{From: args[0], To: args[1], Rel: memRel}
		if err := memAddLinkFn(store, l); err != nil {
			return err
		}
		fmt.Printf("Linked %s --%s--> %s\n", l.From, l.Rel, l.To)
		return nil
	},
}

var memUnlinkCmd = &cobra.Command{
	Use:   "unlink <from> <to>",
	Short: "Remove a knowledge-graph link",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openMemoryStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		if err := memRemoveLinkFn(store, args[0], args[1]); err != nil {
			return err
		}
		fmt.Printf("Unlinked %s ---> %s\n", args[0], args[1])
		return nil
	},
}

var memGraphCmd = &cobra.Command{
	Use:   "graph <id>",
	Short: "Show knowledge-graph neighborhood",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openMemoryStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		tree, err := memGraphFn(store, args[0], memDepth)
		if err != nil {
			return err
		}
		if memFormat == "json" {
			return json.NewEncoder(os.Stdout).Encode(tree)
		}
		fmt.Printf("Graph from %s (depth %d):\n", args[0], memDepth)
		for id, links := range tree {
			fmt.Printf("  %s\n", id)
			for _, l := range links {
				fmt.Printf("    --%s--> %s\n", l.Rel, l.To)
			}
		}
		return nil
	},
}

var memPrimeCmd = &cobra.Command{
	Use:   "prime <query>",
	Short: "Print top-K relevant memories for an LLM prompt",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openMemoryStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		text, err := memPrimeFn(store, args[0], memProject, memTopK)
		if err != nil {
			return err
		}
		fmt.Print(text)
		return nil
	},
}

var memForgetCmd = &cobra.Command{
	Use:   "forget <id>",
	Short: "Soft-delete a memory (--hard for permanent)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openMemoryStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		if err := memDeleteFn(store, args[0], memForget); err != nil {
			return err
		}
		verb := "Forgotten"
		if memForget {
			verb = "Hard-deleted"
		}
		fmt.Printf("%s %s\n", verb, args[0])
		return nil
	},
}

var memStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show memory statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openMemoryStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		stats, err := memStatsFn(store)
		if err != nil {
			return err
		}
		enabled, dim := store.EmbeddingStatus()
		if memFormat == "json" {
			out := map[string]interface{}{
				"stats":     stats,
				"embedder":  enabled,
				"embed_dim": dim,
			}
			return json.NewEncoder(os.Stdout).Encode(out)
		}
		fmt.Printf("Total:      %d memories\n", stats["total"])
		fmt.Printf("Links:      %d\n", stats["links"])
		fmt.Printf("Embeddings: %d cached\n", stats["embeddings"])
		if enabled {
			fmt.Printf("Embedder:   enabled (dim=%d)\n", dim)
		} else {
			fmt.Println("Embedder:   disabled (set SIN_NIM_API_KEY to enable semantic search)")
		}
		return nil
	},
}

func openMemoryStore() (*memory.Store, error) {
	memory.SetupNIMEmbedder()
	return memory.Open(memDBPath)
}

func memTruncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
