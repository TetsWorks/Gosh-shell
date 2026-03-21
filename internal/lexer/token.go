package lexer

// TokenType representa o tipo de cada token
type TokenType int

const (
	// Literais
	TOKEN_WORD    TokenType = iota // palavra comum / comando
	TOKEN_STRING                   // string entre aspas
	TOKEN_NUMBER                   // número literal

	// Operadores de redirecionamento
	TOKEN_REDIRECT_IN    // <
	TOKEN_REDIRECT_OUT   // >
	TOKEN_REDIRECT_APPEND // >>
	TOKEN_REDIRECT_ERR   // 2>
	TOKEN_REDIRECT_ERR_APPEND // 2>>
	TOKEN_REDIRECT_BOTH  // &>
	TOKEN_HEREDOC        // <<
	TOKEN_HEREDOC_STRIP  // <<-

	// Operadores de controle
	TOKEN_PIPE         // |
	TOKEN_PIPE_ERR     // |&
	TOKEN_AND          // &&
	TOKEN_OR           // ||
	TOKEN_SEMICOLON    // ;
	TOKEN_BACKGROUND   // &
	TOKEN_NEWLINE      // \n

	// Agrupamento
	TOKEN_LPAREN  // (
	TOKEN_RPAREN  // )
	TOKEN_LBRACE  // {
	TOKEN_RBRACE  // }

	// Substituição
	TOKEN_SUBSHELL   // $(...)
	TOKEN_BACKTICK   // `...`
	TOKEN_ARITH      // $((...))
	TOKEN_PROCESS_SUB_IN  // <(...)
	TOKEN_PROCESS_SUB_OUT // >(...)

	// Palavras-chave POSIX
	TOKEN_IF
	TOKEN_THEN
	TOKEN_ELSE
	TOKEN_ELIF
	TOKEN_FI
	TOKEN_FOR
	TOKEN_IN
	TOKEN_DO
	TOKEN_DONE
	TOKEN_WHILE
	TOKEN_UNTIL
	TOKEN_CASE
	TOKEN_ESAC
	TOKEN_FUNCTION
	TOKEN_SELECT
	TOKEN_TIME

	// Especiais
	TOKEN_EOF
	TOKEN_ILLEGAL
)

var keywords = map[string]TokenType{
	"if":       TOKEN_IF,
	"then":     TOKEN_THEN,
	"else":     TOKEN_ELSE,
	"elif":     TOKEN_ELIF,
	"fi":       TOKEN_FI,
	"for":      TOKEN_FOR,
	"in":       TOKEN_IN,
	"do":       TOKEN_DO,
	"done":     TOKEN_DONE,
	"while":    TOKEN_WHILE,
	"until":    TOKEN_UNTIL,
	"case":     TOKEN_CASE,
	"esac":     TOKEN_ESAC,
	"function": TOKEN_FUNCTION,
	"select":   TOKEN_SELECT,
	"time":     TOKEN_TIME,
}

// LookupKeyword verifica se uma palavra é keyword
func LookupKeyword(word string) (TokenType, bool) {
	t, ok := keywords[word]
	return t, ok
}

// Token representa um token individual
type Token struct {
	Type    TokenType
	Value   string
	Line    int
	Column  int
}

func (t Token) String() string {
	return t.Value
}

func (tt TokenType) String() string {
	names := map[TokenType]string{
		TOKEN_WORD:          "WORD",
		TOKEN_STRING:        "STRING",
		TOKEN_NUMBER:        "NUMBER",
		TOKEN_REDIRECT_IN:   "<",
		TOKEN_REDIRECT_OUT:  ">",
		TOKEN_REDIRECT_APPEND: ">>",
		TOKEN_REDIRECT_ERR:  "2>",
		TOKEN_HEREDOC:       "<<",
		TOKEN_PIPE:          "|",
		TOKEN_PIPE_ERR:      "|&",
		TOKEN_AND:           "&&",
		TOKEN_OR:            "||",
		TOKEN_SEMICOLON:     ";",
		TOKEN_BACKGROUND:    "&",
		TOKEN_NEWLINE:       "NEWLINE",
		TOKEN_LPAREN:        "(",
		TOKEN_RPAREN:        ")",
		TOKEN_LBRACE:        "{",
		TOKEN_RBRACE:        "}",
		TOKEN_IF:            "if",
		TOKEN_THEN:          "then",
		TOKEN_ELSE:          "else",
		TOKEN_ELIF:          "elif",
		TOKEN_FI:            "fi",
		TOKEN_FOR:           "for",
		TOKEN_IN:            "in",
		TOKEN_DO:            "do",
		TOKEN_DONE:          "done",
		TOKEN_WHILE:         "while",
		TOKEN_UNTIL:         "until",
		TOKEN_CASE:          "case",
		TOKEN_ESAC:          "esac",
		TOKEN_FUNCTION:      "function",
		TOKEN_EOF:           "EOF",
	}
	if s, ok := names[tt]; ok {
		return s
	}
	return "UNKNOWN"
}
