package asmcalc

import (
	"fmt"
	"io"
	"runtime"
	"strings"
	"unicode"
)

type TokenType int

const (
	TOK_NUM TokenType = iota
	TOK_PLUS
	TOK_MINUS
	TOK_MUL
	TOK_DIV
	TOK_LPAREN
	TOK_RPAREN
	TOK_IDENT
	TOK_ASSIGN
	TOK_EOF
)

type Token struct {
	Type  TokenType
	Value int
	Name  string
}

type Compiler struct {
	input    string
	pos      int
	tokens   []Token
	tokenPos int
}

func NewCompiler(input string) *Compiler {
	return &Compiler{
		input:  input,
		tokens: make([]Token, 0),
	}
}

func write(w io.Writer, s string) {
	w.Write([]byte(s + "\n"))
}

func (c *Compiler) Lex() {
	c.tokens = make([]Token, 0)
	c.pos = 0

	for c.pos < len(c.input) {
		ch := c.input[c.pos]
		if unicode.IsSpace(rune(ch)) {
			c.pos++
			continue
		}
		if unicode.IsDigit(rune(ch)) {
			value := 0
			for c.pos < len(c.input) && unicode.IsDigit(rune(c.input[c.pos])) {
				value = value*10 + int(c.input[c.pos]-'0')
				c.pos++
			}
			c.tokens = append(c.tokens, Token{Type: TOK_NUM, Value: value})
			continue
		}
		if unicode.IsLetter(rune(ch)) {
			var name strings.Builder
			for c.pos < len(c.input) && unicode.IsLetter(rune(c.input[c.pos])) {
				name.WriteByte(c.input[c.pos])
				c.pos++
			}
			nameStr := name.String()
			if nameStr == "set" {
				c.tokens = append(c.tokens, Token{Type: TOK_ASSIGN})
			} else {
				c.tokens = append(c.tokens, Token{Type: TOK_IDENT, Name: nameStr})
			}
			continue
		}
		if ch == '+' {
			c.tokens = append(c.tokens, Token{Type: TOK_PLUS})
			c.pos++
			continue
		}
		if ch == '-' {
			c.tokens = append(c.tokens, Token{Type: TOK_MINUS})
			c.pos++
			continue
		}
		if ch == '*' {
			c.tokens = append(c.tokens, Token{Type: TOK_MUL})
			c.pos++
			continue
		}
		if ch == '/' {
			c.tokens = append(c.tokens, Token{Type: TOK_DIV})
			c.pos++
			continue
		}
		if ch == '(' {
			c.tokens = append(c.tokens, Token{Type: TOK_LPAREN})
			c.pos++
			continue
		}
		if ch == ')' {
			c.tokens = append(c.tokens, Token{Type: TOK_RPAREN})
			c.pos++
			continue
		}
		panic(fmt.Sprintf("unknown char: %c", ch))
	}
	c.tokens = append(c.tokens, Token{Type: TOK_EOF})
}

func (c *Compiler) Eval() int {
	c.tokenPos = 0
	return c.evalExpr()
}

func (c *Compiler) Compile(w io.Writer) error {
	c.tokenPos = 0

	if runtime.GOOS == "windows" {
		return c.compileWindows(w)
	}
	return c.compileLinux(w)
}

func (c *Compiler) compileLinux(w io.Writer) error {
	write(w, ".text")
	write(w, ".globl _start")
	write(w, "")
	write(w, "_start:")
	c.emitExpr(w)
	write(w, "  popq %rax                  # Result on stack -> RAX")
	write(w, "  movq $10, %rbx             # Base 10 for conversion")
	write(w, "  leaq buffer+31(%rip), %rcx # Start at end of buffer")
	write(w, "  movb $10, (%rcx)           # Add newline")
	write(w, "  decq %rcx                  # Move back")
	write(w, "  movb $0, (%rcx)            # Null terminator (unused)")
	write(w, "")
	write(w, "convert_loop:")
	write(w, "  xorq %rdx, %rdx            # Clear RDX for division")
	write(w, "  divq %rbx                  # RAX / 10, remainder in RDX")
	write(w, "  addb $48, %dl              # Convert to ASCII")
	write(w, "  movb %dl, (%rcx)           # Store character")
	write(w, "  decq %rcx                  # Move back in buffer")
	write(w, "  testq %rax, %rax           # Check if more digits")
	write(w, "  jnz convert_loop           # Continue if not zero")
	write(w, "")
	write(w, "  incq %rcx                  # Move to first digit")
	write(w, "  movq $1, %rax              # Syscall: write")
	write(w, "  movq $1, %rdi              # File descriptor: stdout")
	write(w, "  movq %rcx, %rsi            # Buffer address")
	write(w, "  leaq buffer+32(%rip), %rdx # End of buffer")
	write(w, "  subq %rsi, %rdx            # Calculate length")
	write(w, "  syscall                    # Call kernel")

	write(w, "  movq $60, %rax             # Syscall: exit")
	write(w, "  xorq %rdi, %rdi            # Exit code: 0")
	write(w, "  syscall                    # Call kernel")
	write(w, "")
	write(w, ".bss")
	write(w, "buffer:")
	write(w, ".space 32                    # 32-byte buffer for number")
	return nil
}

