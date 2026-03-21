package parser

import "fmt"

// Node é a interface base de todos os nós da AST
type Node interface {
	nodeType() string
	String() string
}

// ─── Nós de Comando ──────────────────────────────────────────────────────────

// Word representa uma palavra com possíveis expansões
type Word struct {
	Parts []WordPart
}

func (w *Word) nodeType() string { return "Word" }
func (w *Word) String() string {
	s := ""
	for _, p := range w.Parts {
		s += p.String()
	}
	return s
}

// WordPart é parte de uma Word (literal, variável, subshell, etc)
type WordPart interface {
	String() string
}

type LiteralPart struct{ Value string }
type VarPart struct{ Name string; Modifier string; Default string }
type SubshellPart struct{ Body []Node }
type ArithPart struct{ Expr string }
type BacktickPart struct{ Body []Node }

func (l *LiteralPart) String() string { return l.Value }
func (v *VarPart) String() string     { return "$" + v.Name }
func (s *SubshellPart) String() string { return "$(...)" }
func (a *ArithPart) String() string   { return "$((" + a.Expr + "))" }
func (b *BacktickPart) String() string { return "`...`" }

// Redirect representa um redirecionamento de I/O
type Redirect struct {
	Op   string // >, >>, <, <<, 2>, &>
	Fd   int    // file descriptor source (-1 = padrão)
	File *Word  // arquivo destino
	Here string // heredoc content
}

func (r *Redirect) String() string {
	if r.File != nil {
		return fmt.Sprintf("%s%s", r.Op, r.File.String())
	}
	return r.Op
}

// ─── Comandos simples ────────────────────────────────────────────────────────

// SimpleCmd é um comando simples: cmd arg1 arg2
type SimpleCmd struct {
	Assigns   []*Assign   // VAR=val antes do comando
	Args      []*Word     // argumentos
	Redirects []*Redirect // redirecionamentos
}

func (s *SimpleCmd) nodeType() string { return "SimpleCmd" }
func (s *SimpleCmd) String() string {
	if len(s.Args) == 0 {
		return "<empty>"
	}
	return s.Args[0].String()
}

// Assign é uma atribuição de variável: VAR=val
type Assign struct {
	Name  string
	Value *Word
}

// ─── Pipeline ────────────────────────────────────────────────────────────────

// Pipeline é uma sequência de comandos ligados por pipes
type Pipeline struct {
	Negate   bool        // ! no início
	Cmds     []Node      // comandos
	PipeErr  bool        // |& (redireciona stderr também)
}

func (p *Pipeline) nodeType() string { return "Pipeline" }
func (p *Pipeline) String() string   { return "pipeline" }

// ─── Listas ──────────────────────────────────────────────────────────────────

// AndOrList é &&/|| entre pipelines
type AndOrList struct {
	Pipelines []*Pipeline
	Ops       []string // "&&" ou "||" entre cada pipeline
}

func (a *AndOrList) nodeType() string { return "AndOrList" }
func (a *AndOrList) String() string   { return "andor" }

// List é uma sequência de AndOrLists separadas por ; ou &
type List struct {
	Items []ListItem
}

type ListItem struct {
	Node       *AndOrList
	Background bool // termina com &
}

func (l *List) nodeType() string { return "List" }
func (l *List) String() string   { return "list" }

// ─── Compostos ───────────────────────────────────────────────────────────────

// IfCmd representa if/then/elif/else/fi
type IfCmd struct {
	Condition *List
	Then      *List
	Elifs     []ElseIf
	Else      *List
}

type ElseIf struct {
	Condition *List
	Body      *List
}

func (i *IfCmd) nodeType() string { return "IfCmd" }
func (i *IfCmd) String() string   { return "if" }

// ForCmd representa for var in words; do list; done
type ForCmd struct {
	Var   string
	Words []*Word
	Body  *List
}

func (f *ForCmd) nodeType() string { return "ForCmd" }
func (f *ForCmd) String() string   { return "for " + f.Var }

// WhileCmd representa while/until condition; do body; done
type WhileCmd struct {
	Until     bool
	Condition *List
	Body      *List
}

func (w *WhileCmd) nodeType() string { return "WhileCmd" }
func (w *WhileCmd) String() string {
	if w.Until {
		return "until"
	}
	return "while"
}

// CaseCmd representa case word in pattern) body;; esac
type CaseCmd struct {
	Word  *Word
	Items []CaseItem
}

type CaseItem struct {
	Patterns []*Word
	Body     *List
}

func (c *CaseCmd) nodeType() string { return "CaseCmd" }
func (c *CaseCmd) String() string   { return "case" }

// FuncDef define uma função shell
type FuncDef struct {
	Name string
	Body Node // geralmente um CompoundCmd
}

func (f *FuncDef) nodeType() string { return "FuncDef" }
func (f *FuncDef) String() string   { return "function " + f.Name }

// SubshellCmd executa em subshell: (list)
type SubshellCmd struct {
	Body *List
}

func (s *SubshellCmd) nodeType() string { return "SubshellCmd" }
func (s *SubshellCmd) String() string   { return "(subshell)" }

// BraceGroup executa no shell atual: { list; }
type BraceGroup struct {
	Body      *List
	Redirects []*Redirect
}

func (b *BraceGroup) nodeType() string { return "BraceGroup" }
func (b *BraceGroup) String() string   { return "{group}" }

// ArithCmd executa expressão aritmética: (( expr ))
type ArithCmd struct {
	Expr string
}

func (a *ArithCmd) nodeType() string { return "ArithCmd" }
func (a *ArithCmd) String() string   { return "((" + a.Expr + "))" }

// SelectCmd: select var in words; do body; done
type SelectCmd struct {
	Var   string
	Words []*Word
	Body  *List
}

func (s *SelectCmd) nodeType() string { return "SelectCmd" }
func (s *SelectCmd) String() string   { return "select " + s.Var }
