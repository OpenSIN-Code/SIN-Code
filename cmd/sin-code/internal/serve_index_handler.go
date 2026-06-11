// SPDX-License-Identifier: MIT

package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func handleIndex(ctx context.Context, args map[string]any) (string, error) {
	action, _ := args["action"].(string)
	if action == "" {
		action = "status"
	}
	root, _ := args["root"].(string)
	if root == "" {
		root = "."
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("invalid root: %w", err)
	}

	var res any
	switch action {
	case "build":
		idx, err := buildIndex(root)
		if err != nil {
			return "", err
		}
		if err := saveIndex(idx); err != nil {
			return "", err
		}
		setFileIndex(idx)
		res = map[string]any{"files": idx.len(), "root": root}
	case "refresh":
		idx, existed, err := getFileIndex(root)
		if err != nil {
			return "", err
		}
		if !existed {
			return "", fmt.Errorf("no index found — run build first")
		}
		idx, added, removed, err := refreshIndex(idx)
		if err != nil {
			return "", err
		}
		if err := saveIndex(idx); err != nil {
			return "", err
		}
		setFileIndex(idx)
		res = map[string]any{"added": added, "removed": removed, "total": idx.len()}
	case "status":
		idx, existed, err := getFileIndex(root)
		if err != nil {
			return "", err
		}
		if !existed || idx.len() == 0 {
			res = map[string]any{"exists": false, "root": root}
		} else {
			res = map[string]any{
				"exists":     true,
				"root":       root,
				"files":      idx.len(),
				"created_at": idx.createdAt.Format(time.RFC3339),
			}
		}
	case "clear":
		p := indexPath(root)
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return "", err
		}
		setFileIndex(nil)
		res = map[string]any{"cleared": true}
	default:
		return "", fmt.Errorf("unknown action: %s (build|refresh|status|clear)", action)
	}
	b, _ := json.Marshal(res)
	return string(b), nil
}

func handleIndexSearch(ctx context.Context, args map[string]any) (string, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	root, _ := args["root"].(string)
	if root == "" {
		root = "."
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("invalid root: %w", err)
	}
	searchType, _ := args["search_type"].(string)
	if searchType == "" {
		searchType = "regex"
	}
	maxResults := intArg(args, "max_results", 50)

	idx, existed, err := getFileIndex(root)
	if err != nil {
		return "", fmt.Errorf("index load: %w", err)
	}
	if !existed || idx.len() == 0 {
		idx, err = buildIndex(root)
		if err != nil {
			return "", fmt.Errorf("index build: %w", err)
		}
		if err := saveIndex(idx); err != nil {
			return "", fmt.Errorf("index save: %w", err)
		}
		setFileIndex(idx)
	} else {
		idx, _, _, err = refreshIndex(idx)
		if err != nil {
			return "", fmt.Errorf("index refresh: %w", err)
		}
		if err := saveIndex(idx); err != nil {
			return "", fmt.Errorf("index save: %w", err)
		}
		setFileIndex(idx)
	}

	results, err := searchWithIndex(idx, root, query, searchType, maxResults, false)
	if err != nil {
		return "", err
	}
	b, _ := json.Marshal(results)
	return string(b), nil
}
