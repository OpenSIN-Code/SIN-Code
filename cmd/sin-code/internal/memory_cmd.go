// SPDX-License-Identifier: MIT
// Purpose: sin-code memory CLI — long-term project knowledge.
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/memory"
)

var (
	memDBPath  string
	memInsight string
	memProject string
	memTags    string
	memActor   string
	memLimit   int
	memTopK    int
	memDepth   int
	memForgetID string
	memForget   bool
	memFormat  string
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
		store, err := openMemoryStore()
		if err != nil {
			return err
		}
		defer store.Close()
		m := &memory.Memory{
			Insight: memInsight,
			Project: memProject,
			Tags:    splitCSV(memTags),
			Actor:   memActor,
		}
		if err := store.Add(m); err != nil {
			return err
		}
		fmt.Printf("Stored %s: %s\n", m.ID, truncate(m.Insight, 80))
		return nil
	},
}

var memListCmd = &cobra.Command{
	Use:   "list",
	Short: "List memories",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openMemoryStore()
		if err != nil {
			return err
		}
		defer store.Close()
		results, err := store.List(memory.ListFilter{
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
			fmt.Printf("%s  [%-12s]  %s\n", m.ID, project, truncate(m.Insight, 80))
		}
		return nil
	},
}

var memShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show a single memory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openMemoryStore()
		if err != nil {
			return err
		}
		defer store.Close()
		m, err := store.Get(args[0])
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
		store, err := openMemoryStore()
		if err != nil {
			return err
		}
		defer store.Close()
		results, err := store.Search(args[0], memProject, memTopK)
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
			fmt.Printf("  %.4f  %s  [%-12s]  %s\n", r.Score, r.ID, project, truncate(r.Insight, 70))
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
		store, err := openMemoryStore()
		if err != nil {
			return err
		}
		defer store.Close()
		l := memory.Link{From: args[0], To: args[1], Rel: memRel}
		if err := store.AddLink(l); err != nil {
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
		store, err := openMemoryStore()
		if err != nil {
			return err
		}
		defer store.Close()
		if err := store.RemoveLink(args[0], args[1]); err != nil {
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
		store, err := openMemoryStore()
		if err != nil {
			return err
		}
		defer store.Close()
		tree, err := store.Graph(args[0], memDepth)
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
		store, err := openMemoryStore()
		if err != nil {
			return err
		}
		defer store.Close()
		text, err := store.Prime(args[0], memProject, memTopK)
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
		store, err := openMemoryStore()
		if err != nil {
			return err
		}
		defer store.Close()
		if err := store.Delete(args[0], memForget); err != nil {
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
		store, err := openMemoryStore()
		if err != nil {
			return err
		}
		defer store.Close()
		stats, err := store.Stats()
		if err != nil {
			return err
		}
		enabled, dim := store.EmbeddingStatus()
		if memFormat == "json" {
			out := map[string]interface{}{
				"stats":         stats,
				"embedder":      enabled,
				"embed_dim":     dim,
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
