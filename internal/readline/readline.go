package readline

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/term"

	"github.com/yourusername/gosh/internal/builtin"
	"github.com/yourusername/gosh/internal/env"
)

// Cores ANSI
const (
	colorReset   = "\033[0m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
	colorBold    = "\033[1m"
	colorDim     = "\033[2m"
)

// Editor gerencia a edição de linha de comando
type Editor struct {
	e       *env.Env
	history []string
	histIdx int
	buf     []rune
	pos     int
	prompt  string

	// Estado para buscas e completions
	lastTab   string
	tabCycle  []string
	tabIdx    int
}

// New cria um Editor
func New(e *env.Env) *Editor {
	return &Editor{e: e}
}

// AddHistory adiciona uma entrada ao histórico
func (ed *Editor) AddHistory(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	// Não duplica entradas consecutivas
	if len(ed.history) > 0 && ed.history[len(ed.history)-1] == line {
		return
	}
	ed.history = append(ed.history, line)
	builtin.HistoryList = ed.history
}

// SetHistory define a lista de histórico
func (ed *Editor) SetHistory(h []string) {
	ed.history = h
	builtin.HistoryList = h
}

// ReadLine lê uma linha com edição interativa
func (ed *Editor) ReadLine(prompt string) (string, error) {
	ed.prompt = prompt
	ed.buf = nil
	ed.pos = 0
	ed.histIdx = len(ed.history)
	ed.tabCycle = nil
	ed.lastTab = ""

	// Coloca terminal em modo raw
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		// Fallback para leitura simples
		return ed.readSimple(prompt)
	}
	defer term.Restore(fd, oldState)

	// Exibe prompt com highlight
	fmt.Print(prompt)

	for {
		b := make([]byte, 8)
		n, err := os.Stdin.Read(b)
		if err != nil {
			return "", err
		}

		key := b[:n]

		// ESC sequences
		if key[0] == 27 && n > 1 && key[1] == '[' {
			switch {
			case n >= 3 && key[2] == 'A': // Up arrow
				ed.historyUp()
			case n >= 3 && key[2] == 'B': // Down arrow
				ed.historyDown()
			case n >= 3 && key[2] == 'C': // Right arrow
				if ed.pos < len(ed.buf) {
					ed.pos++
				}
			case n >= 3 && key[2] == 'D': // Left arrow
				if ed.pos > 0 {
					ed.pos--
				}
			case n >= 3 && key[2] == 'H': // Home
				ed.pos = 0
			case n >= 3 && key[2] == 'F': // End
				ed.pos = len(ed.buf)
			case n >= 4 && key[2] == '3' && key[3] == '~': // Delete
				if ed.pos < len(ed.buf) {
					ed.buf = append(ed.buf[:ed.pos], ed.buf[ed.pos+1:]...)
				}
			}
			ed.redraw()
			continue
		}

		ch := rune(key[0])

		switch ch {
		case '\r', '\n': // Enter
			fmt.Print("\r\n")
			return string(ed.buf), nil

		case 3: // Ctrl+C
			fmt.Print("^C\r\n")
			ed.buf = nil
			ed.pos = 0
			fmt.Print(prompt)
			continue

		case 4: // Ctrl+D
			if len(ed.buf) == 0 {
				fmt.Print("\r\n")
				return "", fmt.Errorf("EOF")
			}
			// Delete char at cursor
			if ed.pos < len(ed.buf) {
				ed.buf = append(ed.buf[:ed.pos], ed.buf[ed.pos+1:]...)
			}

		case 127, 8: // Backspace
			if ed.pos > 0 {
				ed.buf = append(ed.buf[:ed.pos-1], ed.buf[ed.pos:]...)
				ed.pos--
			}

		case 9: // Tab
			ed.handleTab()
			continue

		case 1: // Ctrl+A - início da linha
			ed.pos = 0

		case 5: // Ctrl+E - fim da linha
			ed.pos = len(ed.buf)

		case 11: // Ctrl+K - apaga até o fim
			ed.buf = ed.buf[:ed.pos]

		case 21: // Ctrl+U - apaga até o início
			ed.buf = ed.buf[ed.pos:]
			ed.pos = 0

		case 23: // Ctrl+W - apaga palavra anterior
			ed.deleteWordBack()

		case 12: // Ctrl+L - limpa tela
			fmt.Print("\033[2J\033[H")
			ed.redraw()
			continue

		case 18: // Ctrl+R - busca reversa no histórico
			result, err := ed.reverseSearch()
			if err == nil && result != "" {
				ed.buf = []rune(result)
				ed.pos = len(ed.buf)
			}
			fmt.Print(prompt)
			ed.redraw()
			continue

		default:
			if ch >= 32 || ch > 127 {
				// Caractere normal
				ed.buf = append(ed.buf[:ed.pos], append([]rune{ch}, ed.buf[ed.pos:]...)...)
				ed.pos++
				// Reset tab completion quando digita
				ed.tabCycle = nil
				ed.lastTab = ""
			}
		}

		ed.redraw()
	}
}

