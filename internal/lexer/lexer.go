package lexer

import (
	"fmt"
	"strings"
	"unicode"
)

// Lexer tokeniza input do shell
type Lexer struct {
	input  []rune
	pos    int
	line   int
	col    int
	tokens []Token
}

// New cria um novo Lexer
func New(input string) *Lexer {
	return &Lexer{
		input: []rune(input),
		pos:   0,
		line:  1,
		col:   1,
	}
}

// Tokenize processa todo o input e retorna lista de tokens
func (l *Lexer) Tokenize() ([]Token, error) {
	for {
		tok, err := l.nextToken()
		if err != nil {
			return nil, err
		}
		l.tokens = append(l.tokens, tok)
		if tok.Type == TOKEN_EOF {
			break
		}
	}
	return l.tokens, nil
}

func (l *Lexer) peek() rune {
	if l.pos >= len(l.input) {
		return 0
	}
	return l.input[l.pos]
}

func (l *Lexer) peekAt(offset int) rune {
	p := l.pos + offset
	if p >= len(l.input) {
		return 0
	}
	return l.input[p]
}

func (l *Lexer) advance() rune {
	if l.pos >= len(l.input) {
		return 0
	}
	ch := l.input[l.pos]
	l.pos++
	if ch == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return ch
}

func (l *Lexer) makeToken(t TokenType, val string) Token {
	return Token{Type: t, Value: val, Line: l.line, Column: l.col}
}

func (l *Lexer) skipComment() {
	for l.peek() != '\n' && l.peek() != 0 {
		l.advance()
	}
}

func (l *Lexer) nextToken() (Token, error) {
	// Pula espaços (mas não newlines)
	for l.peek() == ' ' || l.peek() == '\t' || l.peek() == '\r' {
		l.advance()
	}

	if l.pos >= len(l.input) {
		return l.makeToken(TOKEN_EOF, ""), nil
	}

	ch := l.peek()

	// Comentário
	if ch == '#' {
		l.skipComment()
		return l.nextToken()
	}

	// Newline
	if ch == '\n' {
		l.advance()
		return l.makeToken(TOKEN_NEWLINE, "\n"), nil
	}

	// Escape de newline (line continuation)
	if ch == '\\' && l.peekAt(1) == '\n' {
		l.advance()
		l.advance()
		return l.nextToken()
	}

	// Operadores de redirecionamento e controle
	switch ch {
	case '|':
		l.advance()
		if l.peek() == '|' {
			l.advance()
			return l.makeToken(TOKEN_OR, "||"), nil
		}
		if l.peek() == '&' {
			l.advance()
			return l.makeToken(TOKEN_PIPE_ERR, "|&"), nil
		}
		return l.makeToken(TOKEN_PIPE, "|"), nil

	case '&':
		l.advance()
		if l.peek() == '&' {
			l.advance()
			return l.makeToken(TOKEN_AND, "&&"), nil
		}
		if l.peek() == '>' {
			l.advance()
			return l.makeToken(TOKEN_REDIRECT_BOTH, "&>"), nil
		}
		return l.makeToken(TOKEN_BACKGROUND, "&"), nil

	case ';':
		l.advance()
		return l.makeToken(TOKEN_SEMICOLON, ";"), nil

	case '(':
		l.advance()
		return l.makeToken(TOKEN_LPAREN, "("), nil

	case ')':
		l.advance()
		return l.makeToken(TOKEN_RPAREN, ")"), nil

	case '{':
		l.advance()
		return l.makeToken(TOKEN_LBRACE, "{"), nil

	case '}':
		l.advance()
		return l.makeToken(TOKEN_RBRACE, "}"), nil

	case '<':
		l.advance()
		if l.peek() == '<' {
			l.advance()
			if l.peek() == '-' {
				l.advance()
				return l.makeToken(TOKEN_HEREDOC_STRIP, "<<-"), nil
			}
			return l.makeToken(TOKEN_HEREDOC, "<<"), nil
		}
		if l.peek() == '(' {
			content, err := l.readBalanced('(', ')')
			if err != nil {
				return Token{}, err
			}
			return l.makeToken(TOKEN_PROCESS_SUB_IN, "<("+content+")"), nil
		}
		return l.makeToken(TOKEN_REDIRECT_IN, "<"), nil

	case '>':
		l.advance()
		if l.peek() == '>' {
			l.advance()
			return l.makeToken(TOKEN_REDIRECT_APPEND, ">>"), nil
		}
		if l.peek() == '(' {
			content, err := l.readBalanced('(', ')')
			if err != nil {
				return Token{}, err
			}
			return l.makeToken(TOKEN_PROCESS_SUB_OUT, ">("+content+")"), nil
		}
		return l.makeToken(TOKEN_REDIRECT_OUT, ">"), nil

	case '"':
		s, err := l.readDoubleQuote()
		if err != nil {
			return Token{}, err
		}
		return l.makeToken(TOKEN_STRING, s), nil

	case '\'':
		s, err := l.readSingleQuote()
		if err != nil {
			return Token{}, err
		}
		return l.makeToken(TOKEN_STRING, s), nil

	case '`':
		s, err := l.readBacktick()
		if err != nil {
			return Token{}, err
		}
		return l.makeToken(TOKEN_BACKTICK, s), nil

	case '$':
		return l.readDollar()
	}

	// Redirecionamento com fd numérico: 2>, 2>>
	if unicode.IsDigit(ch) && (l.peekAt(1) == '>' || l.peekAt(1) == '<') {
		fd := string(l.advance())
		op := string(l.advance())
		if l.peek() == '>' {
			l.advance()
			return l.makeToken(TOKEN_REDIRECT_ERR_APPEND, fd+op+">"), nil
		}
		return l.makeToken(TOKEN_REDIRECT_ERR, fd+op), nil
	}

	// Palavra genérica
	return l.readWord()
}

