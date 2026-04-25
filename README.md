# mame

A tiny toy compiler that translates expressions into GAS (GNU Assembler) format x86-64 assembly code.

Supports both Linux and Windows.

## Features

- Arithmetic: `+`, `-`, `*`, `/`, `%`, parentheses, operator precedence
- Variables: `x = expr`
- Multiple statements separated by `;` or newlines
- Command-line arguments via `$1`, `$2`, ...
- `println(expr)` builtin: prints the value with a newline and returns it

## Requirements

- Go (to build the compiler)
- GNU `as` (assembler)
- GNU `ld` (linker)
  - On Windows, `-lkernel32 -lshell32` are linked

## Building

```sh
go build ./cmd/mame
```

## Usage

```
mame [-run|-eval] [-f file] expr [args...]
```

- default: prints generated assembly to stdout
- `-run`: compiles, assembles (`as`), links (`ld`), and executes
- `-eval`: evaluates in-process via the built-in interpreter (no asm pipeline; useful for quick checks)

### Quick start

```sh
./mame -run 'println(3*4+5)'
# 17
```

### Pass arguments

```sh
./mame -run 'println($1 * $2)' 6 7
# 42
```

### Read from file

```sh
echo 'x = 10
y = 20
println(x + y)' > prog.mame

./mame -run -f prog.mame
# 30
```

### Manual pipeline

```sh
./mame 'println((2+3)*4)' | as -64 - -o out.o
ld out.o -o out                  # Linux
# ld out.o -o out.exe -lkernel32 -lshell32   # Windows
./out
# 20
```

## Examples

```sh
./mame -run 'println(10 % 3)'           # 1
./mame -run 'x = 5; println(x * x)'     # 25
./mame -run 'println($1 + 1)' 41        # 42
```

## How it works

1. Lexes the input into tokens
2. Parses tokens into an AST (`Program`, `Stmt`, `Expr`)
3. Walks the AST to emit x86-64 assembly using a stack-based evaluation strategy
4. Emits platform-specific runtime helpers (`__atoi` for `$N`, `__println_int` for `println`) as needed
5. Pipes the assembly to `as` and links with `ld` to produce a standalone executable
