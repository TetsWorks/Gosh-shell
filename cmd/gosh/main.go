package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/yourusername/gosh/internal/builtin"
	"github.com/yourusername/gosh/internal/env"
	"github.com/yourusername/gosh/internal/executor"
	"github.com/yourusername/gosh/internal/jobcontrol"
	"github.com/yourusername/gosh/internal/rc"
	"github.com/yourusername/gosh/internal/readline"
)

const (
	Version = "0.1.0"
	Banner  = `
  ██████╗  ██████╗ ███████╗██╗  ██╗
 ██╔════╝ ██╔═══██╗██╔════╝██║  ██║
 ██║  ███╗██║   ██║███████╗███████║
 ██║   ██║██║   ██║╚════██║██╔══██║
 ╚██████╔╝╚██████╔╝███████║██║  ██║
  ╚═════╝  ╚═════╝ ╚══════╝╚═╝  ╚═╝  v%s
`
)

func main() {
	// Flags
	flagC := flag.String("c", "", "Executa string de comando")
	flagS := flag.Bool("s", false, "Lê comandos do stdin")
	flagLogin := flag.Bool("l", false, "Shell de login")
	flagInteractive := flag.Bool("i", false, "Força modo interativo")
	flagNoRC := flag.Bool("norc", false, "Não carrega .goshrc")
	flagVersion := flag.Bool("version", false, "Exibe versão")
	flagXtrace := flag.Bool("x", false, "Ativa xtrace (set -x)")
	flagExit := flag.Bool("e", false, "Sai em erro (set -e)")
	flag.Parse()

	if *flagVersion {
		fmt.Printf("gosh %s\n", Version)
		os.Exit(0)
	}

	// Inicializa ambiente
	e := env.New()
	jm := jobcontrol.New()
	ex := executor.New(e, jm)
	ex.Xtrace = *flagXtrace
	ex.ExitOnError = *flagExit

	// Coloca o shell em seu próprio grupo de processos
	jobcontrol.InitShellProcessGroup()
	jobcontrol.SetupSignals()

	// Modo -c: executa string
	if *flagC != "" {
		code, err := ex.ExecScript("", nil)
		_ = code
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		// Executa diretamente
		os.Exit(runString(ex, *flagC))
	}

	// Modo script: arquivo passado como argumento
	args := flag.Args()
	if len(args) > 0 && !*flagS && !*flagInteractive {
		scriptPath := args[0]
		e.Set("0", scriptPath)
		e.Set("@", strings.Join(args[1:], " "))
		e.Set("#", fmt.Sprintf("%d", len(args)-1))
		for i, a := range args[1:] {
			e.Set(fmt.Sprintf("%d", i+1), a)
		}

		code, err := ex.ExecScript(scriptPath, args[1:])
		if err != nil {
			if exitErr, ok := err.(interface{ Error() string }); ok {
				fmt.Fprintln(os.Stderr, exitErr.Error())
			}
		}
		os.Exit(code)
	}

	// Modo interativo (padrão) ou -s
	isInteractive := *flagInteractive || (*flagS == false && len(args) == 0)

	if isInteractive {
		runInteractive(ex, e, jm, *flagLogin, *flagNoRC)
	} else {
		// Lê stdin
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(runString(ex, string(data)))
	}
}

func runInteractive(ex *executor.Executor, e *env.Env, jm *jobcontrol.Manager, login, noRC bool) {
	fmt.Printf(Banner, Version)
	fmt.Println("\033[90mDigite 'help' para ver os builtins. 'exit' para sair.\033[0m")

	// Carrega histórico
	histPath := rc.GetHistoryPath()
	history := rc.ReadHistory(histPath)

	// Cria readline editor
	rl := readline.New(e)
	rl.SetHistory(history)

	// Cria .goshrc padrão se necessário
	rc.CreateDefaultRC()

	// Carrega .gosh_profile em login shells
	if login {
		profilePath := rc.GetProfilePath()
		if _, err := os.Stat(profilePath); err == nil {
			ex.ExecScript(profilePath, nil)
		}
	}

	// Carrega .goshrc
	if !noRC {
		rcPath := rc.GetRCPath()
		if _, err := os.Stat(rcPath); err == nil {
			ex.ExecScript(rcPath, nil)
		}
	}

	// Configura sinal para salvar histórico no Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sigChan
		rc.SaveHistory(histPath, history)
		os.Exit(0)
	}()

	// Loop principal
	for {
		// Verifica jobs completos
		jm.UpdateStatus()
		jm.PrintCompleted()

		// Gera prompt
		prompt := buildPrompt(e, ex)

		// Lê linha
		line, err := rl.ReadLine(prompt)
		if err != nil {
			if err.Error() == "EOF" {
				fmt.Println("exit")
				break
			}
			continue
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Adiciona ao histórico
		rl.AddHistory(line)

		// Executa
		code := runString(ex, line)
		_ = code
	}

	// Salva histórico
	rc.SaveHistory(histPath, rl.GetHistory())
}

func runString(ex *executor.Executor, s string) int {
	code, err := ex.ExecDirect(s)
	if err != nil {
		if _, ok := err.(*builtin.ExitError); ok {
			exitErr := err.(*builtin.ExitError)
			os.Exit(exitErr.Code)
		}
		fmt.Fprintf(os.Stderr, "gosh: %v\n", err)
		return 1
	}
	return code
}

func buildPrompt(e *env.Env, ex *executor.Executor) string {
	ps1, _ := e.Get("PS1")
	if ps1 == "" {
		ps1 = `\u@\h:\w\$ `
	}
	return expandPrompt(ps1, e)
}

func expandPrompt(ps1 string, e *env.Env) string {
	var result strings.Builder
	runes := []rune(ps1)

	for i := 0; i < len(runes); i++ {
		if runes[i] != '\\' || i+1 >= len(runes) {
			result.WriteRune(runes[i])
			continue
		}
		i++
		switch runes[i] {
		case 'u':
			u, _ := e.Get("USER")
			result.WriteString(u)
		case 'h':
			h, _ := e.Get("HOSTNAME")
			if h == "" {
				h, _ = os.Hostname()
			}
			result.WriteString(strings.SplitN(h, ".", 2)[0])
		case 'H':
			h, _ := os.Hostname()
			result.WriteString(h)
		case 'w':
			cwd, _ := os.Getwd()
			home, _ := e.Get("HOME")
			if strings.HasPrefix(cwd, home) {
				cwd = "~" + cwd[len(home):]
			}
			result.WriteString(cwd)
		case 'W':
			cwd, _ := os.Getwd()
			parts := strings.Split(cwd, "/")
			result.WriteString(parts[len(parts)-1])
		case '$':
			if os.Getuid() == 0 {
				result.WriteRune('#')
			} else {
				result.WriteRune('$')
			}
		case 'n', '\n':
			result.WriteRune('\n')
		case 't':
			// Hora HH:MM:SS - poderia usar time.Now()
			result.WriteString("--:--:--")
		case '[':
			result.WriteString("\001")
		case ']':
			result.WriteString("\002")
		case '\\':
			result.WriteRune('\\')
		default:
			result.WriteRune('\\')
			result.WriteRune(runes[i])
		}
	}
	return result.String()
}
