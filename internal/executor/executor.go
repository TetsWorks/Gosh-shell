package executor

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/yourusername/gosh/internal/builtin"
	"github.com/yourusername/gosh/internal/env"
	"github.com/yourusername/gosh/internal/jobcontrol"
	"github.com/yourusername/gosh/internal/lexer"
	"github.com/yourusername/gosh/internal/parser"
)

// Executor executa a AST do shell
type Executor struct {
	Env     *env.Env
	Jobs    *jobcontrol.Manager
	Funcs   map[string]*parser.FuncDef
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	// Flags de opções
	ExitOnError  bool // set -e
	NoUnset      bool // set -u
	Xtrace       bool // set -x
}

// New cria um Executor
func New(e *env.Env, jm *jobcontrol.Manager) *Executor {
	return &Executor{
		Env:    e,
		Jobs:   jm,
		Funcs:  make(map[string]*parser.FuncDef),
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
}

// Clone cria um executor filho (para subshells)
func (ex *Executor) Clone() *Executor {
	child := &Executor{
		Env:         ex.Env.Clone(),
		Jobs:        ex.Jobs,
		Funcs:       ex.Funcs,
		Stdin:       ex.Stdin,
		Stdout:      ex.Stdout,
		Stderr:      ex.Stderr,
		ExitOnError: ex.ExitOnError,
		NoUnset:     ex.NoUnset,
		Xtrace:      ex.Xtrace,
	}
	return child
}

// ExecList executa uma List
func (ex *Executor) ExecList(list *parser.List) (int, error) {
	code := 0
	for _, item := range list.Items {
		var err error
		if item.Background {
			go ex.execAndOr(item.Node)
			code = 0
		} else {
			code, err = ex.execAndOr(item.Node)
			if err != nil {
				return code, err
			}
		}
		ex.Env.Set("?", strconv.Itoa(code))
		if ex.ExitOnError && code != 0 {
			return code, &builtin.ExitError{Code: code}
		}
	}
	return code, nil
}

func (ex *Executor) execAndOr(aol *parser.AndOrList) (int, error) {
	code, err := ex.execPipeline(aol.Pipelines[0])
	if err != nil {
		return code, err
	}
	for i, op := range aol.Ops {
		switch op {
		case "&&":
			if code != 0 {
				return code, nil
			}
		case "||":
			if code == 0 {
				return code, nil
			}
		}
		code, err = ex.execPipeline(aol.Pipelines[i+1])
		if err != nil {
			return code, err
		}
	}
	return code, nil
}

func (ex *Executor) execPipeline(pl *parser.Pipeline) (int, error) {
	if len(pl.Cmds) == 1 {
		code, err := ex.execNode(pl.Cmds[0], ex.Stdin, ex.Stdout, ex.Stderr)
		if pl.Negate {
			if code == 0 {
				code = 1
			} else {
				code = 0
			}
		}
		return code, err
	}

	// Cria pipes entre comandos
	readers := make([]*io.PipeReader, len(pl.Cmds)-1)
	writers := make([]*io.PipeWriter, len(pl.Cmds)-1)
	for i := range readers {
		readers[i], writers[i] = io.Pipe()
	}

	type result struct {
		code int
		err  error
	}
	results := make(chan result, len(pl.Cmds))

	for i, node := range pl.Cmds {
		var stdin io.Reader
		var stdout io.Writer

		if i == 0 {
			stdin = ex.Stdin
		} else {
			stdin = readers[i-1]
		}

		if i == len(pl.Cmds)-1 {
			stdout = ex.Stdout
		} else {
			stdout = writers[i]
		}

		go func(n parser.Node, in io.Reader, out io.Writer, idx int) {
			code, err := ex.execNode(n, in, out, ex.Stderr)
			if idx < len(writers) {
				writers[idx].Close()
			}
			results <- result{code, err}
		}(node, stdin, stdout, i)
	}

	// Coleta resultados (o último é o exit code do pipeline)
	var lastCode int
	var lastErr error
	for range pl.Cmds {
		r := <-results
		lastCode = r.code
		if r.err != nil {
			lastErr = r.err
		}
	}

	if pl.Negate {
		if lastCode == 0 {
			lastCode = 1
		} else {
			lastCode = 0
		}
	}

	return lastCode, lastErr
}

func (ex *Executor) execNode(node parser.Node, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
	switch n := node.(type) {
	case *parser.SimpleCmd:
		return ex.execSimple(n, stdin, stdout, stderr)
	case *parser.IfCmd:
		return ex.execIf(n, stdin, stdout, stderr)
	case *parser.ForCmd:
		return ex.execFor(n, stdin, stdout, stderr)
	case *parser.WhileCmd:
		return ex.execWhile(n, stdin, stdout, stderr)
	case *parser.CaseCmd:
		return ex.execCase(n, stdin, stdout, stderr)
	case *parser.FuncDef:
		ex.Funcs[n.Name] = n
		return 0, nil
	case *parser.SubshellCmd:
		return ex.execSubshell(n, stdin, stdout, stderr)
	case *parser.BraceGroup:
		return ex.execBraceGroup(n, stdin, stdout, stderr)
	case *parser.ArithCmd:
		return ex.execArith(n)
	case *parser.AndOrList:
		return ex.execAndOr(n)
	case *parser.Pipeline:
		return ex.execPipeline(n)
	}
	return 0, nil
}

// ─── Simple Command ───────────────────────────────────────────────────────────

func (ex *Executor) execSimple(cmd *parser.SimpleCmd, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
	// Atribuições sem comando
	if len(cmd.Args) == 0 {
		for _, a := range cmd.Assigns {
			val := ex.expandWord(a.Value)
			ex.Env.Set(a.Name, val)
		}
		return 0, nil
	}

	// Expande argumentos
	args, err := ex.expandArgs(cmd.Args)
	if err != nil {
		return 1, err
	}

	if len(args) == 0 {
		return 0, nil
	}

	if ex.Xtrace {
		fmt.Fprintf(ex.Stderr, "+ %s\n", strings.Join(args, " "))
	}

	// Expande alias
	if alias, ok := builtin.Aliases[args[0]]; ok {
		newLine := alias + " " + strings.Join(args[1:], " ")
		return ex.execString(newLine, stdin, stdout, stderr)
	}

	// Aplica redirecionamentos
	stdin, stdout, stderr, cleanups, err := ex.applyRedirects(cmd.Redirects, stdin, stdout, stderr)
	if err != nil {
		return 1, err
	}
	defer func() {
		for _, f := range cleanups {
			f()
		}
	}()

	// Verifica se é função definida pelo usuário
	if fn, ok := ex.Funcs[args[0]]; ok {
		return ex.execFunc(fn, args, stdin, stdout, stderr)
	}

	// Builtins
	if handler, ok := builtin.Registry[args[0]]; ok {
		return ex.runBuiltin(handler, args, stdin, stdout, stderr)
	}

	// Comando externo
	return ex.execExternal(args, cmd.Assigns, stdin, stdout, stderr)
}

func (ex *Executor) runBuiltin(handler builtin.Handler, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
	// Redireciona I/O do bulitin
	oldIn, oldOut, oldErr := ex.Stdin, ex.Stdout, ex.Stderr
	ex.Stdin, ex.Stdout, ex.Stderr = stdin, stdout, stderr
	// Redireciona os.Stdout temporariamente para builtins que usam fmt.Print
	oldOsStdout := os.Stdout
	if f, ok := stdout.(*os.File); ok {
		os.Stdout = f
	}
	result := handler(args, ex.Env, ex.Jobs)
	os.Stdout = oldOsStdout
	ex.Stdin, ex.Stdout, ex.Stderr = oldIn, oldOut, oldErr

	// Trata sinais de controle de fluxo
	switch result.Code {
	case -1: // source
		if len(args) >= 2 {
			return ex.sourceFile(args[1], args[2:])
		}
		return 1, nil
	case -2: // exec
		if len(args) >= 2 {
			return ex.doExec(args[1:])
		}
		return 0, nil
	case -3: // eval
		if len(args) >= 2 {
			return ex.execString(strings.Join(args[1:], " "), stdin, stdout, stderr)
		}
		return 0, nil
	}

	if result.Err != nil {
		return result.Code, result.Err
	}
	return result.Code, nil
}

func (ex *Executor) execExternal(args []string, assigns []*parser.Assign, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
	// Resolve caminho
	path, err := ex.resolvePath(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "gosh: %s: comando não encontrado\n", args[0])
		return 127, nil
	}

	cmd := exec.Command(path, args[1:]...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = ex.Env.Environ()

	// Atribuições locais
	for _, a := range assigns {
		val := ex.expandWord(a.Value)
		cmd.Env = append(cmd.Env, a.Name+"="+val)
	}

	// Cria novo grupo de processos
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(stderr, "gosh: %s: %v\n", args[0], err)
		return 126, nil
	}

	// Espera o processo terminar
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return 1, nil
	}
	return 0, nil
}

