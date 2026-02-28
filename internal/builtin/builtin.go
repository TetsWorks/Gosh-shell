package builtin

import (
	"fmt"
	"math"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/TetsWorks/Gosh-shell/internal/env"
	"github.com/TetsWorks/Gosh-shell/internal/jobcontrol"
)

// Result de um builtin
type Result struct {
	Code int
	Err  error
}

// Handler é a assinatura de uma função builtin
type Handler func(args []string, e *env.Env, jm *jobcontrol.Manager) Result

// Registry é o mapa de builtins
var Registry map[string]Handler

func init() {
	Registry = map[string]Handler{
		"cd":       Cd,
		"pwd":      Pwd,
		"echo":     Echo,
		"printf":   Printf,
		"export":   Export,
		"unset":    Unset,
		"set":      Set,
		"readonly": Readonly,
		"shift":    Shift,
		"source":   Source,
		".":        Source,
		"alias":    Alias,
		"unalias":  Unalias,
		"type":     Type,
		"which":    Which,
		"true":     True,
		"false":    False,
		"test":     Test,
		"[":        TestBracket,
		"exit":     Exit,
		"return":   Return,
		"break":    Break,
		"continue": Continue,
		"jobs":     Jobs,
		"fg":       Fg,
		"bg":       Bg,
		"kill":     Kill,
		"wait":     Wait,
		"read":     Read,
		"exec":     Exec,
		"eval":     Eval,
		"getopts":  Getopts,
		"local":    Local,
		"declare":  Declare,
		"help":     Help,
		"history":  History,
		"dirs":     Dirs,
		"pushd":    Pushd,
		"popd":     Popd,
		"umask":    Umask,
		"ulimit":   Ulimit,
	}
}

// Aliases mapa de aliases
var Aliases = make(map[string]string)

// Historylist armazena o histórico de comandos
var HistoryList []string

// Dirstack é a pilha de diretórios para pushd/popd
var Dirstack []string

// Lookup verifica se um nome é builtin
func Lookup(name string) (Handler, bool) {
	// Expande alias
	if alias, ok := Aliases[name]; ok {
		_ = alias // O executor precisa re-parsear o alias
		return nil, false
	}
	h, ok := Registry[name]
	return h, ok
}

// ─── Implementações ───────────────────────────────────────────────────────────

func Cd(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	var dir string

	if len(args) < 2 || args[1] == "~" {
		home, ok := e.Get("HOME")
		if !ok {
			u, err := user.Current()
			if err != nil {
				return Result{1, fmt.Errorf("cd: não foi possível determinar HOME")}
			}
			home = u.HomeDir
		}
		dir = home
	} else if args[1] == "-" {
		oldpwd, ok := e.Get("OLDPWD")
		if !ok {
			return Result{1, fmt.Errorf("cd: OLDPWD não definido")}
		}
		dir = oldpwd
		fmt.Println(dir)
	} else {
		dir = args[1]
		// Expande ~user
		if strings.HasPrefix(dir, "~") {
			dir = e.Expand(dir)
		}
	}

	oldpwd, _ := os.Getwd()

	if err := os.Chdir(dir); err != nil {
		return Result{1, fmt.Errorf("cd: %s: %v", dir, err)}
	}

	newpwd, _ := os.Getwd()
	e.Set("OLDPWD", oldpwd)
	e.Set("PWD", newpwd)
	return Result{0, nil}
}

func Pwd(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	cwd, err := os.Getwd()
	if err != nil {
		return Result{1, err}
	}
	// -P resolve symlinks
	if len(args) > 1 && args[1] == "-P" {
		cwd, err = filepath.EvalSymlinks(cwd)
		if err != nil {
			return Result{1, err}
		}
	}
	fmt.Println(cwd)
	return Result{0, nil}
}

func Echo(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	newline := true
	interpret := false
	i := 1
	for i < len(args) {
		if args[i] == "-n" {
			newline = false
			i++
		} else if args[i] == "-e" {
			interpret = true
			i++
		} else if args[i] == "-E" {
			interpret = false
			i++
		} else {
			break
		}
	}

	out := strings.Join(args[i:], " ")
	if interpret {
		out = interpretEscapes(out)
	}

	if newline {
		fmt.Println(out)
	} else {
		fmt.Print(out)
	}
	return Result{0, nil}
}