func (ed *Editor) readSimple(prompt string) (string, error) {
	fmt.Print(prompt)
	var line string
	_, err := fmt.Scanln(&line)
	return line, err
}

func (ed *Editor) redraw() {
	// Vai para início da linha e limpa
	fmt.Print("\r\033[K")
	fmt.Print(ed.prompt)
	// Escreve buffer com syntax highlight
	fmt.Print(highlight(string(ed.buf)))
	// Posiciona cursor
	promptLen := visibleLen(ed.prompt)
	cursorPos := promptLen + ed.pos
	fmt.Printf("\r\033[%dC", cursorPos)
}

func (ed *Editor) historyUp() {
	if ed.histIdx > 0 {
		ed.histIdx--
		ed.buf = []rune(ed.history[ed.histIdx])
		ed.pos = len(ed.buf)
	}
}

func (ed *Editor) historyDown() {
	if ed.histIdx < len(ed.history)-1 {
		ed.histIdx++
		ed.buf = []rune(ed.history[ed.histIdx])
		ed.pos = len(ed.buf)
	} else {
		ed.histIdx = len(ed.history)
		ed.buf = nil
		ed.pos = 0
	}
}

func (ed *Editor) deleteWordBack() {
	if ed.pos == 0 {
		return
	}
	end := ed.pos
	// Pula espaços
	for ed.pos > 0 && ed.buf[ed.pos-1] == ' ' {
		ed.pos--
	}
	// Apaga palavra
	for ed.pos > 0 && ed.buf[ed.pos-1] != ' ' {
		ed.pos--
	}
	ed.buf = append(ed.buf[:ed.pos], ed.buf[end:]...)
}

// ─── Tab Completion ───────────────────────────────────────────────────────────

func (ed *Editor) handleTab() {
	line := string(ed.buf[:ed.pos])

	// Se já estava ciclando, avança
	if ed.lastTab == line && len(ed.tabCycle) > 0 {
		ed.tabIdx = (ed.tabIdx + 1) % len(ed.tabCycle)
		ed.applyCompletion(ed.tabCycle[ed.tabIdx])
		return
	}

	ed.lastTab = line
	ed.tabIdx = 0
	completions := ed.complete(line)
	ed.tabCycle = completions

	if len(completions) == 0 {
		// Beep
		fmt.Print("\007")
		return
	}

	if len(completions) == 1 {
		ed.applyCompletion(completions[0])
		ed.tabCycle = nil
		return
	}

	// Múltiplas: mostra lista e aplica comum
	prefix := commonPrefix(completions)
	if prefix != getCurrentToken(line) {
		ed.applyCompletion(prefix)
		return
	}

	// Exibe opções
	fmt.Print("\r\n")
	for _, c := range completions {
		fmt.Printf("%-20s", c)
	}
	fmt.Print("\r\n")
	fmt.Print(ed.prompt)
	ed.redraw()
}

func (ed *Editor) applyCompletion(comp string) {
	line := string(ed.buf)
	// Substitui o token atual
	tokenStart := strings.LastIndexFunc(line[:ed.pos], func(r rune) bool {
		return r == ' ' || r == '/' || r == '='
	})
	if tokenStart < 0 {
		tokenStart = 0
	} else {
		tokenStart++
	}

	newLine := line[:tokenStart] + comp
	rest := line[ed.pos:]
	ed.buf = []rune(newLine + rest)
	ed.pos = len([]rune(newLine))
	ed.redraw()
}

func getCurrentToken(line string) string {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return ""
	}
	if strings.HasSuffix(line, " ") {
		return ""
	}
	return parts[len(parts)-1]
}