func (ex *Executor) resolvePath(name string) (string, error) {
	// Caminho absoluto ou relativo
	if strings.Contains(name, "/") {
		return name, nil
	}

	pathVar, _ := ex.Env.Get("PATH")
	for _, dir := range strings.Split(pathVar, ":") {
		full := filepath.Join(dir, name)
		if info, err := os.Stat(full); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return full, nil
		}
	}
	return "", fmt.Errorf("não encontrado: %s", name)
}

// ─── Redirecionamentos ────────────────────────────────────────────────────────

func (ex *Executor) applyRedirects(redirs []*parser.Redirect, stdin io.Reader, stdout io.Writer, stderr io.Writer) (io.Reader, io.Writer, io.Writer, []func(), error) {
	var cleanups []func()

	for _, r := range redirs {
		filename := ""
		if r.File != nil {
			filename = ex.Env.Expand(ex.expandWord(r.File))
		}

		switch r.Op {
		case ">":
			f, err := os.Create(filename)
			if err != nil {
				return stdin, stdout, stderr, cleanups, err
			}
			stdout = f
			cleanups = append(cleanups, func() { f.Close() })

		case ">>":
			f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return stdin, stdout, stderr, cleanups, err
			}
			stdout = f
			cleanups = append(cleanups, func() { f.Close() })

		case "<":
			f, err := os.Open(filename)
			if err != nil {
				return stdin, stdout, stderr, cleanups, err
			}
			stdin = f
			cleanups = append(cleanups, func() { f.Close() })

		case "<<", "<<-":
			stdin = strings.NewReader(r.Here)

		case "2>":
			f, err := os.Create(filename)
			if err != nil {
				return stdin, stdout, stderr, cleanups, err
			}
			stderr = f
			cleanups = append(cleanups, func() { f.Close() })

		case "2>>":
			f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return stdin, stdout, stderr, cleanups, err
			}
			stderr = f
			cleanups = append(cleanups, func() { f.Close() })

		case "&>":
			f, err := os.Create(filename)
			if err != nil {
				return stdin, stdout, stderr, cleanups, err
			}
			stdout = f
			stderr = f
			cleanups = append(cleanups, func() { f.Close() })
		}
	}

	return stdin, stdout, stderr, cleanups, nil
}

