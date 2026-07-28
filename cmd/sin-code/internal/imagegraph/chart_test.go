package imagegraph

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	oldScreenshot := chromeScreenshot
	oldOpen := browserOpen
	chromeScreenshot = func(_, _ string) bool { return false }
	browserOpen = func(string) error { return nil }
	code := m.Run()
	chromeScreenshot = oldScreenshot
	browserOpen = oldOpen
	os.Exit(code)
}

func TestRender_Bar(t *testing.T) {
	spec := ChartSpec{
		Title:      "Test Bar Chart",
		Type:       "bar",
		Categories: []string{"A", "B", "C"},
		Series: []Series{
			{Name: "S1", Values: []float64{10, 20, 30}},
		},
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "bar.html")
	if err := Render(spec, out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(data), "echarts") {
		t.Error("output does not contain echarts reference")
	}
	if !contains(string(data), "Test Bar Chart") {
		t.Error("output does not contain title")
	}
}

func TestRender_Line(t *testing.T) {
	spec := ChartSpec{
		Title:      "Line Chart",
		Type:       "line",
		Categories: []string{"Q1", "Q2", "Q3", "Q4"},
		Series: []Series{
			{Name: "Revenue", Values: []float64{100, 200, 150, 300}},
		},
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "line.html")
	if err := Render(spec, out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal("output file not created")
	}
}

func TestRender_Area(t *testing.T) {
	spec := ChartSpec{
		Title:      "Area Chart",
		Type:       "area",
		Categories: []string{"Jan", "Feb", "Mar"},
		Series: []Series{
			{Name: "Growth", Values: []float64{5, 10, 15}},
		},
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "area.html")
	if err := Render(spec, out); err != nil {
		t.Fatal(err)
	}
}

func TestRender_Pie(t *testing.T) {
	spec := ChartSpec{
		Title: "Pie Chart",
		Type:  "pie",
		Items: []Item{
			{Label: "A", Value: 40},
			{Label: "B", Value: 30},
			{Label: "C", Value: 30},
		},
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "pie.html")
	if err := Render(spec, out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(data), "pie") {
		t.Error("pie output does not contain pie type")
	}
}

func TestRender_InvalidType(t *testing.T) {
	spec := ChartSpec{Type: "invalid", Title: "Bad"}
	dir := t.TempDir()
	out := filepath.Join(dir, "bad.html")
	err := Render(spec, out)
	if err == nil {
		t.Fatal("expected error for invalid chart type")
	}
}

func TestParseSpec_ValidJSON(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "spec.json")
	jsonContent := `{"title":"Test","type":"bar","categories":["X"],"series":[{"name":"S","data":[1]}]}`
	if err := os.WriteFile(in, []byte(jsonContent), 0644); err != nil {
		t.Fatal(err)
	}
	spec, err := ParseSpec(in)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Title != "Test" {
		t.Errorf("title = %q, want %q", spec.Title, "Test")
	}
	if spec.Type != "bar" {
		t.Errorf("type = %q, want %q", spec.Type, "bar")
	}
}

func TestParseSpec_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "bad.json")
	os.WriteFile(in, []byte(`{invalid`), 0644)
	_, err := ParseSpec(in)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseSpec_Nonexistent(t *testing.T) {
	_, err := ParseSpec("/nonexistent/path/spec.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestRender_EmptyCategories(t *testing.T) {
	spec := ChartSpec{
		Title:      "Empty",
		Type:       "bar",
		Categories: []string{},
		Series:     []Series{{Name: "S", Values: []float64{}}},
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "empty.html")
	if err := Render(spec, out); err != nil {
		t.Fatal(err)
	}
}

func TestRender_DarkTheme(t *testing.T) {
	spec := ChartSpec{
		Title:      "Dark",
		Type:       "bar",
		Categories: []string{"A"},
		Series:     []Series{{Name: "S", Values: []float64{1}}},
		Theme:      "dark",
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "dark.html")
	if err := Render(spec, out); err != nil {
		t.Fatal(err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || indexOfContains(s, substr) >= 0)
}

func indexOfContains(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
