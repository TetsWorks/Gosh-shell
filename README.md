# gosh — Go Shell

> Um shell POSIX-compatible moderno escrito em Go, com job control, scripting completo, autocompletar e syntax highlighting.

```
  ██████╗  ██████╗ ███████╗██╗  ██╗
 ██╔════╝ ██╔═══██╗██╔════╝██║  ██║
 ██║  ███╗██║   ██║███████╗███████║
 ██║   ██║██║   ██║╚════██║██╔══██║
 ╚██████╔╝╚██████╔╝███████║██║  ██║
  ╚═════╝  ╚═════╝ ╚══════╝╚═╝  ╚═╝  v0.1.0
```

## Features

### Core
- **Parser POSIX completo** — Lexer + parser próprios que constroem uma AST real
- **Scripting completo** — `if/then/elif/else/fi`, `for`, `while`, `until`, `case/esac`, funções
- **Pipelines** — `|`, `|&` (stderr no pipe), `&&`, `||`, `;`, `&` (background)
- **Redirecionamentos** — `>`, `>>`, `<`, `<<` (heredoc), `2>`, `2>>`, `&>`
- **Expansões** — `$VAR`, `${VAR:-default}`, `$(cmd)`, `$((arith))`, `~`, glob (`*`, `?`, `[...]`)

### Interativo
- **Syntax highlighting** em tempo real — keywords azuis, comandos válidos verdes, inválidos vermelhos, strings amarelas, variáveis ciano
- **Autocompletar com Tab** — comandos (PATH + builtins + aliases + histórico), arquivos e diretórios, cicla múltiplas opções
- **Histórico persistente** — salvo em `~/.gosh_history`, navegação com ↑↓, busca reversa com `Ctrl+R`
- **Edição de linha** — `Ctrl+A/E`, `Ctrl+K/U/W`, `Ctrl+L`, `Home/End`, `Del`

### Job Control
- Background com `&` — `sleep 10 &`
- `jobs` — lista jobs ativos
- `fg %N` — traz job N para foreground
- `bg %N` — retoma job N em background
- `Ctrl+Z` — suspende processo foreground
- `kill %N` — mata job N

### Builtins (30+)
`cd`, `pwd`, `echo`, `printf`, `export`, `unset`, `set`, `readonly`, `alias`, `unalias`, `type`, `which`, `test`/`[`, `true`, `false`, `source`/`.`, `exit`, `return`, `break`, `continue`, `jobs`, `fg`, `bg`, `kill`, `wait`, `read`, `exec`, `eval`, `local`, `declare`, `getopts`, `shift`, `history`, `dirs`, `pushd`, `popd`, `help`

### Configuração
- **`~/.goshrc`** — aliases, funções, variáveis (criado automaticamente com defaults úteis)
- **`~/.gosh_profile`** — executado em login shells (`gosh -l`)
- **PS1 customizável** — suporta `\u`, `\h`, `\w`, `\W`, `\$`, `\n`, escapes de cor

## Instalação

### Termux (Android)
```bash
pkg install golang git

git clone https://github.com/yourusername/gosh
cd gosh

make termux
# ou manualmente:
go build -o gosh ./cmd/gosh
cp gosh $PREFIX/bin/
```

### Linux/macOS
```bash
git clone https://github.com/yourusername/gosh
cd gosh
make install
```

### Definir como shell padrão
```bash
# Termux
echo "$PREFIX/bin/gosh" >> $PREFIX/etc/shells
chsh -s gosh  # pode não funcionar no Termux sem root

# Linux
sudo sh -c 'echo /usr/local/bin/gosh >> /etc/shells'
chsh -s /usr/local/bin/gosh
```

## Uso

```bash
# Shell interativo
gosh

# Executar script
gosh script.sh [args...]

# Executar string
gosh -c 'echo hello; ls -la'

# Ler do stdin
echo 'echo hello' | gosh -s

# Login shell
gosh -l

# Com opções
gosh -xe script.sh   # xtrace + exit on error
```

## Scripting

