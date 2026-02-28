package env

import (
	"fmt"
	"math/rand"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Env gerencia variáveis de ambiente do shell
type Env struct {
	mu      sync.RWMutex
	vars    map[string]string
	exports map[string]bool // variáveis marcadas para exportar
	readonly map[string]bool
}

// New cria um novo Env inicializado com o ambiente do processo
func New() *Env {
	e := &Env{
		vars:    make(map[string]string),
		exports: make(map[string]bool),
		readonly: make(map[string]bool),
	}
	// Importa variáveis do ambiente atual
	for _, kv := range os.Environ() {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			e.vars[parts[0]] = parts[1]
			e.exports[parts[0]] = true
		}
	}
	// Define variáveis especiais do shell
	e.setShellVars()
	return e
}

func (e *Env) setShellVars() {
	if _, ok := e.vars["PS1"]; !ok {
		e.vars["PS1"] = `\u@\h:\w$ `
	}
	if _, ok := e.vars["IFS"]; !ok {
		e.vars["IFS"] = " \t\n"
	}
	if _, ok := e.vars["PATH"]; !ok {
		e.vars["PATH"] = "/usr/local/bin:/usr/bin:/bin"
	}
	e.vars["GOSH_VERSION"] = "0.1.0"
	e.vars["GOSH_PID"] = strconv.Itoa(os.Getpid())

	u, _ := user.Current()
	if u != nil {
		e.vars["HOME"] = u.HomeDir
		e.vars["USER"] = u.Username
		e.vars["LOGNAME"] = u.Username
	}
}

// Get retorna uma variável
func (e *Env) Get(name string) (string, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	v, ok := e.vars[name]
	return v, ok
}

// Set define uma variável
func (e *Env) Set(name, value string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.readonly[name] {
		return fmt.Errorf("%s: variável somente leitura", name)
	}
	e.vars[name] = value
	if e.exports[name] {
		os.Setenv(name, value)
	}
	return nil
}

// Unset remove uma variável
func (e *Env) Unset(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.readonly[name] {
		return fmt.Errorf("%s: variável somente leitura", name)
	}
	delete(e.vars, name)
	delete(e.exports, name)
	os.Unsetenv(name)
	return nil
}

// Export marca variável para exportar ao ambiente filho
func (e *Env) Export(name, value string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.vars[name] = value
	e.exports[name] = true
	os.Setenv(name, value)
}

// SetReadonly marca variável como somente leitura
func (e *Env) SetReadonly(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.readonly[name] = true
}

