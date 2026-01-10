# asmcalc

A sample compiler that translates arithmetic expressions into GAS (GNU Assembler) format x86-64 assembly code.

## Features

- Compiles arithmetic expressions (addition, subtraction, multiplication, division) to x86-64 assembly
- Generates standalone executables that print the result
- Supports operator precedence (multiplication/division before addition/subtraction)
- Supports parentheses for grouping

## Requirements

- Go (for building the compiler)
- GNU `as` (assembler)
- GNU `ld` (linker)

## Building

```sh
cd cmd/asmcalc
go build
```

## Usage

Basic usage:

```sh
./asmcalc "3*4+5"
```

This outputs GAS format assembly code to stdout.

### Compile and run the expression

To compile the expression to an executable and run it:

```sh
./asmcalc "3*4+5" | as -64 - -o out.o
ld out.o -o out
./out
```

The output will be `17` (the result of 3*4+5).

### Example with cleanup

```sh
trap 'rm -f out.o out' EXIT
./asmcalc "(2+3)*4" | as -64 - -o out.o
ld out.o -o out
./out
```

## Testing

Run the test script:

```sh
cd cmd/asmcalc
./test.sh
```

The test script compiles the expression `1+2-3+4`, links it into an executable, runs it, and verifies the output is `4`.

## Examples

More examples:

```sh
# Operator precedence: multiplication before addition
./asmcalc "2+3*4" | as -64 - -o out.o && ld out.o -o out && ./out
# Output: 14

# Parentheses for grouping
./asmcalc "(2+3)*4" | as -64 - -o out.o && ld out.o -o out && ./out
# Output: 20

# Division
./asmcalc "20/4+2" | as -64 - -o out.o && ld out.o -o out && ./out
# Output: 7
```

## How it works

1. The compiler lexes and parses arithmetic expressions
2. Generates x86-64 assembly using a stack-based approach
3. The generated assembly uses Linux syscalls to print the result and exit
4. The assembly is piped to `as` (assembler) which creates an object file
5. The object file is linked with `ld` to create a standalone executable