func (c *Compiler) compileWindows(w io.Writer) error {
	write(w, ".text")
	write(w, ".globl main")
	write(w, "")
	write(w, "main:")
	write(w, "  subq $56, %rsp             # Shadow space + alignment")
	c.emitExpr(w)
	write(w, "  popq %rax                  # Result on stack -> RAX")
	write(w, "  movq $10, %rbx             # Base 10 for conversion")
	write(w, "  leaq buffer+31(%rip), %rcx # Start at end of buffer")
	write(w, "  movb $10, (%rcx)           # Add newline")
	write(w, "  decq %rcx                  # Move back")
	write(w, "")
	write(w, "convert_loop:")
	write(w, "  xorq %rdx, %rdx            # Clear RDX for division")
	write(w, "  divq %rbx                  # RAX / 10, remainder in RDX")
	write(w, "  addb $48, %dl              # Convert to ASCII")
	write(w, "  movb %dl, (%rcx)           # Store character")
	write(w, "  decq %rcx                  # Move back in buffer")
	write(w, "  testq %rax, %rax           # Check if more digits")
	write(w, "  jnz convert_loop           # Continue if not zero")
	write(w, "")
	write(w, "  incq %rcx                  # Move to first digit")
	write(w, "  movq %rcx, %r12            # Save buffer start")
	write(w, "  movq $-11, %rcx            # STD_OUTPUT_HANDLE")
	write(w, "  call GetStdHandle          # Get stdout handle")
	write(w, "  movq %rax, %rcx            # Handle in RCX")
	write(w, "  movq %r12, %rdx            # Buffer address")
	write(w, "  leaq buffer+32(%rip), %r8  # End of buffer")
	write(w, "  subq %rdx, %r8             # Length")
	write(w, "  leaq written(%rip), %r9    # Bytes written")
	write(w, "  movq $0, 32(%rsp)          # lpOverlapped = NULL")
	write(w, "  call WriteFile             # Write to stdout")
	write(w, "")
	write(w, "  xorq %rcx, %rcx            # Exit code 0")
	write(w, "  call ExitProcess           # Exit")
	write(w, "")
	write(w, ".bss")
	write(w, "buffer:")
	write(w, ".space 32                    # 32-byte buffer for number")
	write(w, "written:")
	write(w, ".space 8                     # Bytes written")
	return nil
}

func (c *Compiler) emitExpr(w io.Writer) {
	c.emitTerm(w)
	for c.peek().Type == TOK_PLUS || c.peek().Type == TOK_MINUS {
		op := c.consume(c.peek().Type).Type
		c.emitTerm(w)
		write(w, "  popq %rax                  # Get second operand")
		write(w, "  popq %rbx                  # Get first operand")
		if op == TOK_PLUS {
			write(w, "  addq %rbx, %rax            # Add them")
		} else {
			write(w, "  subq %rax, %rbx            # Subtract")
			write(w, "  movq %rbx, %rax            # Result in RAX")

		}
		write(w, "  pushq %rax                 # Save result")
	}
}

func (c *Compiler) peek() Token {
	if c.tokenPos >= len(c.tokens) {
		return Token{Type: TOK_EOF}
	}
	return c.tokens[c.tokenPos]
}

func (c *Compiler) consume(typ TokenType) Token {
	if c.peek().Type == typ {
		tok := c.tokens[c.tokenPos]
		c.tokenPos++
		return tok
	}
	panic(fmt.Sprintf("expected token type %d", typ))
}

func (c *Compiler) emitFactor(w io.Writer) {
	if c.peek().Type == TOK_NUM {
		tok := c.consume(TOK_NUM)
		s := fmt.Sprintf("  movq $%d, %%rax              # Load number\n", tok.Value)
		write(w, s)
		write(w, "  pushq %rax                 # Push to stack")
		return
	}
	if c.peek().Type == TOK_LPAREN {
		c.consume(TOK_LPAREN)
		c.emitExpr(w)
		c.consume(TOK_RPAREN)
		return
	}
	panic("unexpected token")
}

func (c *Compiler) emitTerm(w io.Writer) {
	c.emitFactor(w)
	for c.peek().Type == TOK_MUL || c.peek().Type == TOK_DIV {
		op := c.consume(c.peek().Type).Type
		c.emitFactor(w)
		write(w, "  popq %rax                  # Get second operand")
		write(w, "  popq %rbx                  # Get first operand")
		if op == TOK_MUL {
			write(w, "  imulq %rbx, %rax           # Multiply")
		} else {
			write(w, "  movq %rax, %rcx            # Save divisor")
			write(w, "  movq %rbx, %rax            # Move dividend to RAX")
			write(w, "  xorq %rdx, %rdx      # Clear RDX for division")
			write(w, "  idivq %rcx           # Divide RDX:RAX by divisor")
		}
		write(w, "  pushq %rax                 # Save result")
	}
}

func (c *Compiler) evalExpr() int {
	result := c.evalTerm()
	for c.peek().Type == TOK_PLUS || c.peek().Type == TOK_MINUS {
		op := c.consume(c.peek().Type).Type
		right := c.evalTerm()
		if op == TOK_PLUS {
			result += right
		} else {
			result -= right
		}
	}
	return result
}

func (c *Compiler) evalTerm() int {
	result := c.evalFactor()
	for c.peek().Type == TOK_MUL || c.peek().Type == TOK_DIV {
		op := c.consume(c.peek().Type).Type
		right := c.evalFactor()
		if op == TOK_MUL {
			result *= right
		} else {
			result /= right
		}
	}
	return result
}

func (c *Compiler) evalFactor() int {
	if c.peek().Type == TOK_NUM {
		tok := c.consume(TOK_NUM)
		return tok.Value
	}
	if c.peek().Type == TOK_LPAREN {
		c.consume(TOK_LPAREN)
		result := c.evalExpr()
		c.consume(TOK_RPAREN)
		return result
	}
	panic("unexpected token")
}