// Environ retorna as variáveis exportadas no formato KEY=VALUE
func (e *Env) Environ() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var env []string
	for k, v := range e.vars {
		if e.exports[k] {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// All retorna todas as variáveis (para `set`)
func (e *Env) All() map[string]string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make(map[string]string, len(e.vars))
	for k, v := range e.vars {
		result[k] = v
	}
	return result
}

// Clone cria um Env filho (para subshells)
func (e *Env) Clone() *Env {
	e.mu.RLock()
	defer e.mu.RUnlock()
	child := &Env{
		vars:    make(map[string]string, len(e.vars)),
		exports: make(map[string]bool, len(e.exports)),
		readonly: make(map[string]bool, len(e.readonly)),
	}
	for k, v := range e.vars {
		child.vars[k] = v
	}
	for k, v := range e.exports {
		child.exports[k] = v
	}
	for k, v := range e.readonly {
		child.readonly[k] = v
	}
	return child
}

// ─── Expansão ─────────────────────────────────────────────────────────────────

// Expand expande uma string com variáveis, subshells, aritmética e glob
func (e *Env) Expand(s string) string {
	return e.expandString(s)
}

func (e *Env) expandString(s string) string {
	if !strings.ContainsAny(s, "$~") {
		return s
	}

	var sb strings.Builder
	runes := []rune(s)
	i := 0

	for i < len(runes) {
		ch := runes[i]

		// Tilde expansion no início
		if ch == '~' && i == 0 {
			end := i + 1
			for end < len(runes) && runes[end] != '/' && runes[end] != ':' {
				end++
			}
			username := string(runes[1:end])
			home := e.expandTilde(username)
			sb.WriteString(home)
			i = end
			continue
		}

		if ch != '$' {
			sb.WriteRune(ch)
			i++
			continue
		}

		i++ // pula $
		if i >= len(runes) {
			sb.WriteRune('$')
			break
		}

		next := runes[i]

		switch next {
		case '{':
			// ${VAR} ou ${VAR:-default} etc
			i++
			end := i
			depth := 1
			for end < len(runes) && depth > 0 {
				if runes[end] == '{' {
					depth++
				} else if runes[end] == '}' {
					depth--
				}
				if depth > 0 {
					end++
				}
			}
			expr := string(runes[i:end])
			sb.WriteString(e.expandBrace(expr))
			i = end + 1

		case '(':
			if i+1 < len(runes) && runes[i+1] == '(' {
				// Aritmética $((...))
				i += 2
				end := i
				depth := 2
				for end < len(runes) && depth > 0 {
					if runes[end] == '(' {
						depth++
					} else if runes[end] == ')' {
						depth--
					}
					if depth > 0 {
						end++
					}
				}
				expr := string(runes[i:end])
				result := e.evalArith(expr)
				sb.WriteString(strconv.FormatInt(result, 10))
				i = end + 2
			} else {
				// Subshell $(...) - placeholder
				i++
				end := i
				depth := 1
				for end < len(runes) && depth > 0 {
					if runes[end] == '(' {
						depth++
					} else if runes[end] == ')' {
						depth--
					}
					if depth > 0 {
						end++
					}
				}
				// O executor lida com subshells reais
				sb.WriteString("$(" + string(runes[i:end]) + ")")
				i = end + 1
			}

		case '?':
			v, _ := e.Get("?")
			sb.WriteString(v)
			i++

		case '$':
			v, _ := e.Get("GOSH_PID")
			sb.WriteString(v)
			i++

		case '!':
			v, _ := e.Get("!")
			sb.WriteString(v)
			i++

		case '#':
			v, _ := e.Get("#")
			sb.WriteString(v)
			i++

		case '*':
			v, _ := e.Get("*")
			sb.WriteString(v)
			i++

		case '@':
			v, _ := e.Get("@")
			sb.WriteString(v)
			i++

		default:
			if (next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') || next == '_' || (next >= '0' && next <= '9') {
				end := i
				for end < len(runes) && (isVarChar(runes[end])) {
					end++
				}
				name := string(runes[i:end])
				v, _ := e.Get(name)
				sb.WriteString(v)
				i = end
			} else {
				sb.WriteRune('$')
			}
		}
	}

	return sb.String()
}

func isVarChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

func (e *Env) expandTilde(username string) string {
	if username == "" {
		if home, ok := e.Get("HOME"); ok {
			return home
		}
		u, err := user.Current()
		if err == nil {
			return u.HomeDir
		}
		return "~"
	}
	u, err := user.Lookup(username)
	if err != nil {
		return "~" + username
	}
	return u.HomeDir
}

func (e *Env) expandBrace(expr string) string {
	// ${VAR:-default}
	if idx := strings.Index(expr, ":-"); idx >= 0 {
		name := expr[:idx]
		def := expr[idx+2:]
		if v, ok := e.Get(name); ok && v != "" {
			return v
		}
		return e.expandString(def)
	}
	// ${VAR:=default}
	if idx := strings.Index(expr, ":="); idx >= 0 {
		name := expr[:idx]
		def := expr[idx+2:]
		if v, ok := e.Get(name); ok && v != "" {
			return v
		}
		val := e.expandString(def)
		e.Set(name, val)
		return val
	}
	// ${VAR:+alt}
	if idx := strings.Index(expr, ":+"); idx >= 0 {
		name := expr[:idx]
		alt := expr[idx+2:]
		if v, ok := e.Get(name); ok && v != "" {
			return e.expandString(alt)
		}
		return ""
	}
	// ${VAR:?message}
	if idx := strings.Index(expr, ":?"); idx >= 0 {
		name := expr[:idx]
		msg := expr[idx+2:]
		if v, ok := e.Get(name); ok && v != "" {
			return v
		}
		fmt.Fprintf(os.Stderr, "gosh: %s: %s\n", name, msg)
		os.Exit(1)
	}
	// ${#VAR} - comprimento
	if strings.HasPrefix(expr, "#") {
		name := expr[1:]
		v, _ := e.Get(name)
		return strconv.Itoa(len([]rune(v)))
	}
	// ${VAR%pattern}, ${VAR%%pattern}
	if idx := strings.Index(expr, "%%"); idx >= 0 {
		name := expr[:idx]
		v, _ := e.Get(name)
		// Remove sufixo mais longo - simplificado
		return v
	}
	if idx := strings.Index(expr, "%"); idx >= 0 {
		name := expr[:idx]
		v, _ := e.Get(name)
		return v
	}
	// Variável simples
	v, _ := e.Get(expr)
	return v
}

// ─── Aritmética ───────────────────────────────────────────────────────────────

// evalArith avalia expressões aritméticas simples
func (e *Env) evalArith(expr string) int64 {
	expr = strings.TrimSpace(e.expandString(expr))
	return evalArithExpr(expr, e)
}

func evalArithExpr(expr string, env *Env) int64 {
	// Suporte a operações básicas: +, -, *, /, %, ** e variáveis
	return parseArithOr(expr, env)
}

func parseArithOr(expr string, env *Env) int64 {
	return parseArithAnd(expr, env)
}

func parseArithAnd(expr string, env *Env) int64 {
	return parseArithAdd(expr, env)
}

func parseArithAdd(expr string, env *Env) int64 {
	expr = strings.TrimSpace(expr)
	// Procura + ou - de mais baixa precedência
	for i := len(expr) - 1; i >= 0; i-- {
		if (expr[i] == '+' || expr[i] == '-') && i > 0 {
			left := parseArithMul(expr[:i], env)
			right := parseArithMul(expr[i+1:], env)
			if expr[i] == '+' {
				return left + right
			}
			return left - right
		}
	}
	return parseArithMul(expr, env)
}

func parseArithMul(expr string, env *Env) int64 {
	expr = strings.TrimSpace(expr)
	for i := len(expr) - 1; i >= 0; i-- {
		if expr[i] == '*' || expr[i] == '/' || expr[i] == '%' {
			left := parseArithAtom(strings.TrimSpace(expr[:i]), env)
			right := parseArithAtom(strings.TrimSpace(expr[i+1:]), env)
			switch expr[i] {
			case '*':
				return left * right
			case '/':
				if right == 0 {
					return 0
				}
				return left / right
			case '%':
				if right == 0 {
					return 0
				}
				return left % right
			}
		}
	}
	return parseArithAtom(expr, env)
}

func parseArithAtom(expr string, env *Env) int64 {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return 0
	}
	// Tenta número
	if n, err := strconv.ParseInt(expr, 10, 64); err == nil {
		return n
	}
	// Variável
	if v, ok := env.Get(expr); ok {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	// RANDOM
	if expr == "RANDOM" {
		return int64(rand.Intn(32768))
	}
	return 0
}

// ─── Glob ─────────────────────────────────────────────────────────────────────

// ExpandGlob expande padrões glob em uma lista de arquivos
func ExpandGlob(pattern string) ([]string, error) {
	if !strings.ContainsAny(pattern, "*?[") {
		return []string{pattern}, nil
	}
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return []string{pattern}, nil
	}
	return matches, nil
}

// ExpandGlobs expande múltiplos args com glob
func ExpandGlobs(args []string) []string {
	var result []string
	for _, arg := range args {
		expanded, _ := ExpandGlob(arg)
		result = append(result, expanded...)
	}
	return result
}
