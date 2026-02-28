package parser

import (
	"fmt"
	"strings"

	"github.com/TetsWorks/Gosh-shell/internal/lexer"
)

// Parser constrói a AST a partir de tokens
type Parser struct {
	tokens []lexer.Token
	pos    int
}

// New cria um Parser
func New(tokens []lexer.Token) *Parser {
	return &Parser{tokens: tokens, pos: 0}
}

// Parse faz o parse completo e retorna a List raiz
func (p *Parser) Parse() (*List, error) {
	p.skipNewlines()
	if p.peek().Type == lexer.TOKEN_EOF {
		return &List{}, nil
	}
	list, err := p.parseList()
	if err != nil {
		return nil, err
	}
	return list, nil
}

// ─── Utilitários ─────────────────────────────────────────────────────────────

func (p *Parser) peek() lexer.Token {
	if p.pos >= len(p.tokens) {
		return lexer.Token{Type: lexer.TOKEN_EOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) peekAt(offset int) lexer.Token {
	i := p.pos + offset
	if i >= len(p.tokens) {
		return lexer.Token{Type: lexer.TOKEN_EOF}
	}
	return p.tokens[i]
}

func (p *Parser) advance() lexer.Token {
	tok := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return tok
}

func (p *Parser) expect(t lexer.TokenType) (lexer.Token, error) {
	tok := p.advance()
	if tok.Type != t {
		return tok, fmt.Errorf("esperava %s, encontrou %s (%q) na linha %d",
			t, tok.Type, tok.Value, tok.Line)
	}
	return tok, nil
}

func (p *Parser) skipNewlines() {
	for p.peek().Type == lexer.TOKEN_NEWLINE {
		p.advance()
	}
}

func (p *Parser) isTerminator() bool {
	t := p.peek().Type
	return t == lexer.TOKEN_NEWLINE || t == lexer.TOKEN_SEMICOLON ||
		t == lexer.TOKEN_EOF || t == lexer.TOKEN_BACKGROUND
}

// ─── List ─────────────────────────────────────────────────────────────────

func (p *Parser) parseList() (*List, error) {
	list := &List{}
	for {
		p.skipNewlines()
		t := p.peek().Type
		if t == lexer.TOKEN_EOF ||
			t == lexer.TOKEN_RBRACE ||
			t == lexer.TOKEN_RPAREN ||
			t == lexer.TOKEN_FI ||
			t == lexer.TOKEN_DONE ||
			t == lexer.TOKEN_ESAC ||
			t == lexer.TOKEN_THEN ||
			t == lexer.TOKEN_ELSE ||
			t == lexer.TOKEN_ELIF {
			break
		}

		aol, err := p.parseAndOr()
		if err != nil {
			return nil, err
		}

		item := ListItem{Node: aol}
		if p.peek().Type == lexer.TOKEN_BACKGROUND {
			item.Background = true
			p.advance()
		} else if p.peek().Type == lexer.TOKEN_SEMICOLON || p.peek().Type == lexer.TOKEN_NEWLINE {
			p.advance()
		}

		list.Items = append(list.Items, item)
	}
	return list, nil
}

// ─── And/Or ───────────────────────────────────────────────────────────────

func (p *Parser) parseAndOr() (*AndOrList, error) {
	aol := &AndOrList{}
	pl, err := p.parsePipeline()
	if err != nil {
		return nil, err
	}
	aol.Pipelines = append(aol.Pipelines, pl)

	for p.peek().Type == lexer.TOKEN_AND || p.peek().Type == lexer.TOKEN_OR {
		op := p.advance().Value
		aol.Ops = append(aol.Ops, op)
		p.skipNewlines()
		pl, err = p.parsePipeline()
		if err != nil {
			return nil, err
		}
		aol.Pipelines = append(aol.Pipelines, pl)
	}

	return aol, nil
}

// ─── Pipeline ─────────────────────────────────────────────────────────────

func (p *Parser) parsePipeline() (*Pipeline, error) {
	pl := &Pipeline{}

	// Negação com !
	if p.peek().Type == lexer.TOKEN_WORD && p.peek().Value == "!" {
		pl.Negate = true
		p.advance()
	}

	cmd, err := p.parseCommand()
	if err != nil {
		return nil, err
	}
	pl.Cmds = append(pl.Cmds, cmd)

	for p.peek().Type == lexer.TOKEN_PIPE || p.peek().Type == lexer.TOKEN_PIPE_ERR {
		pipeErr := p.advance().Type == lexer.TOKEN_PIPE_ERR
		pl.PipeErr = pl.PipeErr || pipeErr
		p.skipNewlines()
		cmd, err = p.parseCommand()
		if err != nil {
			return nil, err
		}
		pl.Cmds = append(pl.Cmds, cmd)
	}

	return pl, nil
}

// ─── Command ──────────────────────────────────────────────────────────────

func (p *Parser) parseCommand() (Node, error) {
	tok := p.peek()

	switch tok.Type {
	case lexer.TOKEN_IF:
		return p.parseIf()
	case lexer.TOKEN_FOR:
		return p.parseFor()
	case lexer.TOKEN_WHILE:
		return p.parseWhile(false)
	case lexer.TOKEN_UNTIL:
		return p.parseWhile(true)
	case lexer.TOKEN_CASE:
		return p.parseCase()
	case lexer.TOKEN_FUNCTION:
		return p.parseFuncDef()
	case lexer.TOKEN_LBRACE:
		return p.parseBraceGroup()
	case lexer.TOKEN_LPAREN:
		return p.parseSubshell()
	case lexer.TOKEN_WORD:
		// Checa se é definição de função: name()
		if p.isFuncDef() {
			return p.parseFuncDefShort()
		}
		return p.parseSimpleCmd()
	}

	return p.parseSimpleCmd()
}

func (p *Parser) isFuncDef() bool {
	// word ( ) { ...
	if p.peek().Type != lexer.TOKEN_WORD {
		return false
	}
	i := 1
	for p.peekAt(i).Type == lexer.TOKEN_NEWLINE {
		i++
	}
	if p.peekAt(i).Type != lexer.TOKEN_LPAREN {
		return false
	}
	i++
	for p.peekAt(i).Type == lexer.TOKEN_NEWLINE {
		i++
	}
	return p.peekAt(i).Type == lexer.TOKEN_RPAREN
}

// ─── Simple Command ───────────────────────────────────────────────────────

func (p *Parser) parseSimpleCmd() (*SimpleCmd, error) {
	cmd := &SimpleCmd{}

	for {
		tok := p.peek()

		// Atribuição: VAR=value no início
		if tok.Type == lexer.TOKEN_WORD && isAssignment(tok.Value) && len(cmd.Args) == 0 {
			assign, err := p.parseAssign()
			if err != nil {
				return nil, err
			}
			cmd.Assigns = append(cmd.Assigns, assign)
			continue
		}

		// Redirecionamento
		if isRedirect(tok.Type) || (tok.Type == lexer.TOKEN_WORD && isNumericFd(tok.Value)) {
			redir, err := p.parseRedirect()
			if err != nil {
				return nil, err
			}
			cmd.Redirects = append(cmd.Redirects, redir)
			continue
		}

		// Argumento
		if tok.Type == lexer.TOKEN_WORD || tok.Type == lexer.TOKEN_STRING ||
			tok.Type == lexer.TOKEN_SUBSHELL || tok.Type == lexer.TOKEN_BACKTICK ||
			tok.Type == lexer.TOKEN_ARITH {
			word, err := p.parseWord()
			if err != nil {
				return nil, err
			}
			cmd.Args = append(cmd.Args, word)
			continue
		}

		break
	}

	return cmd, nil
}

func isAssignment(s string) bool {
	if len(s) == 0 {
		return false
	}
	eq := strings.IndexByte(s, '=')
	if eq <= 0 {
		return false
	}
	name := s[:eq]
	for i, c := range name {
		if i == 0 && (c >= '0' && c <= '9') {
			return false
		}
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

func isNumericFd(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func isRedirect(t lexer.TokenType) bool {
	return t == lexer.TOKEN_REDIRECT_IN || t == lexer.TOKEN_REDIRECT_OUT ||
		t == lexer.TOKEN_REDIRECT_APPEND || t == lexer.TOKEN_REDIRECT_ERR ||
		t == lexer.TOKEN_REDIRECT_ERR_APPEND || t == lexer.TOKEN_REDIRECT_BOTH ||
		t == lexer.TOKEN_HEREDOC || t == lexer.TOKEN_HEREDOC_STRIP
}

func (p *Parser) parseAssign() (*Assign, error) {
	tok := p.advance()
	eq := strings.IndexByte(tok.Value, '=')
	name := tok.Value[:eq]
	valStr := tok.Value[eq+1:]

	l := lexer.New(valStr)
	toks, err := l.Tokenize()
	if err != nil {
		return nil, err
	}
	pp := New(toks)
	word, err := pp.parseWord()
	if err != nil {
		// valor vazio
		return &Assign{Name: name, Value: &Word{}}, nil
	}

	return &Assign{Name: name, Value: word}, nil
}

func (p *Parser) parseRedirect() (*Redirect, error) {
	tok := p.advance()
	redir := &Redirect{Op: tok.Value, Fd: -1}

	// File destino
	if p.peek().Type == lexer.TOKEN_WORD || p.peek().Type == lexer.TOKEN_STRING {
		word, err := p.parseWord()
		if err != nil {
			return nil, err
		}
		redir.File = word
	}

	return redir, nil
}

func (p *Parser) parseWord() (*Word, error) {
	tok := p.advance()
	word := &Word{}
	word.Parts = append(word.Parts, &LiteralPart{Value: tok.Value})
	return word, nil
}

// ─── Compostos ────────────────────────────────────────────────────────────

func (p *Parser) parseIf() (*IfCmd, error) {
	p.advance() // if
	p.skipNewlines()

	cmd := &IfCmd{}

	cond, err := p.parseList()
	if err != nil {
		return nil, err
	}
	cmd.Condition = cond

	if _, err := p.expect(lexer.TOKEN_THEN); err != nil {
		p.skipNewlines()
		if p.peek().Type != lexer.TOKEN_THEN {
			return nil, err
		}
		p.advance()
	}

	then, err := p.parseList()
	if err != nil {
		return nil, err
	}
	cmd.Then = then

	for p.peek().Type == lexer.TOKEN_ELIF {
		p.advance()
		p.skipNewlines()
		elifCond, err := p.parseList()
		if err != nil {
			return nil, err
		}
		if p.peek().Type == lexer.TOKEN_THEN {
			p.advance()
		}
		elifBody, err := p.parseList()
		if err != nil {
			return nil, err
		}
		cmd.Elifs = append(cmd.Elifs, ElseIf{Condition: elifCond, Body: elifBody})
	}

	if p.peek().Type == lexer.TOKEN_ELSE {
		p.advance()
		p.skipNewlines()
		elseBody, err := p.parseList()
		if err != nil {
			return nil, err
		}
		cmd.Else = elseBody
	}

	if _, err := p.expect(lexer.TOKEN_FI); err != nil {
		return nil, err
	}

	return cmd, nil
}

func (p *Parser) parseFor() (*ForCmd, error) {
	p.advance() // for
	p.skipNewlines()

	varTok, err := p.expect(lexer.TOKEN_WORD)
	if err != nil {
		return nil, err
	}
	cmd := &ForCmd{Var: varTok.Value}
	p.skipNewlines()

	if p.peek().Type == lexer.TOKEN_IN {
		p.advance()
		for p.peek().Type == lexer.TOKEN_WORD || p.peek().Type == lexer.TOKEN_STRING {
			w, err := p.parseWord()
			if err != nil {
				return nil, err
			}
			cmd.Words = append(cmd.Words, w)
		}
	}

	p.skipNewlines()
	if p.peek().Type == lexer.TOKEN_SEMICOLON {
		p.advance()
	}
	p.skipNewlines()
	if _, err := p.expect(lexer.TOKEN_DO); err != nil {
		return nil, err
	}

	body, err := p.parseList()
	if err != nil {
		return nil, err
	}
	cmd.Body = body

	if _, err := p.expect(lexer.TOKEN_DONE); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseWhile(until bool) (*WhileCmd, error) {
	p.advance() // while ou until
	p.skipNewlines()

	cmd := &WhileCmd{Until: until}

	cond, err := p.parseList()
	if err != nil {
		return nil, err
	}
	cmd.Condition = cond

	p.skipNewlines()
	if _, err := p.expect(lexer.TOKEN_DO); err != nil {
		return nil, err
	}

	body, err := p.parseList()
	if err != nil {
		return nil, err
	}
	cmd.Body = body

	if _, err := p.expect(lexer.TOKEN_DONE); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseCase() (*CaseCmd, error) {
	p.advance() // case
	p.skipNewlines()

	word, err := p.parseWord()
	if err != nil {
		return nil, err
	}
	cmd := &CaseCmd{Word: word}
	p.skipNewlines()

	// expect 'in'
	if p.peek().Type == lexer.TOKEN_IN {
		p.advance()
	}
	p.skipNewlines()

	for p.peek().Type != lexer.TOKEN_ESAC && p.peek().Type != lexer.TOKEN_EOF {
		item := CaseItem{}

		// Padrões: pat1|pat2)
		for p.peek().Type == lexer.TOKEN_WORD || p.peek().Type == lexer.TOKEN_STRING {
			w, err := p.parseWord()
			if err != nil {
				return nil, err
			}
			item.Patterns = append(item.Patterns, w)
			if p.peek().Type == lexer.TOKEN_PIPE {
				p.advance()
			} else {
				break
			}
		}

		// Consome )
		if p.peek().Type == lexer.TOKEN_RPAREN {
			p.advance()
		}
		p.skipNewlines()

		// Body até ;;
		body, err := p.parseCaseBody()
		if err != nil {
			return nil, err
		}
		item.Body = body

		// Consome ;;
		if p.peek().Type == lexer.TOKEN_WORD && p.peek().Value == ";;" {
			p.advance()
		} else if p.peek().Type == lexer.TOKEN_SEMICOLON {
			p.advance()
			if p.peek().Type == lexer.TOKEN_SEMICOLON {
				p.advance()
			}
		}
		p.skipNewlines()

		cmd.Items = append(cmd.Items, item)
	}

	if _, err := p.expect(lexer.TOKEN_ESAC); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (p *Parser) parseCaseBody() (*List, error) {
	list := &List{}
	for {
		p.skipNewlines()
		t := p.peek()
		if t.Type == lexer.TOKEN_ESAC || t.Type == lexer.TOKEN_EOF {
			break
		}
		if t.Type == lexer.TOKEN_WORD && t.Value == ";;" {
			break
		}
		if t.Type == lexer.TOKEN_SEMICOLON && p.peekAt(1).Type == lexer.TOKEN_SEMICOLON {
			break
		}
		aol, err := p.parseAndOr()
		if err != nil {
			return nil, err
		}
		item := ListItem{Node: aol}
		if p.peek().Type == lexer.TOKEN_SEMICOLON {
			p.advance()
		}
		list.Items = append(list.Items, item)
	}
	return list, nil
}

func (p *Parser) parseFuncDef() (*FuncDef, error) {
	p.advance() // function
	nameTok, err := p.expect(lexer.TOKEN_WORD)
	if err != nil {
		return nil, err
	}
	// Opcionais ()
	if p.peek().Type == lexer.TOKEN_LPAREN {
		p.advance()
		if _, err := p.expect(lexer.TOKEN_RPAREN); err != nil {
			return nil, err
		}
	}
	p.skipNewlines()
	body, err := p.parseCommand()
	if err != nil {
		return nil, err
	}
	return &FuncDef{Name: nameTok.Value, Body: body}, nil
}

func (p *Parser) parseFuncDefShort() (*FuncDef, error) {
	nameTok := p.advance() // name
	p.advance()            // (
	p.skipNewlines()
	p.advance() // )
	p.skipNewlines()
	body, err := p.parseCommand()
	if err != nil {
		return nil, err
	}
	return &FuncDef{Name: nameTok.Value, Body: body}, nil
}

func (p *Parser) parseBraceGroup() (*BraceGroup, error) {
	p.advance() // {
	p.skipNewlines()
	body, err := p.parseList()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TOKEN_RBRACE); err != nil {
		return nil, err
	}
	bg := &BraceGroup{Body: body}
	// Redirecionamentos após }
	for isRedirect(p.peek().Type) {
		redir, err := p.parseRedirect()
		if err != nil {
			return nil, err
		}
		bg.Redirects = append(bg.Redirects, redir)
	}
	return bg, nil
}

func (p *Parser) parseSubshell() (*SubshellCmd, error) {
	p.advance() // (
	p.skipNewlines()
	body, err := p.parseList()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.TOKEN_RPAREN); err != nil {
		return nil, err
	}
	return &SubshellCmd{Body: body}, nil
}
