// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
)

type tokenType int

const (
	tokPlain tokenType = iota
	tokComment
	tokString
	tokNumber
	tokKeyword
	tokBoolean
)

type langConfig struct {
	re     *regexp.Regexp
	groups []tokenType
}

type syntaxStyles struct {
	keyword lipgloss.Style
	str     lipgloss.Style
	comment lipgloss.Style
	number  lipgloss.Style
	plain   lipgloss.Style
	boolean lipgloss.Style
}

type SyntaxHighlighter struct {
	theme  Theme
	styles syntaxStyles
	langs  map[string]*langConfig
	mu     sync.RWMutex
}

var highlighterInit sync.Once

// sin-debt: verify, upgrade: analyzer false positive — var is read in non-test code
var sharedLangConfigs map[string]*langConfig

func initLangConfigs() {
	sharedLangConfigs = make(map[string]*langConfig)

	goComment := `//[^\n]*|/\*[\s\S]*?\*/`
	goString := `"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'` + "|" + "`(?:[^`\\\\]|\\\\.)*`"
	goNumber := `\b\d[\d_]*\.?\d*(?:[eE][+-]?\d+)?\b`
	goKeywords := `\b(?:break|case|chan|const|continue|default|defer|else|fallthrough|for|func|go|goto|if|import|interface|map|package|range|return|select|struct|switch|type|var|nil|true|false|iota|byte|rune|error|bool|string|int|int8|int16|int32|int64|uint|uint8|uint16|uint32|uint64|uintptr|float32|float64|complex64|complex128)\b`

	sharedLangConfigs["go"] = &langConfig{
		re:     regexp.MustCompile(goComment + "|" + goString + "|" + goNumber + "|" + goKeywords),
		groups: []tokenType{tokComment, tokString, tokNumber, tokKeyword},
	}

	pyComment := `#[^\n]*`
	pyString := `"""[\s\S]*?"""|'''[\s\S]*?'''|"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'`
	pyNumber := `\b\d+\.?\d*(?:[eE][+-]?\d+)?\b`
	pyKeywords := `\b(?:def|class|if|elif|else|for|while|try|except|finally|with|import|from|as|return|yield|lambda|async|await|pass|break|continue|raise|global|nonlocal|assert|del|in|is|not|and|or|self|print|len|range|open|super|property|staticmethod|classmethod|abs|all|any|bin|bool|chr|dict|float|format|hash|hex|input|int|list|max|min|next|object|ord|pow|repr|round|set|sorted|str|sum|tuple|type|zip)\b`

	sharedLangConfigs["python"] = &langConfig{
		re:     regexp.MustCompile(pyComment + "|" + pyString + "|" + pyNumber + "|" + pyKeywords),
		groups: []tokenType{tokComment, tokString, tokNumber, tokKeyword},
	}
	sharedLangConfigs["py"] = sharedLangConfigs["python"]

	jsComment := `//[^\n]*|/\*[\s\S]*?\*/`
	jsString := `"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'` + "|" + "`(?:[^`\\\\]|\\\\.)*`"
	jsNumber := `\b\d+\.?\d*(?:[eE][+-]?\d+)?\b`
	jsKeywords := `\b(?:var|let|const|function|return|if|else|for|while|do|switch|case|break|continue|new|try|catch|finally|throw|typeof|instanceof|in|of|class|extends|super|this|import|export|from|default|async|await|yield|void|delete|static|get|set|public|private|protected|readonly|namespace|declare|abstract|as|enum|implements|interface|module|require|exports|Buffer|process|console|window|document)\b`

	sharedLangConfigs["javascript"] = &langConfig{
		re:     regexp.MustCompile(jsComment + "|" + jsString + "|" + jsNumber + "|" + jsKeywords),
		groups: []tokenType{tokComment, tokString, tokNumber, tokKeyword},
	}
	sharedLangConfigs["js"] = sharedLangConfigs["javascript"]
	sharedLangConfigs["ts"] = sharedLangConfigs["javascript"]
	sharedLangConfigs["typescript"] = sharedLangConfigs["javascript"]

	jsonString := `"(?:[^"\\]|\\.)*"`
	jsonNumber := `\b-?\d+\.?\d*(?:[eE][+-]?\d+)?\b`
	jsonBool := `\b(?:true|false|null)\b`

	sharedLangConfigs["json"] = &langConfig{
		re:     regexp.MustCompile(jsonString + "|" + jsonNumber + "|" + jsonBool),
		groups: []tokenType{tokString, tokNumber, tokBoolean},
	}

	bashComment := `#[^\n]*`
	bashString := `"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'`
	bashNumber := `\b\d+\b`
	bashKeywords := `\b(?:if|then|else|elif|fi|for|while|do|done|case|esac|function|in|return|local|export|echo|read|set|unset|shift|source|alias|unalias|exit|trap|wait|select|time|until|coproc|declare|typeset|readonly|let|eval|exec|printf|test|cd|pwd|pushd|popd|true|false)\b`

	sharedLangConfigs["bash"] = &langConfig{
		re:     regexp.MustCompile(bashComment + "|" + bashString + "|" + bashNumber + "|" + bashKeywords),
		groups: []tokenType{tokComment, tokString, tokNumber, tokKeyword},
	}
	sharedLangConfigs["sh"] = sharedLangConfigs["bash"]
	sharedLangConfigs["shell"] = sharedLangConfigs["bash"]
	sharedLangConfigs["zsh"] = sharedLangConfigs["bash"]

	yamlComment := `#[^\n]*`
	yamlString := `"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'`
	yamlNumber := `\b\d+\.?\d*\b`
	yamlKeywords := `\b(?:true|false|null|yes|no|on|off)\b`

	sharedLangConfigs["yaml"] = &langConfig{
		re:     regexp.MustCompile(yamlComment + "|" + yamlString + "|" + yamlNumber + "|" + yamlKeywords),
		groups: []tokenType{tokComment, tokString, tokNumber, tokBoolean},
	}
	sharedLangConfigs["yml"] = sharedLangConfigs["yaml"]

	rustComment := `//[^\n]*|/\*[\s\S]*?\*/`
	rustString := `"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'`
	rustNumber := `\b\d+\.?\d*(?:[eE][+-]?\d+)?\b`
	rustKeywords := `\b(?:fn|let|mut|const|static|struct|enum|impl|trait|pub|use|mod|match|if|else|for|while|loop|break|continue|return|as|in|ref|move|where|async|await|dyn|unsafe|self|Self|true|false|crate|extern|super|type|box|i8|i16|i32|i64|i128|isize|u8|u16|u32|u64|u128|usize|f32|f64|bool|char|str|String|Vec|Option|Result|Some|None|Ok|Err)\b`

	sharedLangConfigs["rust"] = &langConfig{
		re:     regexp.MustCompile(rustComment + "|" + rustString + "|" + rustNumber + "|" + rustKeywords),
		groups: []tokenType{tokComment, tokString, tokNumber, tokKeyword},
	}
	sharedLangConfigs["rs"] = sharedLangConfigs["rust"]
}

