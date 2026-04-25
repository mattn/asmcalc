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
	TOK_MOD
	TOK_LPAREN
	TOK_RPAREN
	TOK_IDENT
	TOK_ASSIGN
	TOK_SEMI
	TOK_ARG
	TOK_EOF
)

type Token struct {
	Type  TokenType
	Value int
	Name  string
}

type Compiler struct {
	input     string
	pos       int
	tokens    []Token
	tokenPos  int
	program   *Program
	vars      map[string]bool
	varValues map[string]int
	args      []int
	usesArg   bool
	usesPrint bool
}

func NewCompiler(input string) *Compiler {
	return &Compiler{
		input:  input,
		tokens: make([]Token, 0),
	}
}

const commentColumn = 29

func write(w io.Writer, code string, comment ...string) {
	line := code
	if len(comment) > 0 && comment[0] != "" {
		pad := commentColumn - len(line)
		if pad < 1 {
			pad = 1
		}
		line += strings.Repeat(" ", pad) + "# " + comment[0]
	}
	w.Write([]byte(line + "\n"))
}

func (c *Compiler) Lex() {
	c.tokens = make([]Token, 0)
	c.pos = 0

	for c.pos < len(c.input) {
		ch := c.input[c.pos]
		if ch == '\n' {
			c.tokens = append(c.tokens, Token{Type: TOK_SEMI})
			c.pos++
			continue
		}
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
			c.tokens = append(c.tokens, Token{Type: TOK_IDENT, Name: name.String()})
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
		if ch == '%' {
			c.tokens = append(c.tokens, Token{Type: TOK_MOD})
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
		if ch == '=' {
			c.tokens = append(c.tokens, Token{Type: TOK_ASSIGN})
			c.pos++
			continue
		}
		if ch == ';' {
			c.tokens = append(c.tokens, Token{Type: TOK_SEMI})
			c.pos++
			continue
		}
		if ch == '$' {
			c.pos++
			value := 0
			for c.pos < len(c.input) && unicode.IsDigit(rune(c.input[c.pos])) {
				value = value*10 + int(c.input[c.pos]-'0')
				c.pos++
			}
			c.tokens = append(c.tokens, Token{Type: TOK_ARG, Value: value})
			continue
		}
		panic(fmt.Sprintf("unknown char: %c", ch))
	}
	c.tokens = append(c.tokens, Token{Type: TOK_EOF})
}

func (c *Compiler) Eval(args ...int) int {
	if c.program == nil {
		c.Parse()
	}
	c.varValues = map[string]int{}
	c.args = args
	result := 0
	for _, stmt := range c.program.Stmts {
		result = c.evalStmt(stmt)
	}
	return result
}

func (c *Compiler) evalStmt(s Stmt) int {
	switch s := s.(type) {
	case *AssignStmt:
		v := c.evalExpr(s.Value)
		c.varValues[s.Name] = v
		return v
	case *ExprStmt:
		return c.evalExpr(s.X)
	}
	panic("unknown stmt")
}

func (c *Compiler) evalExpr(e Expr) int {
	switch e := e.(type) {
	case *NumLit:
		return e.Value
	case *ArgRef:
		if e.Index < 1 || e.Index > len(c.args) {
			panic(fmt.Sprintf("arg $%d not provided", e.Index))
		}
		return c.args[e.Index-1]
	case *VarRef:
		return c.varValues[e.Name]
	case *CallExpr:
		switch e.Name {
		case "println":
			if len(e.Args) != 1 {
				panic(fmt.Sprintf("println takes 1 arg, got %d", len(e.Args)))
			}
			v := c.evalExpr(e.Args[0])
			fmt.Println(v)
			return v
		}
		panic(fmt.Sprintf("unknown function: %s", e.Name))
	case *BinOp:
		l := c.evalExpr(e.L)
		r := c.evalExpr(e.R)
		switch e.Op {
		case TOK_PLUS:
			return l + r
		case TOK_MINUS:
			return l - r
		case TOK_MUL:
			return l * r
		case TOK_DIV:
			return l / r
		case TOK_MOD:
			return l % r
		}
	}
	panic("unknown expr")
}

func (c *Compiler) Compile(w io.Writer) error {
	if c.program == nil {
		c.Parse()
	}
	c.vars = map[string]bool{}

	if runtime.GOOS == "windows" {
		return c.compileWindows(w)
	}
	return c.compileLinux(w)
}

func (c *Compiler) emitProgram(w io.Writer) {
	for _, stmt := range c.program.Stmts {
		c.emitStmt(w, stmt)
	}
}

func (c *Compiler) emitStmt(w io.Writer, s Stmt) {
	switch s := s.(type) {
	case *AssignStmt:
		c.vars[s.Name] = true
		c.emitExpr(w, s.Value)
		write(w, "  popq %rax", "Read value")
		write(w, fmt.Sprintf("  movq %%rax, var_%s(%%rip)", s.Name), "Store to variable")
	case *ExprStmt:
		c.emitExpr(w, s.X)
		write(w, "  popq %rax", "Discard stmt result")
	}
}

func (c *Compiler) emitBssVars(w io.Writer) {
	for name := range c.vars {
		write(w, fmt.Sprintf("var_%s:", name))
		write(w, ".space 8", "Variable storage")
	}
}

func (c *Compiler) compileLinux(w io.Writer) error {
	write(w, ".text")
	write(w, ".globl _start")
	write(w, "")
	write(w, "_start:")
	write(w, "  movq %rsp, %rbp", "Save argv base")
	c.emitProgram(w)
	write(w, "  movq $60, %rax", "Syscall: exit")
	write(w, "  xorq %rdi, %rdi", "Exit code: 0")
	write(w, "  syscall", "Call kernel")
	write(w, "")
	c.emitAtoi(w)
	c.emitPrintln(w)
	write(w, ".bss")
	if c.usesPrint {
		write(w, "buffer:")
		write(w, ".space 32", "32-byte buffer for number")
	}
	c.emitBssVars(w)
	return nil
}

func (c *Compiler) emitAtoi(w io.Writer) {
	if !c.usesArg {
		return
	}
	if runtime.GOOS == "windows" {
		c.emitAtoiWide(w)
		return
	}
	write(w, "__atoi:")
	write(w, "  xorq %rax, %rax", "result = 0")
	write(w, "  xorq %rcx, %rcx", "sign flag = 0")
	write(w, "  movzbq (%rdi), %rdx")
	write(w, "  cmpb $45, %dl", "'-'")
	write(w, "  jne __atoi_loop")
	write(w, "  movq $1, %rcx", "negative")
	write(w, "  incq %rdi")
	write(w, "__atoi_loop:")
	write(w, "  movzbq (%rdi), %rdx")
	write(w, "  testb %dl, %dl")
	write(w, "  jz __atoi_done")
	write(w, "  subq $48, %rdx", "'0'")
	write(w, "  imulq $10, %rax")
	write(w, "  addq %rdx, %rax")
	write(w, "  incq %rdi")
	write(w, "  jmp __atoi_loop")
	write(w, "__atoi_done:")
	write(w, "  testq %rcx, %rcx")
	write(w, "  jz __atoi_ret")
	write(w, "  negq %rax")
	write(w, "__atoi_ret:")
	write(w, "  ret")
	write(w, "")
}

func (c *Compiler) emitAtoiWide(w io.Writer) {
	write(w, "__atoi:")
	write(w, "  xorq %rax, %rax", "result = 0")
	write(w, "  xorq %rcx, %rcx", "sign flag = 0")
	write(w, "  movzwl (%rdi), %edx")
	write(w, "  cmpw $45, %dx", "L'-'")
	write(w, "  jne __atoi_loop")
	write(w, "  movq $1, %rcx", "negative")
	write(w, "  addq $2, %rdi")
	write(w, "__atoi_loop:")
	write(w, "  movzwl (%rdi), %edx")
	write(w, "  testw %dx, %dx")
	write(w, "  jz __atoi_done")
	write(w, "  subq $48, %rdx", "L'0'")
	write(w, "  imulq $10, %rax")
	write(w, "  addq %rdx, %rax")
	write(w, "  addq $2, %rdi")
	write(w, "  jmp __atoi_loop")
	write(w, "__atoi_done:")
	write(w, "  testq %rcx, %rcx")
	write(w, "  jz __atoi_ret")
	write(w, "  negq %rax")
	write(w, "__atoi_ret:")
	write(w, "  ret")
	write(w, "")
}

func (c *Compiler) compileWindows(w io.Writer) error {
	write(w, ".text")
	write(w, ".globl main")
	write(w, "")
	write(w, "main:")
	write(w, "  subq $56, %rsp", "Shadow space + alignment")
	c.emitWindowsArgvPreamble(w)
	c.emitProgram(w)
	write(w, "  xorq %rcx, %rcx", "Exit code 0")
	write(w, "  call ExitProcess", "Exit")
	write(w, "")
	c.emitAtoi(w)
	c.emitPrintln(w)
	write(w, ".bss")
	if c.usesPrint {
		write(w, "buffer:")
		write(w, ".space 32", "32-byte buffer for number")
		write(w, "written:")
		write(w, ".space 8", "Bytes written")
	}
	if c.usesArg {
		write(w, "argv_ptr:")
		write(w, ".space 8", "LPWSTR* argv")
		write(w, "argc_storage:")
		write(w, ".space 8", "int argc")
	}
	c.emitBssVars(w)
	return nil
}

func (c *Compiler) emitCall(w io.Writer, e *CallExpr) {
	switch e.Name {
	case "println":
		if len(e.Args) != 1 {
			panic(fmt.Sprintf("println takes 1 arg, got %d", len(e.Args)))
		}
		c.emitExpr(w, e.Args[0])
		if runtime.GOOS == "windows" {
			write(w, "  popq %rcx", "Arg into RCX (aligns RSP)")
		} else {
			write(w, "  popq %rdi", "Arg into RDI (aligns RSP)")
		}
		write(w, "  call __println_int", "Print value, returns it in RAX")
		write(w, "  pushq %rax", "Push return value as expr result")
	default:
		panic(fmt.Sprintf("unknown function: %s", e.Name))
	}
}

func (c *Compiler) emitPrintln(w io.Writer) {
	if !c.usesPrint {
		return
	}
	if runtime.GOOS == "windows" {
		c.emitPrintlnWindows(w)
		return
	}
	write(w, "__println_int:")
	write(w, "  movq %rdi, %r10", "Save input value (preserved across syscall)")
	write(w, "  movq %rdi, %rax", "Value for division")
	write(w, "  movq $10, %r8", "Base 10")
	write(w, "  leaq buffer+31(%rip), %r9", "End of buffer")
	write(w, "  movb $10, (%r9)", "Newline")
	write(w, "  decq %r9", "Move back")
	write(w, ".Lpli_conv:")
	write(w, "  xorq %rdx, %rdx", "Clear RDX for division")
	write(w, "  divq %r8", "RAX / 10")
	write(w, "  addb $48, %dl", "Digit to ASCII")
	write(w, "  movb %dl, (%r9)", "Store digit")
	write(w, "  decq %r9", "Move back")
	write(w, "  testq %rax, %rax", "More digits?")
	write(w, "  jnz .Lpli_conv", "Continue")
	write(w, "  incq %r9", "First digit")
	write(w, "  movq $1, %rax", "Syscall: write")
	write(w, "  movq $1, %rdi", "Stdout")
	write(w, "  movq %r9, %rsi", "Buffer")
	write(w, "  leaq buffer+32(%rip), %rdx")
	write(w, "  subq %r9, %rdx", "Length")
	write(w, "  syscall", "Call kernel")
	write(w, "  movq %r10, %rax", "Return original value")
	write(w, "  ret")
	write(w, "")
}

func (c *Compiler) emitPrintlnWindows(w io.Writer) {
	write(w, "__println_int:")
	write(w, "  subq $56, %rsp", "Shadow + 5th arg + spill, keep RSP aligned")
	write(w, "  movq %rcx, 40(%rsp)", "Spill input value")
	write(w, "  movq %rcx, %rax", "Value for division")
	write(w, "  movq $10, %r8", "Base 10")
	write(w, "  leaq buffer+31(%rip), %r9", "End of buffer")
	write(w, "  movb $10, (%r9)", "Newline")
	write(w, "  decq %r9", "Move back")
	write(w, ".Lpli_conv:")
	write(w, "  xorq %rdx, %rdx", "Clear RDX for division")
	write(w, "  divq %r8", "RAX / 10")
	write(w, "  addb $48, %dl", "Digit to ASCII")
	write(w, "  movb %dl, (%r9)", "Store digit")
	write(w, "  decq %r9", "Move back")
	write(w, "  testq %rax, %rax", "More digits?")
	write(w, "  jnz .Lpli_conv", "Continue")
	write(w, "  incq %r9", "First digit")
	write(w, "  movq %r9, 48(%rsp)", "Spill buffer ptr across GetStdHandle")
	write(w, "  movq $-11, %rcx", "STD_OUTPUT_HANDLE")
	write(w, "  call GetStdHandle", "Get stdout handle")
	write(w, "  movq %rax, %rcx", "Handle")
	write(w, "  movq 48(%rsp), %rdx", "Buffer ptr")
	write(w, "  leaq buffer+32(%rip), %r8")
	write(w, "  subq %rdx, %r8", "Length")
	write(w, "  leaq written(%rip), %r9", "Bytes written")
	write(w, "  movq $0, 32(%rsp)", "lpOverlapped = NULL")
	write(w, "  call WriteFile", "Write to stdout")
	write(w, "  movq 40(%rsp), %rax", "Return original value")
	write(w, "  addq $56, %rsp")
	write(w, "  ret")
	write(w, "")
}

func (c *Compiler) emitWindowsArgvPreamble(w io.Writer) {
	if !c.usesArg {
		return
	}
	write(w, "  call GetCommandLineW", "RAX = LPWSTR")
	write(w, "  movq %rax, %rcx", "arg1: lpCmdLine")
	write(w, "  leaq argc_storage(%rip), %rdx", "arg2: pNumArgs")
	write(w, "  call CommandLineToArgvW", "RAX = LPWSTR*")
	write(w, "  movq %rax, argv_ptr(%rip)", "Save argv pointer")
}

func (c *Compiler) emitExpr(w io.Writer, e Expr) {
	switch e := e.(type) {
	case *NumLit:
		write(w, fmt.Sprintf("  movq $%d, %%rax", e.Value), "Load number")
		write(w, "  pushq %rax", "Push to stack")
	case *ArgRef:
		if runtime.GOOS == "windows" {
			offset := 8 * e.Index
			write(w, "  movq argv_ptr(%rip), %rax", "Load argv base")
			write(w, fmt.Sprintf("  movq %d(%%rax), %%rdi", offset), fmt.Sprintf("Load argv[%d]", e.Index))
		} else {
			offset := 8 * (e.Index + 1)
			write(w, fmt.Sprintf("  movq %d(%%rbp), %%rdi", offset), fmt.Sprintf("Load argv[%d]", e.Index))
		}
		write(w, "  call __atoi", "Parse as integer")
		write(w, "  pushq %rax", "Push to stack")
	case *VarRef:
		c.vars[e.Name] = true
		write(w, fmt.Sprintf("  movq var_%s(%%rip), %%rax", e.Name), "Load variable")
		write(w, "  pushq %rax", "Push to stack")
	case *CallExpr:
		c.emitCall(w, e)
	case *BinOp:
		c.emitExpr(w, e.L)
		c.emitExpr(w, e.R)
		write(w, "  popq %rax", "Get second operand")
		write(w, "  popq %rbx", "Get first operand")
		switch e.Op {
		case TOK_PLUS:
			write(w, "  addq %rbx, %rax", "Add them")
		case TOK_MINUS:
			write(w, "  subq %rax, %rbx", "Subtract")
			write(w, "  movq %rbx, %rax", "Result in RAX")
		case TOK_MUL:
			write(w, "  imulq %rbx, %rax", "Multiply")
		case TOK_DIV:
			write(w, "  movq %rax, %rcx", "Save divisor")
			write(w, "  movq %rbx, %rax", "Move dividend to RAX")
			write(w, "  xorq %rdx, %rdx", "Clear RDX for division")
			write(w, "  idivq %rcx", "Divide RDX:RAX by divisor")
		case TOK_MOD:
			write(w, "  movq %rax, %rcx", "Save divisor")
			write(w, "  movq %rbx, %rax", "Move dividend to RAX")
			write(w, "  cqto", "Sign-extend RAX into RDX")
			write(w, "  idivq %rcx", "RDX = remainder")
			write(w, "  movq %rdx, %rax", "Result = remainder")
		}
		write(w, "  pushq %rax", "Save result")
	default:
		panic("unknown expr")
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