// ─── Compostos ────────────────────────────────────────────────────────────────

func (ex *Executor) execIf(cmd *parser.IfCmd, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
	code, err := ex.ExecList(cmd.Condition)
	if err != nil {
		return code, err
	}

	if code == 0 {
		return ex.ExecList(cmd.Then)
	}

	for _, elif := range cmd.Elifs {
		code, err = ex.ExecList(elif.Condition)
		if err != nil {
			return code, err
		}
		if code == 0 {
			return ex.ExecList(elif.Body)
		}
	}

	if cmd.Else != nil {
		return ex.ExecList(cmd.Else)
	}

	return 0, nil
}

func (ex *Executor) execFor(cmd *parser.ForCmd, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
	var words []string

	if len(cmd.Words) == 0 {
		// for var; do - itera sobre "$@"
		params, _ := ex.Env.Get("@")
		words = strings.Fields(params)
	} else {
		for _, w := range cmd.Words {
			expanded := ex.Env.Expand(ex.expandWord(w))
			// Glob expansion
			globbed, _ := env.ExpandGlob(expanded)
			words = append(words, globbed...)
		}
	}

	code := 0
	for _, word := range words {
		ex.Env.Set(cmd.Var, word)
		var err error
		code, err = ex.ExecList(cmd.Body)
		if err != nil {
			if brk, ok := err.(*builtin.BreakError); ok {
				if brk.N <= 1 {
					break
				}
				return 0, &builtin.BreakError{N: brk.N - 1}
			}
			if cont, ok := err.(*builtin.ContinueError); ok {
				if cont.N <= 1 {
					continue
				}
				return 0, &builtin.ContinueError{N: cont.N - 1}
			}
			return code, err
		}
	}
	return code, nil
}