func (ed *Editor) complete(line string) []string {
	token := getCurrentToken(line)
	isFirstToken := len(strings.Fields(line)) <= 1 && !strings.HasSuffix(line, " ")

	if isFirstToken {
		return ed.completeCommand(token)
	}
	return ed.completePath(token)
}

func (ed *Editor) completeCommand(prefix string) []string {
	var completions []string
	seen := make(map[string]bool)

	// Builtins
	for name := range builtin.Registry {
		if strings.HasPrefix(name, prefix) && !seen[name] {
			completions = append(completions, name)
			seen[name] = true
		}
	}

	// Aliases
	for name := range builtin.Aliases {
		if strings.HasPrefix(name, prefix) && !seen[name] {
			completions = append(completions, name)
			seen[name] = true
		}
	}

	// PATH
	pathVar, _ := ed.e.Get("PATH")
	for _, dir := range strings.Split(pathVar, ":") {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasPrefix(name, prefix) && !seen[name] {
				// Verifica se é executável
				info, err := entry.Info()
				if err == nil && info.Mode()&0111 != 0 {
					completions = append(completions, name)
					seen[name] = true
				}
			}
		}
	}

	// Histórico
	for _, h := range ed.history {
		first := strings.Fields(h)
		if len(first) > 0 && strings.HasPrefix(first[0], prefix) && !seen[first[0]] {
			completions = append(completions, first[0])
			seen[first[0]] = true
		}
	}

	sort.Strings(completions)
	return completions
}

func (ed *Editor) completePath(prefix string) []string {
	// Expande ~
	expanded := prefix
	if strings.HasPrefix(prefix, "~") {
		home, _ := ed.e.Get("HOME")
		expanded = home + prefix[1:]
	}

	dir := filepath.Dir(expanded)
	base := filepath.Base(expanded)

	if expanded == "" || strings.HasSuffix(prefix, "/") {
		dir = expanded
		base = ""
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		// Tenta diretório atual
		entries, err = os.ReadDir(".")
		if err != nil {
			return nil
		}
		dir = "."
	}

	var completions []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, base) {
			full := filepath.Join(dir, name)
			if strings.HasPrefix(prefix, "~") {
				home, _ := ed.e.Get("HOME")
				full = "~" + strings.TrimPrefix(full, home)
			}
			if entry.IsDir() {
				full += "/"
			}
			completions = append(completions, full)
		}
	}

	sort.Strings(completions)
	return completions
}

// ─── Reverse Search ───────────────────────────────────────────────────────────

func (ed *Editor) reverseSearch() (string, error) {
	query := ""
	for {
		fmt.Printf("\r\033[K(reverse-i-search)`%s': ", query)
		// Encontra match no histórico
		match := ""
		for i := len(ed.history) - 1; i >= 0; i-- {
			if strings.Contains(ed.history[i], query) {
				match = ed.history[i]
				break
			}
		}
		fmt.Print(match)

		b := make([]byte, 4)
		n, err := os.Stdin.Read(b)
		if err != nil {
			return "", err
		}

		ch := rune(b[0])
		if n == 1 {
			switch ch {
			case '\r', '\n':
				return match, nil
			case 18: // Ctrl+R de novo - busca anterior
				// TODO: busca anterior
			case 7, 27: // Ctrl+G ou ESC - cancela
				return "", nil
			case 127, 8: // Backspace
				if len(query) > 0 {
					query = query[:len(query)-1]
				}
			default:
				if ch >= 32 {
					query += string(ch)
				}
			}
		}
	}
}

// ─── Syntax Highlighting ─────────────────────────────────────────────────────