```bash
#!/usr/bin/env gosh

# Variáveis
NAME="mundo"
echo "Hello, $NAME!"

# Aritmética
COUNT=$((10 + 5))
echo "Conta: $COUNT"

# If/else
if [ -f "/etc/passwd" ]; then
    echo "Arquivo existe"
elif [ -d "/etc" ]; then
    echo "Diretório existe"
else
    echo "Nada encontrado"
fi

# For loop
for file in *.go; do
    echo "Processando: $file"
done

# While
i=0
while [ $i -lt 5 ]; do
    echo "i=$i"
    i=$((i + 1))
done

# Case
case "$1" in
    start)  echo "Iniciando..." ;;
    stop)   echo "Parando..." ;;
    *)      echo "Uso: $0 {start|stop}" ;;
esac

# Funções
greet() {
    local name="$1"
    echo "Olá, $name!"
}
greet "gosh"

# Pipes e redirecionamentos
ls -la | grep ".go" | wc -l
echo "Log" >> output.log
cat < input.txt
```

## Expansões suportadas

| Sintaxe | Descrição |
|---|---|
| `$VAR` | Valor da variável |
| `${VAR}` | Valor (delimitado) |
| `${VAR:-default}` | Valor ou default se não definido |
| `${VAR:=default}` | Define e retorna default se não definido |
| `${VAR:+alt}` | alt se VAR definido, vazio caso contrário |
| `${VAR:?msg}` | Erro com msg se não definido |
| `${#VAR}` | Comprimento do valor |
| `$(cmd)` | Substituição de comando |
| `` `cmd` `` | Substituição de comando (legacy) |
| `$((expr))` | Aritmética: `+`, `-`, `*`, `/`, `%` |
| `~` | Home do usuário atual |
| `~user` | Home de outro usuário |
| `*`, `?`, `[...]` | Glob patterns |

## Arquitetura

```
gosh/
├── cmd/gosh/main.go          # Entrypoint, REPL, prompt
├── internal/
│   ├── lexer/                # Tokenizador POSIX
│   │   ├── token.go          # Tipos de tokens
│   │   └── lexer.go          # Lexer completo
│   ├── parser/               # Parser que gera AST
│   │   ├── ast.go            # Nós da AST
│   │   └── parser.go         # Parser recursivo descendente
│   ├── executor/             # Executa a AST
│   │   ├── executor.go       # Engine principal
│   │   └── extra.go          # Métodos auxiliares
│   ├── env/
│   │   └── env.go            # Variáveis + expansões
│   ├── builtin/
│   │   └── builtin.go        # 30+ builtins
│   ├── jobcontrol/
│   │   └── manager.go        # bg/fg/jobs/sinais
│   ├── readline/
│   │   └── readline.go       # Editor + highlight + tab
│   └── rc/
│       └── rc.go             # ~/.goshrc, histórico
├── Makefile
└── README.md
```

**Pipeline de execução:**
```
Input → Lexer → Tokens → Parser → AST → Executor → Output
                                    ↓
                              (expansões, glob,
                               builtins, pipes,
                               job control)
```

## Compatibilidade POSIX

| Feature | Status |
|---|---|
| Simple commands | ✅ |
| Pipelines | ✅ |
| Redirecionamentos | ✅ |
| Heredoc `<<` | ✅ |
| `if/for/while/until/case` | ✅ |
| Funções | ✅ |
| Variáveis e expansões | ✅ |
| `$?`, `$$`, `$!`, `$#`, `$@` | ✅ |
| Job control | ✅ |
| `set -e`, `set -x`, `set -u` | ✅ |
| Subshells `(...)` | ✅ |
| Brace groups `{...}` | ✅ |
| `getopts` | ✅ |
| Arrays | ❌ (roadmap) |
| `[[` (bashism) | ❌ (roadmap) |
| Process substitution `<()` | Parcial |

## Roadmap

- [ ] Arrays (`arr=(a b c)`, `${arr[@]}`)
- [ ] `[[` extended test
- [ ] Coprocess com `coproc`
- [ ] Completions programáticas (tipo `_complete`)
- [ ] Plugin system via `.so`
- [ ] Mode vi para readline
- [ ] `select` interativo
- [ ] Expansão de brace `{a,b,c}`

## Contribuindo

```bash
# Rodar testes
make test

# Testar features básicas
make shell-test

# Coverage
make test-cover

# Lint
make lint
```

## Licença

GPL-3.0 — veja [LICENSE](LICENSE)
