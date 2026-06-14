package skills

import (
	"context"
	"fmt"
	"sync"
)

type ParallelRunner struct {
	runner *Runner
}

func NewParallelRunner(runner *Runner) *ParallelRunner {
	return &ParallelRunner{runner: runner}
}

// RunParallel executes multiple skills concurrently and aggregates results.
func (pr *ParallelRunner) RunParallel(ctx context.Context, skillNames []string, opts RunOptions) (map[string]*RunResult, error) {
	var wg sync.WaitGroup
	results := make(map[string]*RunResult)
	errors := make(map[string]error)
	var mu sync.Mutex

	for _, name := range skillNames {
		wg.Add(1)
		go func(skillName string) {
			defer wg.Done()
			res, err := pr.runner.Run(ctx, skillName, opts)
			mu.Lock()
			if err != nil {
				errors[skillName] = err
			} else {
				results[skillName] = res
			}
			mu.Unlock()
		}(name)
	}
	wg.Wait()
	if len(errors) > 0 {
		return results, fmt.Errorf("parallel execution errors: %v", errors)
	}
	return results, nil
}
