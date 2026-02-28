package executor

import (
	"github.com/TetsWorks/Gosh-shell/internal/lexer"
	"github.com/TetsWorks/Gosh-shell/internal/parser"
)

// ExecDirect parseia e executa uma string diretamente
func (ex *Executor) ExecDirect(input string) (int, error) {
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
	return ex.ExecList(list)
}

// ExecInteractive executa uma linha e trata subshell expansions $(...)
func (ex *Executor) ExecInteractive(line string) (int, error) {
	// Expande subshells antes do parse principal
	expanded, err := ex.expandSubshells(line)
	if err != nil {
		expanded = line
	}
	return ex.ExecDirect(expanded)
}

// expandSubshells expande $(...) capturando output
func (ex *Executor) expandSubshells(input string) (string, error) {
	// O executor já lida com subshells via AST,
	// mas para o caso de subshells no prompt/args simples fazemos aqui
	return ex.Env.Expand(input), nil
}

// GetHistory retorna o histórico (para readline)
func GetHistoryList() []string {
	return nil // gerenciado pelo readline
}
