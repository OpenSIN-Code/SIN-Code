// SPDX-License-Identifier: MIT
package chains

import (
	"errors"
	"io"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/pkg/tools"
)

type fakeTool struct {
	meta tools.ToolMetadata
	run  func(map[string]interface{}) (interface{}, error)
}

func (f *fakeTool) GetMetadata() tools.ToolMetadata { return f.meta }
func (f *fakeTool) Execute(a map[string]interface{}) (interface{}, error) {
	return f.run(a)
}

func reg(t *testing.T, name string, run func(map[string]interface{}) (interface{}, error)) *tools.Registry {
	t.Helper()
	r := tools.NewRegistry()
	if err := r.RegisterTool(&fakeTool{meta: tools.ToolMetadata{Name: name}, run: run}); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestChainSucceeds(t *testing.T) {
	r := reg(t, "gen", func(map[string]interface{}) (interface{}, error) { return "code", nil })
	e := NewEngineWith(r, io.Discard)
	out, err := e.ExecuteLoopChain("c", []Step{{ToolName: "gen"}},
		nil, func(o interface{}) bool { return o == "code" }, 3)
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	if out != "code" {
		t.Errorf("out = %v, want code", out)
	}
}

func TestChainRetriesThenSucceeds(t *testing.T) {
	attempts := 0
	r := reg(t, "flaky", func(map[string]interface{}) (interface{}, error) {
		attempts++
		if attempts < 3 {
			return nil, errors.New("transient")
		}
		return "fixed", nil
	})
	e := NewEngineWith(r, io.Discard)
	out, err := e.ExecuteLoopChain("c", []Step{{ToolName: "flaky"}},
		nil, func(o interface{}) bool { return o == "fixed" }, 5)
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	if out != "fixed" || attempts != 3 {
		t.Errorf("out=%v attempts=%d, want fixed/3", out, attempts)
	}
}

func TestChainExhausted(t *testing.T) {
	r := reg(t, "bad", func(map[string]interface{}) (interface{}, error) {
		return nil, errors.New("always fails")
	})
	e := NewEngineWith(r, io.Discard)
	_, err := e.ExecuteLoopChain("c", []Step{{ToolName: "bad"}}, nil, nil, 2)
	if !errors.Is(err, ErrChainExhausted) {
		t.Errorf("err = %v, want ErrChainExhausted", err)
	}
}

func TestChainMissingTool(t *testing.T) {
	e := NewEngineWith(tools.NewRegistry(), io.Discard)
	_, err := e.ExecuteLoopChain("c", []Step{{ToolName: "ghost"}}, nil, nil, 1)
	if err == nil {
		t.Error("expected missing-tool error")
	}
}

func TestChainNoSteps(t *testing.T) {
	e := NewEngineWith(tools.NewRegistry(), io.Discard)
	if _, err := e.ExecuteLoopChain("c", nil, nil, nil, 1); err == nil {
		t.Error("expected no-steps error")
	}
}

func TestChainInputMapperThreadsData(t *testing.T) {
	r := tools.NewRegistry()
	_ = r.RegisterTool(&fakeTool{meta: tools.ToolMetadata{Name: "a"}, run: func(map[string]interface{}) (interface{}, error) {
		return 21, nil
	}})
	_ = r.RegisterTool(&fakeTool{meta: tools.ToolMetadata{Name: "b"}, run: func(in map[string]interface{}) (interface{}, error) {
		return in["v"].(int) * 2, nil
	}})
	e := NewEngineWith(r, io.Discard)
	out, err := e.ExecuteLoopChain("c", []Step{
		{ToolName: "a"},
		{ToolName: "b", InputMapper: func(prev interface{}, _ map[string]interface{}) map[string]interface{} {
			return map[string]interface{}{"v": prev.(int)}
		}},
	}, nil, func(o interface{}) bool { return o == 42 }, 1)
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	if out != 42 {
		t.Errorf("out = %v, want 42", out)
	}
}