func NewSyntaxHighlighter(theme Theme) *SyntaxHighlighter {
	highlighterInit.Do(initLangConfigs)

	h := &SyntaxHighlighter{
		theme: theme,
		langs: sharedLangConfigs,
	}

	h.styles = syntaxStyles{
		keyword: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Accent)).Bold(true),
		str:     lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Success)),
		comment: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.TextDim)).Italic(true),
		number:  lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Warn)),
		plain:   lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Text)),
		boolean: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.AccentDim)).Bold(true),
	}

	return h
}

func (h *SyntaxHighlighter) Highlight(code, language string) string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if code == "" {
		return ""
	}

	lang := strings.ToLower(strings.TrimSpace(language))
	if lang == "" {
		return h.styles.plain.Render(code)
	}

	cfg, ok := h.langs[lang]
	if !ok {
		return h.styles.plain.Render(code)
	}

	return h.tokenize(code, cfg)
}

func (h *SyntaxHighlighter) tokenize(code string, cfg *langConfig) string {
	matches := cfg.re.FindAllStringSubmatchIndex(code, -1)
	if len(matches) == 0 {
		return h.styles.plain.Render(code)
	}

	var b strings.Builder
	lastEnd := 0

	for _, m := range matches {
		matchStart := m[0]
		matchEnd := m[1]

		if matchStart > lastEnd {
			b.WriteString(h.styles.plain.Render(code[lastEnd:matchStart]))
		}

		tt := h.matchType(m, cfg.groups)
		matched := code[matchStart:matchEnd]
		b.WriteString(h.renderToken(matched, tt))

		lastEnd = matchEnd
	}

	if lastEnd < len(code) {
		b.WriteString(h.styles.plain.Render(code[lastEnd:]))
	}

	return b.String()
}

