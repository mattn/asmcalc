package asmcalc

import (
	"fmt"
	"os"
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

func (c *Compiler) Compile() error {
	out := os.Stdout
	fmt.Fprintln(out, ".text")
	fmt.Fprintln(out, ".globl _start")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "_start:")
	c.emitExpr(out)

	fmt.Fprintln(out, "  movq $60, %rax       # Syscall: exit")
	fmt.Fprintln(out, "  xorq %rdi, %rdi      # Exit code: 0")
	fmt.Fprintln(out, "  syscall              # Call kernel")
	fmt.Fprintln(out)
	fmt.Fprintln(out, ".bss")
	fmt.Fprintln(out, ".space 32              # 32-byte buffer for number")
	fmt.Fprintln(out, "buffer:")
	return nil
}

func (c *Compiler) emitExpr(out *os.File) {
	c.emitTerm(out)
	for c.peek().Type == TOK_PLUS || c.peek().Type == TOK_MINUS {
		op := c.consume(c.peek().Type).Type
		c.emitTerm(out)
		fmt.Fprintln(out, "  popq %rax            # Get second operand")
		fmt.Fprintln(out, "  popq %rbx            # Get first operand")
		if op == TOK_PLUS {
			fmt.Fprintln(out, "  addq %rbx, %rax      # Add them")
		} else {
			fmt.Fprintln(out, "  subq %rax, %rbx      # Subtract")
			fmt.Fprintln(out, "  movq %rbx, %rax      # Result in RAX")

		}
		fmt.Fprintln(out, "  pushq %rax           # Save result")
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

func (c *Compiler) emitFactor(out *os.File) {
	if c.peek().Type == TOK_NUM {
		tok := c.consume(TOK_NUM)
		_ = tok
		// load number into rax and push to stack
		return
	}
	if c.peek().Type == TOK_LPAREN {
		c.consume(TOK_LPAREN)
		c.emitExpr(out)
		c.consume(TOK_RPAREN)
		return
	}
	panic("unexpected token\n")
}

func (c *Compiler) emitTerm(out *os.File) {
	c.emitFactor(out)
	for c.peek().Type == TOK_MUL || c.peek().Type == TOK_DIV {
		op := c.consume(c.peek().Type).Type
		c.emitFactor(out)
		fmt.Fprintln(out, "  popq %rax            # Get second operand")
		fmt.Fprintln(out, "  popq %rbx            # Get first operand")
		if op == TOK_MUL {
			fmt.Fprintln(out, "  imulq %rbx, %rax     # Multiply")
		} else {
			fmt.Fprintln(out, "  movq %rax, %rcx      # Save divisor")
			fmt.Fprintln(out, "  movq %rbx, %rax      # Move dividend to RAX")
			fmt.Fprintln(out, "  xorq %rdx, %rdx      # Clear RDX for division")
			fmt.Fprintln(out, "  idivq %rcx           # Divide RDX:RAX by divisor")
		}
		fmt.Fprintln(out, "  pushq %rax           # Save result")
	}
}