func (ex *Executor) execWhile(cmd *parser.WhileCmd, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
	code := 0
	for {
		condCode, err := ex.ExecList(cmd.Condition)
		if err != nil {
			return condCode, err
		}

		// while: executa enquanto condition == 0
		// until: executa enquanto condition != 0
		if cmd.Until && condCode == 0 {
			break
		}
		if !cmd.Until && condCode != 0 {
			break
		}

		code, err = ex.ExecList(cmd.Body)
		if err != nil {
			if brk, ok := err.(*builtin.BreakError); ok {
				if brk.N <= 1 {
					break
				}
				return 0, &builtin.BreakError{N: brk.N - 1}
			}
			if cont, ok := err.(*builtin.ContinueError); ok {
				if cont.N <= 1 {
					continue
				}
				return 0, &builtin.ContinueError{N: cont.N - 1}
			}
			return code, err
		}
	}
	return code, nil
}

func (ex *Executor) execCase(cmd *parser.CaseCmd, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
	word := ex.Env.Expand(ex.expandWord(cmd.Word))

	for _, item := range cmd.Items {
		for _, pat := range item.Patterns {
			pattern := ex.expandWord(pat)
			matched, err := filepath.Match(pattern, word)
			if err == nil && (matched || pattern == "*") {
				return ex.ExecList(item.Body)
			}
		}
	}
	return 0, nil
}

func (ex *Executor) execSubshell(cmd *parser.SubshellCmd, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
	child := ex.Clone()
	child.Stdin = stdin
	child.Stdout = stdout
	child.Stderr = stderr
	return child.ExecList(cmd.Body)
}

func (ex *Executor) execBraceGroup(cmd *parser.BraceGroup, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
	stdin, stdout, stderr, cleanups, err := ex.applyRedirects(cmd.Redirects, stdin, stdout, stderr)
	if err != nil {
		return 1, err
	}
	defer func() {
		for _, f := range cleanups {
			f()
		}
	}()
	oldIn, oldOut, oldErr := ex.Stdin, ex.Stdout, ex.Stderr
	ex.Stdin, ex.Stdout, ex.Stderr = stdin, stdout, stderr
	code, err := ex.ExecList(cmd.Body)
	ex.Stdin, ex.Stdout, ex.Stderr = oldIn, oldOut, oldErr
	return code, err
}

func (ex *Executor) execArith(cmd *parser.ArithCmd) (int, error) {
	result := ex.Env.Expand("$((" + cmd.Expr + "))")
	n, _ := strconv.ParseInt(strings.TrimSpace(result), 10, 64)
	if n != 0 {
		return 0, nil
	}
	return 1, nil
}

