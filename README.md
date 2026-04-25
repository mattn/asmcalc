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
mame asm  [-f file] expr
mame run  [-f file] expr [args...]
mame eval [-f file] expr [args...]
```

- `asm`: prints generated assembly to stdout
- `run`: compiles, assembles (`as`), links (`ld`), and executes
- `eval`: evaluates in-process via the built-in interpreter (no asm pipeline; useful for quick checks)

### Quick start

```sh
./mame run 'println(3*4+5)'
# 17
```

### Pass arguments

```sh
./mame run 'println($1 * $2)' 6 7
# 42
```

### Read from file

```sh
echo 'x = 10
y = 20
println(x + y)' > prog.mame

./mame run -f prog.mame
# 30
```

### Manual pipeline

```sh
./mame asm 'println((2+3)*4)' | as -64 - -o out.o
ld out.o -o out                  # Linux
# ld out.o -o out.exe -lkernel32 -lshell32   # Windows
./out
# 20
```

## Examples

```sh
./mame run 'println(10 % 3)'           # 1
./mame run 'x = 5; println(x * x)'     # 25
./mame run 'println($1 + 1)' 41        # 42
./mame run 'print("Fizz");println("Buzz")'   # FizzBuzz
```

## How it works

1. Lexes the input into tokens
2. Parses tokens into an AST (`Program`, `Stmt`, `Expr`)
3. Walks the AST to emit x86-64 assembly using a stack-based evaluation strategy
4. Emits platform-specific runtime helpers (`__atoi` for `$N`, `__println_int` for `println`) as needed
5. Pipes the assembly to `as` and links with `ld` to produce a standalone executable

## TODO: how should strings be supported?

Adding string values introduces a design choice that significantly changes the
language's character. Roughly, the options are:

- **Level 1 — string literals only as `println` arguments.** Strings cannot be
  assigned to variables or appear in expressions; `println("Fizz")` is a
  syntactic form. Values stay 8 bytes, no type system needed. Sufficient for
  FizzBuzz.
- **Level 2 — statically typed strings.** Variables can hold a string, but the
  type is fixed at first assignment. Mixing types (`x = "a"; if x < 5`) is a
  compile-time error. Requires a type-inference pass.
- **Level 3 — dynamic typing, runtime errors on mismatch.** Each value carries
  a runtime type tag (16 B per slot). Operators check the tag at runtime; type
  mismatches raise an error mid-execution. Lua-like.
- **Level 4 — dynamic typing with implicit coercion.** Like Level 3 but
  comparisons across types silently coerce. JavaScript-like; semantics get
  hairy fast.

Level 1 is the most natural fit for the current style of the project. Levels 2+
shift mame from "tiny calculator that emits asm" to "a real language with a
type system", which is a deliberate design jump worth discussing before
committing to.