func (h *SyntaxHighlighter) matchType(m []int, groups []tokenType) tokenType {
	for i, tt := range groups {
		idx := 2 * (i + 1)
		if idx+1 < len(m) && m[idx] >= 0 {
			return tt
		}
	}
	return tokPlain
}

func (h *SyntaxHighlighter) renderToken(text string, tt tokenType) string {
	switch tt {
	case tokKeyword:
		return h.styles.keyword.Render(text)
	case tokString:
		return h.styles.str.Render(text)
	case tokComment:
		return h.styles.comment.Render(text)
	case tokNumber:
		return h.styles.number.Render(text)
	case tokBoolean:
		return h.styles.boolean.Render(text)
	default:
		return h.styles.plain.Render(text)
	}
}

func (h *SyntaxHighlighter) SupportedLanguages() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	seen := make(map[string]bool)
	var langs []string
	for lang := range h.langs {
		if !seen[lang] {
			seen[lang] = true
			langs = append(langs, lang)
		}
	}
	return langs
}

func (h *SyntaxHighlighter) SupportsLanguage(language string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	lang := strings.ToLower(strings.TrimSpace(language))
	_, ok := h.langs[lang]
	return ok
}

type codeBlock struct {
	language string
	code     string
}

func extractCodeBlocks(text string) []codeBlock {
	var blocks []codeBlock

	lines := strings.Split(text, "\n")
	var b strings.Builder
	inBlock := false
	var blockLang string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if !inBlock {
				inBlock = true
				blockLang = strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
				b.Reset()
			} else {
				code := b.String()
				code = strings.TrimSuffix(code, "\n")
				blocks = append(blocks, codeBlock{
					language: blockLang,
					code:     code,
				})
				inBlock = false
				blockLang = ""
			}
		} else if inBlock {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(line)
		}
	}

	return blocks
}

func renderCodeBlock(code, language string, h *SyntaxHighlighter, styles Styles, width int, showLineNumbers bool) string {
	if width < 10 {
		width = 10
	}

	innerWidth := width - 8
	if innerWidth < 10 {
		innerWidth = 10
	}

	var highlighted string
	if h != nil {
		highlighted = h.Highlight(code, language)
	} else {
		highlighted = styles.Content.Render(code)
	}

	highlightLines := strings.Split(highlighted, "\n")
	codeLines := strings.Split(code, "\n")

	var bodyLines []string
	maxLines := len(highlightLines)
	if len(codeLines) > maxLines {
		maxLines = len(codeLines)
	}

	for i := 0; i < maxLines; i++ {
		var hl, num string
		if i < len(highlightLines) {
			hl = highlightLines[i]
		}
		if showLineNumbers {
			num = styles.Muted.Render(fmt.Sprintf("%3d ", i+1))
		} else {
			num = "  "
		}
		bodyLines = append(bodyLines, num+hl)
	}
	body := strings.Join(bodyLines, "\n")

	langLabel := ""
	if language != "" {
		langLabel = strings.ToLower(strings.TrimSpace(language))
	}

	var hdr strings.Builder
	hdr.WriteString(styles.Muted.Render("┌─ "))
	if langLabel != "" {
		hdr.WriteString(styles.AccentText.Render(langLabel))
	}
	remaining := innerWidth - 3 - len(langLabel)
	if remaining < 0 {
		remaining = 0
	}
	hdr.WriteString(strings.Repeat("─", remaining+1))
	hdr.WriteString(styles.Muted.Render("┐"))

	ftr := styles.Muted.Render("└" + strings.Repeat("─", innerWidth+2) + "┘")

	return hdr.String() + "\n" + body + "\n" + ftr
}
