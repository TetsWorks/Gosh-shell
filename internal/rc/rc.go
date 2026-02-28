package rc

import (
	"os"
	"path/filepath"
)

const (
	RCFile      = ".goshrc"
	ProfileFile = ".gosh_profile"
	HistoryFile = ".gosh_history"
)

// GetRCPath retorna o caminho do arquivo de configuração
func GetRCPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, RCFile)
}

// GetProfilePath retorna o caminho do profile (login shell)
func GetProfilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ProfileFile)
}

// GetHistoryPath retorna o caminho do arquivo de histórico
func GetHistoryPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, HistoryFile)
}

// ReadHistory lê o histórico salvo
func ReadHistory(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var lines []string
	line := ""
	for _, ch := range string(data) {
		if ch == '\n' {
			if line != "" {
				lines = append(lines, line)
				line = ""
			}
		} else {
			line += string(ch)
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	// Mantém apenas as últimas 1000 entradas
	if len(lines) > 1000 {
		lines = lines[len(lines)-1000:]
	}
	return lines
}

// SaveHistory salva o histórico em disco
func SaveHistory(path string, history []string) error {
	if path == "" {
		return nil
	}
	// Mantém as últimas 1000
	if len(history) > 1000 {
		history = history[len(history)-1000:]
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, h := range history {
		f.WriteString(h + "\n")
	}
	return nil
}

// DefaultRC retorna o conteúdo padrão do .goshrc
func DefaultRC() string {
	return `# ~/.goshrc - configuração do gosh

# Aliases úteis
alias ll='ls -lah'
alias la='ls -a'
alias l='ls -CF'
alias grep='grep --color=auto'
alias ..='cd ..'
alias ...='cd ../..'
alias mkdir='mkdir -pv'

# Variáveis
export EDITOR='vi'
export PAGER='less'
export HISTSIZE=1000

# Prompt customizado (cores ANSI)
# \u = usuário, \h = host, \w = diretório, \$ = $ ou #
export PS1='\[\033[1;32m\]\u@\h\[\033[0m\]:\[\033[1;34m\]\w\[\033[0m\]\$ '

# Funções úteis
mkcd() {
    mkdir -p "$1" && cd "$1"
}

extract() {
    case "$1" in
        *.tar.gz|*.tgz) tar xzf "$1" ;;
        *.tar.bz2)       tar xjf "$1" ;;
        *.tar.xz)        tar xJf "$1" ;;
        *.zip)           unzip "$1" ;;
        *.gz)            gunzip "$1" ;;
        *.bz2)           bunzip2 "$1" ;;
        *)               echo "Formato não suportado: $1" ;;
    esac
}

# PATH personalizado
export PATH="$HOME/.local/bin:$HOME/bin:$PATH"
`
}

// CreateDefaultRC cria o .goshrc se não existir
func CreateDefaultRC() error {
	path := GetRCPath()
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err == nil {
		return nil // já existe
	}
	return os.WriteFile(path, []byte(DefaultRC()), 0644)
}