func (ex *Executor) execFunc(fn *parser.FuncDef, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
	// Cria escopo da função com $0, $1, $2... $@
	child := ex.Clone()
	child.Stdin = stdin
	child.Stdout = stdout
	child.Stderr = stderr
	child.Env.Set("0", args[0])
	child.Env.Set("#", strconv.Itoa(len(args)-1))
	child.Env.Set("@", strings.Join(args[1:], " "))
	child.Env.Set("*", strings.Join(args[1:], " "))
	for i, arg := range args[1:] {
		child.Env.Set(strconv.Itoa(i+1), arg)
	}

	code, err := child.execNode(fn.Body, stdin, stdout, stderr)
	if err != nil {
		if ret, ok := err.(*builtin.ReturnError); ok {
			return ret.Code, nil
		}
		return code, err
	}
	return code, nil
}

// ─── Expansão ─────────────────────────────────────────────────────────────────

func (ex *Executor) expandWord(w *parser.Word) string {
	if w == nil {
		return ""
	}
	var sb strings.Builder
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *parser.LiteralPart:
			sb.WriteString(p.Value)
		case *parser.VarPart:
			v, _ := ex.Env.Get(p.Name)
			sb.WriteString(v)
		case *parser.SubshellPart:
			out := ex.captureList(p.Body)
			sb.WriteString(strings.TrimRight(out, "\n"))
		case *parser.ArithPart:
			expanded := ex.Env.Expand("$((" + p.Expr + "))")
			sb.WriteString(expanded)
		}
	}
	return ex.Env.Expand(sb.String())
}

func (ex *Executor) expandArgs(words []*parser.Word) ([]string, error) {
	var args []string
	for _, w := range words {
		expanded := ex.expandWord(w)
		// Glob expansion
		globbed, _ := env.ExpandGlob(expanded)
		args = append(args, globbed...)
	}
	return args, nil
}

// captureList executa uma List e captura stdout
func (ex *Executor) captureList(nodes []parser.Node) string {
	var buf bytes.Buffer
	child := ex.Clone()
	child.Stdout = &buf
	for _, n := range nodes {
		child.execNode(n, child.Stdin, child.Stdout, child.Stderr)
	}
	return buf.String()
}

// ─── Source e Exec ────────────────────────────────────────────────────────────

func (ex *Executor) sourceFile(path string, args []string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 1, fmt.Errorf("source: %s: %v", path, err)
	}

	// Salva parâmetros posicionais
	oldAt, _ := ex.Env.Get("@")
	oldHash, _ := ex.Env.Get("#")
	ex.Env.Set("@", strings.Join(args, " "))
	ex.Env.Set("#", strconv.Itoa(len(args)))

	code, err := ex.execString(string(data), ex.Stdin, ex.Stdout, ex.Stderr)

	ex.Env.Set("@", oldAt)
	ex.Env.Set("#", oldHash)

	return code, err
}

func (ex *Executor) execString(input string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
	l := lexer.New(input)
	tokens, err := l.Tokenize()
	if err != nil {
		return 1, err
	}
	p := parser.New(tokens)
	list, err := p.Parse()
	if err != nil {
		return 1, err
	}

	oldIn, oldOut, oldErr := ex.Stdin, ex.Stdout, ex.Stderr
	ex.Stdin, ex.Stdout, ex.Stderr = stdin, stdout, stderr
	code, err := ex.ExecList(list)
	ex.Stdin, ex.Stdout, ex.Stderr = oldIn, oldOut, oldErr
	return code, err
}

func (ex *Executor) doExec(args []string) (int, error) {
	path, err := ex.resolvePath(args[0])
	if err != nil {
		return 127, err
	}
	return 0, syscall.Exec(path, args, ex.Env.Environ())
}

// ExecScript executa um arquivo de script
func (ex *Executor) ExecScript(path string, args []string) (int, error) {
	return ex.sourceFile(path, args)
}

// CaptureOutput executa um comando e captura sua saída
func (ex *Executor) CaptureOutput(input string) (string, int, error) {
	var buf bytes.Buffer
	child := ex.Clone()
	child.Stdout = &buf
	code, err := child.execString(input, ex.Stdin, &buf, ex.Stderr)
	return strings.TrimRight(buf.String(), "\n"), code, err
}