func (l *Lexer) readWord() (Token, error) {
	var sb strings.Builder
	for {
		ch := l.peek()
		if ch == 0 || ch == '\n' || ch == ' ' || ch == '\t' || ch == '\r' {
			break
		}
		if ch == '|' || ch == '&' || ch == ';' || ch == '(' || ch == ')' ||
			ch == '{' || ch == '}' || ch == '<' || ch == '>' || ch == '#' {
			break
		}
		// Aspas dentro de palavra
		if ch == '"' {
			s, err := l.readDoubleQuote()
			if err != nil {
				return Token{}, err
			}
			sb.WriteString(s)
			continue
		}
		if ch == '\'' {
			s, err := l.readSingleQuote()
			if err != nil {
				return Token{}, err
			}
			sb.WriteString(s)
			continue
		}
		if ch == '$' {
			tok, err := l.readDollar()
			if err != nil {
				return Token{}, err
			}
			sb.WriteString(tok.Value)
			continue
		}
		if ch == '`' {
			s, err := l.readBacktick()
			if err != nil {
				return Token{}, err
			}
			sb.WriteString("`" + s + "`")
			continue
		}
		if ch == '\\' {
			l.advance()
			next := l.advance()
			sb.WriteRune(next)
			continue
		}
		sb.WriteRune(l.advance())
	}

	word := sb.String()
	if word == "" {
		return Token{}, fmt.Errorf("token vazio inesperado na linha %d", l.line)
	}

	// Checa keyword
	if t, ok := LookupKeyword(word); ok {
		return l.makeToken(t, word), nil
	}

	return l.makeToken(TOKEN_WORD, word), nil
}

func (l *Lexer) readDoubleQuote() (string, error) {
	l.advance() // consome "
	var sb strings.Builder
	for {
		ch := l.peek()
		if ch == 0 {
			return "", fmt.Errorf("string não fechada na linha %d", l.line)
		}
		if ch == '"' {
			l.advance()
			break
		}
		if ch == '\\' {
			l.advance()
			next := l.advance()
			switch next {
			case '"', '\\', '$', '`', '\n':
				sb.WriteRune(next)
			default:
				sb.WriteRune('\\')
				sb.WriteRune(next)
			}
			continue
		}
		if ch == '$' {
			tok, err := l.readDollar()
			if err != nil {
				return "", err
			}
			sb.WriteString(tok.Value)
			continue
		}
		if ch == '`' {
			content, err := l.readBacktick()
			if err != nil {
				return "", err
			}
			sb.WriteString("`" + content + "`")
			continue
		}
		sb.WriteRune(l.advance())
	}
	return sb.String(), nil
}