func interpretEscapes(s string) string {
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\t`, "\t")
	s = strings.ReplaceAll(s, `\r`, "\r")
	s = strings.ReplaceAll(s, `\\`, "\\")
	s = strings.ReplaceAll(s, `\a`, "\a")
	s = strings.ReplaceAll(s, `\b`, "\b")
	s = strings.ReplaceAll(s, `\e`, "\033")
	return s
}

func Printf(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	if len(args) < 2 {
		return Result{1, fmt.Errorf("printf: formato requerido")}
	}
	format := interpretEscapes(args[1])
	// Substitui %s, %d, %f de forma simples
	rest := args[2:]
	idx := 0
	result := ""
	fmtRunes := []rune(format)
	for i := 0; i < len(fmtRunes); i++ {
		if fmtRunes[i] == '%' && i+1 < len(fmtRunes) {
			i++
			var val string
			if idx < len(rest) {
				val = rest[idx]
				idx++
			}
			switch fmtRunes[i] {
			case 's':
				result += val
			case 'd':
				n, _ := strconv.ParseInt(val, 10, 64)
				result += strconv.FormatInt(n, 10)
			case 'f':
				f, _ := strconv.ParseFloat(val, 64)
				result += strconv.FormatFloat(f, 'f', 6, 64)
			case 'x':
				n, _ := strconv.ParseInt(val, 10, 64)
				result += strconv.FormatInt(n, 16)
			case 'o':
				n, _ := strconv.ParseInt(val, 10, 64)
				result += strconv.FormatInt(n, 8)
			case 'b':
				n, _ := strconv.ParseInt(val, 10, 64)
				result += strconv.FormatInt(n, 2)
			case '%':
				result += "%"
				idx--
			}
		} else {
			result += string(fmtRunes[i])
		}
	}
	fmt.Print(result)
	return Result{0, nil}
}

func Export(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	if len(args) < 2 {
		// Lista exportadas
		all := e.All()
		var keys []string
		for k := range all {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("declare -x %s=\"%s\"\n", k, all[k])
		}
		return Result{0, nil}
	}
	for _, arg := range args[1:] {
		if strings.Contains(arg, "=") {
			parts := strings.SplitN(arg, "=", 2)
			e.Export(parts[0], parts[1])
		} else {
			v, _ := e.Get(arg)
			e.Export(arg, v)
		}
	}
	return Result{0, nil}
}

func Unset(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	for _, name := range args[1:] {
		e.Unset(name)
	}
	return Result{0, nil}
}

func Set(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	if len(args) == 1 {
		all := e.All()
		var keys []string
		for k := range all {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("%s=%s\n", k, all[k])
		}
		return Result{0, nil}
	}
	// Opções simples: -e (exit on error), -x (xtrace), -u (unbound)
	for _, opt := range args[1:] {
		e.Set("GOSH_OPT_"+strings.TrimLeft(opt, "-+"), strings.TrimPrefix(opt, "+"))
	}
	return Result{0, nil}
}

func Readonly(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	for _, arg := range args[1:] {
		if strings.Contains(arg, "=") {
			parts := strings.SplitN(arg, "=", 2)
			e.Set(parts[0], parts[1])
			e.SetReadonly(parts[0])
		} else {
			e.SetReadonly(arg)
		}
	}
	return Result{0, nil}
}

func Shift(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	n := 1
	if len(args) > 1 {
		n, _ = strconv.Atoi(args[1])
	}
	params, _ := e.Get("@")
	parts := strings.Fields(params)
	if n > len(parts) {
		return Result{1, fmt.Errorf("shift: contagem maior que número de parâmetros")}
	}
	parts = parts[n:]
	e.Set("@", strings.Join(parts, " "))
	e.Set("#", strconv.Itoa(len(parts)))
	return Result{0, nil}
}

func Source(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	// O executor implementa a lógica de source completa
	// Este builtin sinaliza ao executor para processar o arquivo
	if len(args) < 2 {
		return Result{1, fmt.Errorf("source: arquivo requerido")}
	}
	return Result{-1, nil} // Sinal especial para o executor
}

func Alias(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	if len(args) < 2 {
		for name, val := range Aliases {
			fmt.Printf("alias %s='%s'\n", name, val)
		}
		return Result{0, nil}
	}
	for _, arg := range args[1:] {
		if strings.Contains(arg, "=") {
			parts := strings.SplitN(arg, "=", 2)
			val := strings.Trim(parts[1], "'\"")
			Aliases[parts[0]] = val
		} else {
			if v, ok := Aliases[arg]; ok {
				fmt.Printf("alias %s='%s'\n", arg, v)
			} else {
				return Result{1, fmt.Errorf("alias: %s: não encontrado", arg)}
			}
		}
	}
	return Result{0, nil}
}

func Unalias(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	if len(args) > 1 && args[1] == "-a" {
		Aliases = make(map[string]string)
		return Result{0, nil}
	}
	for _, name := range args[1:] {
		delete(Aliases, name)
	}
	return Result{0, nil}
}

func Type(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	for _, name := range args[1:] {
		if _, ok := Aliases[name]; ok {
			fmt.Printf("%s is aliased to '%s'\n", name, Aliases[name])
			continue
		}
		if _, ok := Registry[name]; ok {
			fmt.Printf("%s is a shell builtin\n", name)
			continue
		}
		// Procura no PATH
		path, _ := e.Get("PATH")
		found := false
		for _, dir := range strings.Split(path, ":") {
			full := filepath.Join(dir, name)
			if _, err := os.Stat(full); err == nil {
				fmt.Printf("%s is %s\n", name, full)
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintf(os.Stderr, "type: %s: not found\n", name)
		}
	}
	return Result{0, nil}
}

func Which(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	code := 0
	path, _ := e.Get("PATH")
	for _, name := range args[1:] {
		found := false
		for _, dir := range strings.Split(path, ":") {
			full := filepath.Join(dir, name)
			if info, err := os.Stat(full); err == nil && !info.IsDir() {
				fmt.Println(full)
				found = true
				break
			}
		}
		if !found {
			code = 1
		}
	}
	return Result{code, nil}
}

func True(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	return Result{0, nil}
}

func False(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	return Result{1, nil}
}

// Test implementa [ e test
func Test(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	return doTest(args[1:])
}

func TestBracket(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	// Remove o ] final
	if len(args) > 0 && args[len(args)-1] == "]" {
		args = args[:len(args)-1]
	}
	return doTest(args[1:])
}

func doTest(args []string) Result {
	if len(args) == 0 {
		return Result{1, nil}
	}

	// Unário
	if len(args) == 2 {
		op := args[0]
		val := args[1]
		switch op {
		case "-n":
			if len(val) > 0 {
				return Result{0, nil}
			}
			return Result{1, nil}
		case "-z":
			if len(val) == 0 {
				return Result{0, nil}
			}
			return Result{1, nil}
		case "-e":
			if _, err := os.Stat(val); err == nil {
				return Result{0, nil}
			}
			return Result{1, nil}
		case "-f":
			info, err := os.Stat(val)
			if err == nil && !info.IsDir() {
				return Result{0, nil}
			}
			return Result{1, nil}
		case "-d":
			info, err := os.Stat(val)
			if err == nil && info.IsDir() {
				return Result{0, nil}
			}
			return Result{1, nil}
		case "-r":
			f, err := os.OpenFile(val, os.O_RDONLY, 0)
			if err == nil {
				f.Close()
				return Result{0, nil}
			}
			return Result{1, nil}
		case "-w":
			f, err := os.OpenFile(val, os.O_WRONLY, 0)
			if err == nil {
				f.Close()
				return Result{0, nil}
			}
			return Result{1, nil}
		case "-x":
			info, err := os.Stat(val)
			if err == nil && info.Mode()&0111 != 0 {
				return Result{0, nil}
			}
			return Result{1, nil}
		case "-s":
			info, err := os.Stat(val)
			if err == nil && info.Size() > 0 {
				return Result{0, nil}
			}
			return Result{1, nil}
		case "-L":
			_, err := os.Lstat(val)
			if err == nil {
				return Result{0, nil}
			}
			return Result{1, nil}
		case "!":
			r := doTest([]string{val})
			if r.Code == 0 {
				return Result{1, nil}
			}
			return Result{0, nil}
		}
	}

	// Binário
	if len(args) == 3 {
		left, op, right := args[0], args[1], args[2]
		switch op {
		// String
		case "=", "==":
			if left == right {
				return Result{0, nil}
			}
		case "!=":
			if left != right {
				return Result{0, nil}
			}
		case "<":
			if left < right {
				return Result{0, nil}
			}
		case ">":
			if left > right {
				return Result{0, nil}
			}
		// Numérico
		case "-eq":
			l, _ := strconv.ParseFloat(left, 64)
			r, _ := strconv.ParseFloat(right, 64)
			if l == r {
				return Result{0, nil}
			}
		case "-ne":
			l, _ := strconv.ParseFloat(left, 64)
			r, _ := strconv.ParseFloat(right, 64)
			if l != r {
				return Result{0, nil}
			}
		case "-lt":
			l, _ := strconv.ParseFloat(left, 64)
			r, _ := strconv.ParseFloat(right, 64)
			if l < r {
				return Result{0, nil}
			}
		case "-le":
			l, _ := strconv.ParseFloat(left, 64)
			r, _ := strconv.ParseFloat(right, 64)
			if l <= r {
				return Result{0, nil}
			}
		case "-gt":
			l, _ := strconv.ParseFloat(left, 64)
			r, _ := strconv.ParseFloat(right, 64)
			if l > r {
				return Result{0, nil}
			}
		case "-ge":
			l, _ := strconv.ParseFloat(left, 64)
			r, _ := strconv.ParseFloat(right, 64)
			if l >= r {
				return Result{0, nil}
			}
		// Lógico
		case "-a":
			if doTest([]string{left}).Code == 0 && doTest([]string{right}).Code == 0 {
				return Result{0, nil}
			}
		case "-o":
			if doTest([]string{left}).Code == 0 || doTest([]string{right}).Code == 0 {
				return Result{0, nil}
			}
		}
		return Result{1, nil}
	}

	// ! expr
	if args[0] == "!" {
		r := doTest(args[1:])
		if r.Code == 0 {
			return Result{1, nil}
		}
		return Result{0, nil}
	}

	return Result{1, nil}
}

// Sentinel errors para controle de fluxo
type ExitError struct{ Code int }
type ReturnError struct{ Code int }
type BreakError struct{ N int }
type ContinueError struct{ N int }

func (e *ExitError) Error() string    { return fmt.Sprintf("exit %d", e.Code) }
func (e *ReturnError) Error() string  { return fmt.Sprintf("return %d", e.Code) }
func (e *BreakError) Error() string   { return fmt.Sprintf("break %d", e.N) }
func (e *ContinueError) Error() string { return fmt.Sprintf("continue %d", e.N) }

func Exit(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	code := 0
	if len(args) > 1 {
		code, _ = strconv.Atoi(args[1])
	}
	return Result{code, &ExitError{Code: code}}
}

func Return(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	code := 0
	if len(args) > 1 {
		code, _ = strconv.Atoi(args[1])
	}
	return Result{code, &ReturnError{Code: code}}
}

func Break(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	n := 1
	if len(args) > 1 {
		n, _ = strconv.Atoi(args[1])
	}
	return Result{0, &BreakError{N: n}}
}

func Continue(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	n := 1
	if len(args) > 1 {
		n, _ = strconv.Atoi(args[1])
	}
	return Result{0, &ContinueError{N: n}}
}

func Jobs(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	jm.UpdateStatus()
	for _, job := range jm.List() {
		fmt.Println(job)
	}
	return Result{0, nil}
}

func Fg(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	var id int
	if len(args) > 1 {
		s := strings.TrimPrefix(args[1], "%")
		id, _ = strconv.Atoi(s)
	} else {
		job := jm.Last()
		if job == nil {
			return Result{1, fmt.Errorf("fg: sem jobs ativos")}
		}
		id = job.ID
	}
	if err := jm.Fg(id); err != nil {
		return Result{1, err}
	}
	return Result{0, nil}
}

func Bg(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	var id int
	if len(args) > 1 {
		s := strings.TrimPrefix(args[1], "%")
		id, _ = strconv.Atoi(s)
	} else {
		job := jm.Last()
		if job == nil {
			return Result{1, fmt.Errorf("bg: sem jobs parados")}
		}
		id = job.ID
	}
	if err := jm.Bg(id); err != nil {
		return Result{1, err}
	}
	return Result{0, nil}
}

func Kill(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	if len(args) < 2 {
		return Result{1, fmt.Errorf("kill: PID ou job requerido")}
	}
	// TODO: suporte a -SIGNAME
	target := args[len(args)-1]
	target = strings.TrimPrefix(target, "%")
	pid, err := strconv.Atoi(target)
	if err != nil {
		return Result{1, fmt.Errorf("kill: %s: inválido", target)}
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return Result{1, err}
	}
	if err := proc.Signal(os.Interrupt); err != nil {
		return Result{1, err}
	}
	return Result{0, nil}
}

func Wait(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	// Simplificado: espera todos os jobs
	for _, job := range jm.List() {
		if job.Status == jobcontrol.StatusRunning && len(job.Procs) > 0 {
			job.Procs[0].Wait()
		}
	}
	return Result{0, nil}
}

func Read(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	var prompt string
	varName := "REPLY"
	i := 1
	for i < len(args) {
		switch args[i] {
		case "-p":
			i++
			if i < len(args) {
				prompt = args[i]
			}
		case "-r":
			// raw mode - ignora backslashes
		default:
			if !strings.HasPrefix(args[i], "-") {
				varName = args[i]
			}
		}
		i++
	}

	if prompt != "" {
		fmt.Print(prompt)
	}

	var line string
	fmt.Scanln(&line)
	e.Set(varName, line)
	return Result{0, nil}
}

func Exec(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	// Sinaliza executor para fazer exec real
	return Result{-2, nil}
}

func Eval(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	// Sinaliza executor para re-parsear e executar
	return Result{-3, nil}
}

func Getopts(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	// Implementação básica de getopts
	if len(args) < 3 {
		return Result{1, nil}
	}
	optstring := args[1]
	varName := args[2]

	optind, _ := e.Get("OPTIND")
	idx, _ := strconv.Atoi(optind)
	if idx == 0 {
		idx = 1
	}

	params, _ := e.Get("@")
	pargs := strings.Fields(params)

	if idx > len(pargs) {
		return Result{1, nil}
	}

	arg := pargs[idx-1]
	if !strings.HasPrefix(arg, "-") || arg == "-" || arg == "--" {
		return Result{1, nil}
	}

	opt := string(arg[1])
	if !strings.Contains(optstring, opt) {
		e.Set(varName, "?")
		return Result{0, nil}
	}

	e.Set(varName, opt)
	e.Set("OPTIND", strconv.Itoa(idx+1))

	if strings.Contains(optstring, opt+":") {
		if idx+1 <= len(pargs) {
			e.Set("OPTARG", pargs[idx])
			e.Set("OPTIND", strconv.Itoa(idx+2))
		}
	}

	return Result{0, nil}
}

func Local(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	// Em um shell real, local cria variável no escopo da função
	// Aqui apenas define no env atual
	for _, arg := range args[1:] {
		if strings.Contains(arg, "=") {
			parts := strings.SplitN(arg, "=", 2)
			e.Set(parts[0], parts[1])
		} else {
			e.Set(arg, "")
		}
	}
	return Result{0, nil}
}

func Declare(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	return Local(args, e, jm)
}

func History(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	n := len(HistoryList)
	if len(args) > 1 {
		count, err := strconv.Atoi(args[1])
		if err == nil && count < n {
			HistoryList = HistoryList[n-count:]
		}
	}
	for i, h := range HistoryList {
		fmt.Printf("%5d  %s\n", i+1, h)
	}
	return Result{0, nil}
}

func Dirs(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	cwd, _ := os.Getwd()
	all := append([]string{cwd}, Dirstack...)
	fmt.Println(strings.Join(all, " "))
	return Result{0, nil}
}

func Pushd(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	if len(args) < 2 {
		if len(Dirstack) == 0 {
			return Result{1, fmt.Errorf("pushd: sem diretórios")}
		}
		// Troca topo com cwd
		cwd, _ := os.Getwd()
		top := Dirstack[0]
		Dirstack[0] = cwd
		if err := os.Chdir(top); err != nil {
			return Result{1, err}
		}
	} else {
		cwd, _ := os.Getwd()
		Dirstack = append([]string{cwd}, Dirstack...)
		if err := os.Chdir(args[1]); err != nil {
			Dirstack = Dirstack[1:]
			return Result{1, err}
		}
	}
	return Dirs([]string{"dirs"}, e, jm)
}

func Popd(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	if len(Dirstack) == 0 {
		return Result{1, fmt.Errorf("popd: pilha vazia")}
	}
	top := Dirstack[0]
	Dirstack = Dirstack[1:]
	if err := os.Chdir(top); err != nil {
		return Result{1, err}
	}
	return Dirs([]string{"dirs"}, e, jm)
}

func Umask(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	if len(args) < 2 {
		mask := 022 // padrão
		fmt.Printf("%04o\n", mask)
		return Result{0, nil}
	}
	// Aplicar nova umask
	val, err := strconv.ParseInt(args[1], 8, 32)
	if err != nil {
		return Result{1, fmt.Errorf("umask: %s: inválido", args[1])}
	}
	_ = val
	return Result{0, nil}
}

func Ulimit(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	// Stub - implementação completa requer syscall
	fmt.Printf("unlimited\n")
	return Result{0, nil}
}

func Help(args []string, e *env.Env, jm *jobcontrol.Manager) Result {
	if len(args) > 1 {
		fmt.Printf("Ajuda para '%s' não disponível.\n", args[1])
		return Result{0, nil}
	}
	fmt.Println("gosh - Go Shell")
	fmt.Println("Builtins disponíveis:")
	var names []string
	for name := range Registry {
		names = append(names, name)
	}
	sort.Strings(names)
	cols := 0
	for _, name := range names {
		fmt.Printf("  %-12s", name)
		cols++
		if cols%5 == 0 {
			fmt.Println()
		}
	}
	fmt.Println()
	return Result{0, nil}
}

// ─── Matemática auxiliar ──────────────────────────────────────────────────────
var _ = math.Sqrt // mantém import usado