// highlight aplica cores ao input do shell
func highlight(line string) string {
	if line == "" {
		return ""
	}

	var result strings.Builder
	runes := []rune(line)
	i := 0

	// Analisa token a token
	for i < len(runes) {
		// String entre aspas simples
		if runes[i] == '\'' {
			result.WriteString(colorYellow)
			result.WriteRune(runes[i])
			i++
			for i < len(runes) && runes[i] != '\'' {
				result.WriteRune(runes[i])
				i++
			}
			if i < len(runes) {
				result.WriteRune(runes[i])
				i++
			}
			result.WriteString(colorReset)
			continue
		}

		// String entre aspas duplas
		if runes[i] == '"' {
			result.WriteString(colorYellow)
			result.WriteRune(runes[i])
			i++
			for i < len(runes) && runes[i] != '"' {
				if runes[i] == '$' {
					result.WriteString(colorCyan)
					for i < len(runes) && runes[i] != '"' && !unicode.IsSpace(runes[i]) {
						result.WriteRune(runes[i])
						i++
					}
					result.WriteString(colorYellow)
				} else {
					result.WriteRune(runes[i])
					i++
				}
			}
			if i < len(runes) {
				result.WriteRune(runes[i])
				i++
			}
			result.WriteString(colorReset)
			continue
		}

		// Comentário
		if runes[i] == '#' {
			result.WriteString(colorDim)
			for i < len(runes) {
				result.WriteRune(runes[i])
				i++
			}
			result.WriteString(colorReset)
			continue
		}

		// Variável
		if runes[i] == '$' {
			result.WriteString(colorCyan)
			result.WriteRune(runes[i])
			i++
			for i < len(runes) && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_' || runes[i] == '{' || runes[i] == '}') {
				result.WriteRune(runes[i])
				i++
			}
			result.WriteString(colorReset)
			continue
		}

		// Operadores
		if runes[i] == '|' || runes[i] == '&' || runes[i] == ';' || runes[i] == '>' || runes[i] == '<' {
			result.WriteString(colorMagenta)
			result.WriteRune(runes[i])
			i++
			if i < len(runes) && (runes[i] == '|' || runes[i] == '&' || runes[i] == '>') {
				result.WriteRune(runes[i])
				i++
			}
			result.WriteString(colorReset)
			continue
		}

		// Palavra
		if unicode.IsLetter(runes[i]) || runes[i] == '_' || runes[i] == '/' || runes[i] == '.' || runes[i] == '-' {
			start := i
			for i < len(runes) && !unicode.IsSpace(runes[i]) && runes[i] != '|' && runes[i] != '&' && runes[i] != ';' && runes[i] != '>' && runes[i] != '<' && runes[i] != '"' && runes[i] != '\'' {
				i++
			}
			word := string(runes[start:i])
			result.WriteString(colorWord(word, start == 0 || (start > 0 && isAfterOp(runes, start))))
			continue
		}

		result.WriteRune(runes[i])
		i++
	}

	return result.String()
}

func colorWord(word string, isCmd bool) string {
	keywords := map[string]bool{
		"if": true, "then": true, "else": true, "elif": true, "fi": true,
		"for": true, "do": true, "done": true, "while": true, "until": true,
		"case": true, "esac": true, "function": true, "in": true,
	}

	if keywords[word] {
		return colorBold + colorBlue + word + colorReset
	}

	if isCmd {
		// Verifica se o comando existe
		if _, ok := builtin.Registry[word]; ok {
			return colorGreen + word + colorReset
		}
		// Checa PATH
		if _, err := findInPath(word); err == nil {
			return colorGreen + word + colorReset
		}
		return colorRed + word + colorReset
	}

	return word
}

func isAfterOp(runes []rune, pos int) bool {
	// Volta atrás pulando espaços
	i := pos - 1
	for i >= 0 && runes[i] == ' ' {
		i--
	}
	if i < 0 {
		return true
	}
	return runes[i] == '|' || runes[i] == '&' || runes[i] == ';' || runes[i] == '(' || runes[i] == '{'
}

func findInPath(name string) (string, error) {
	pathEnv := os.Getenv("PATH")
	for _, dir := range strings.Split(pathEnv, ":") {
		full := filepath.Join(dir, name)
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			return full, nil
		}
	}
	return "", fmt.Errorf("não encontrado")
}

// visibleLen calcula o comprimento visível de uma string com escape codes
func visibleLen(s string) int {
	inEsc := false
	n := 0
	for _, r := range s {
		if r == '\033' {
			inEsc = true
		}
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		n++
	}
	return n
}

func commonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	prefix := strs[0]
	for _, s := range strs[1:] {
		for !strings.HasPrefix(s, prefix) {
			prefix = prefix[:len(prefix)-1]
			if prefix == "" {
				return ""
			}
		}
	}
	return prefix
}

func (ed *Editor) GetHistory() []string {
	return ed.history
}

