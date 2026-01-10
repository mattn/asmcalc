package asmcalc

import (
	"fmt"
	"io"
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
		panic(fmt.Sprintf("unknown char: %c\n", ch))
	}
	c.tokens = append(c.tokens, Token{Type: TOK_EOF})
}

func (c *Compiler) Compile(w io.Writer) error {
	fmt.Fprintln(w, ".text")
	fmt.Fprintln(w, ".globl _start")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "_start:")
	c.emitExpr(w)
	fmt.Fprintln(w, "  popq %rax                  # Result on stack -> RAX")
	fmt.Fprintln(w, "  movq $10, %rbx             # Base 10 for conversion")
	fmt.Fprintln(w, "  leaq buffer+31(%rip), %rcx # Start at end of buffer")
	fmt.Fprintln(w, "  movb $10, (%rcx)           # Add newline")
	fmt.Fprintln(w, "  decq %rcx                  # Move back")
	fmt.Fprintln(w, "  movb $0, (%rcx)            # Null terminator (unused)")
	fmt.Fprintln(w, "  movq $60, %rax             # Syscall: exit")
	fmt.Fprintln(w, "  xorq %rdi, %rdi            # Exit code: 0")
	fmt.Fprintln(w, "  syscall                    # Call kernel")
	fmt.Fprintln(w)
	fmt.Fprintln(w, ".bss")
	fmt.Fprintln(w, ".space 32              # 32-byte buffer for number")
	fmt.Fprintln(w, "buffer:")
	return nil
}

func (c *Compiler) emitExpr(w io.Writer) {
	c.emitTerm(w)
	for c.peek().Type == TOK_PLUS || c.peek().Type == TOK_MINUS {
		op := c.consume(c.peek().Type).Type
		c.emitTerm(w)
		fmt.Fprintln(w, "  popq %rax                  # Get second operand")
		fmt.Fprintln(w, "  popq %rbx                  # Get first operand")
		if op == TOK_PLUS {
			fmt.Fprintln(w, "  addq %rbx, %rax            # Add them")
		} else {
			fmt.Fprintln(w, "  subq %rax, %rbx            # Subtract")
			fmt.Fprintln(w, "  movq %rbx, %rax            # Result in RAX")

		}
		fmt.Fprintln(w, "  pushq %rax                 # Save result")
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
	panic(fmt.Sprintf("expected token type %d\n", typ))
}

func (c *Compiler) emitFactor(w io.Writer) {
	if c.peek().Type == TOK_NUM {
		tok := c.consume(TOK_NUM)
		fmt.Fprintf(w, "  movq $%d, %%rax              # Load number\n", tok.Value)
		fmt.Fprintln(w, "  pushq %rax                 # Push to stack")
		return
	}
	if c.peek().Type == TOK_LPAREN {
		c.consume(TOK_LPAREN)
		c.emitExpr(w)
		c.consume(TOK_RPAREN)
		return
	}
	panic("unexpected token\n")
}

func (c *Compiler) emitTerm(w io.Writer) {
	c.emitFactor(w)
	for c.peek().Type == TOK_MUL || c.peek().Type == TOK_DIV {
		op := c.consume(c.peek().Type).Type
		c.emitFactor(w)
		fmt.Fprintln(w, "  popq %rax                   # Get second operand")
		fmt.Fprintln(w, "  popq %rbx                   # Get first operand")
		if op == TOK_MUL {
			fmt.Fprintln(w, "  imulq %rbx, %rax            # Multiply")
		} else {
			fmt.Fprintln(w, "  movq %rax, %rcx             # Save divisor")
			fmt.Fprintln(w, "  movq %rbx, %rax             # Move dividend to RAX")
			fmt.Fprintln(w, "  xorq %rdx, %rdx      # Clear RDX for division")
			fmt.Fprintln(w, "  idivq %rcx           # Divide RDX:RAX by divisor")
		}
		fmt.Fprintln(w, "  pushq %rax           # Save result")
	}
}