func (l *Lexer) readSingleQuote() (string, error) {
	l.advance() // consome '
	var sb strings.Builder
	for {
		ch := l.peek()
		if ch == 0 {
			return "", fmt.Errorf("string não fechada na linha %d", l.line)
		}
		if ch == '\'' {
			l.advance()
			break
		}
		sb.WriteRune(l.advance())
	}
	return sb.String(), nil
}

func (l *Lexer) readBacktick() (string, error) {
	l.advance() // consome `
	var sb strings.Builder
	for {
		ch := l.peek()
		if ch == 0 {
			return "", fmt.Errorf("backtick não fechado na linha %d", l.line)
		}
		if ch == '`' {
			l.advance()
			break
		}
		if ch == '\\' && l.peekAt(1) == '`' {
			l.advance()
			sb.WriteRune(l.advance())
			continue
		}
		sb.WriteRune(l.advance())
	}
	return sb.String(), nil
}

func (l *Lexer) readDollar() (Token, error) {
	l.advance() // consome $
	ch := l.peek()

	// Aritmética $((...))
	if ch == '(' && l.peekAt(1) == '(' {
		l.advance()
		l.advance()
		content, err := l.readUntil("))")
		if err != nil {
			return Token{}, err
		}
		return l.makeToken(TOKEN_ARITH, "$(("+content+"))"), nil
	}

	// Substituição de comando $(...)
	if ch == '(' {
		content, err := l.readBalanced('(', ')')
		if err != nil {
			return Token{}, err
		}
		return l.makeToken(TOKEN_SUBSHELL, "$("+content+")"), nil
	}

	// Variável ${...}
	if ch == '{' {
		l.advance()
		var sb strings.Builder
		for l.peek() != '}' && l.peek() != 0 {
			sb.WriteRune(l.advance())
		}
		if l.peek() == '}' {
			l.advance()
		}
		return l.makeToken(TOKEN_WORD, "${"+sb.String()+"}"), nil
	}

	// Variável especial: $?, $$, $!, $#, $*, $@, $0-$9
	if ch == '?' || ch == '$' || ch == '!' || ch == '#' || ch == '*' || ch == '@' {
		return l.makeToken(TOKEN_WORD, "$"+string(l.advance())), nil
	}
	if unicode.IsDigit(ch) {
		return l.makeToken(TOKEN_WORD, "$"+string(l.advance())), nil
	}

	// Variável simples $VAR
	var sb strings.Builder
	for unicode.IsLetter(l.peek()) || unicode.IsDigit(l.peek()) || l.peek() == '_' {
		sb.WriteRune(l.advance())
	}
	if sb.Len() == 0 {
		return l.makeToken(TOKEN_WORD, "$"), nil
	}
	return l.makeToken(TOKEN_WORD, "$"+sb.String()), nil
}

func (l *Lexer) readBalanced(open, close rune) (string, error) {
	l.advance() // consome o open
	var sb strings.Builder
	depth := 1
	for depth > 0 {
		ch := l.peek()
		if ch == 0 {
			return "", fmt.Errorf("expressão não fechada na linha %d", l.line)
		}
		if ch == open {
			depth++
		}
		if ch == close {
			depth--
			if depth == 0 {
				l.advance()
				break
			}
		}
		sb.WriteRune(l.advance())
	}
	return sb.String(), nil
}

func (l *Lexer) readUntil(end string) (string, error) {
	endRunes := []rune(end)
	var sb strings.Builder
	for {
		if l.pos+len(endRunes) > len(l.input) {
			return "", fmt.Errorf("expressão não fechada na linha %d", l.line)
		}
		match := true
		for i, r := range endRunes {
			if l.input[l.pos+i] != r {
				match = false
				break
			}
		}
		if match {
			for range endRunes {
				l.advance()
			}
			break
		}
		sb.WriteRune(l.advance())
	}
	return sb.String(), nil
}
