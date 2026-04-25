package mame

import (
	"fmt"
	"io"
	"runtime"
	"strings"
)

const commentColumn = 32

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
	case *StrLit:
		panic("string literal can only appear as a println argument")
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

func (c *Compiler) emitCall(w io.Writer, e *CallExpr) {
	switch e.Name {
	case "println":
		if len(e.Args) != 1 {
			panic(fmt.Sprintf("println takes 1 arg, got %d", len(e.Args)))
		}
		if str, ok := e.Args[0].(*StrLit); ok {
			idx := len(c.strLits)
			c.strLits = append(c.strLits, str.Value)
			c.usesPrintStr = true
			label := fmt.Sprintf(".Lstr_%d", idx)
			length := len(str.Value) + 1
			if runtime.GOOS == "windows" {
				write(w, fmt.Sprintf("  leaq %s(%%rip), %%rcx", label), "Arg: string ptr")
				write(w, fmt.Sprintf("  movq $%d, %%rdx", length), "Arg: length")
			} else {
				write(w, fmt.Sprintf("  leaq %s(%%rip), %%rdi", label), "Arg: string ptr")
				write(w, fmt.Sprintf("  movq $%d, %%rsi", length), "Arg: length")
			}
			write(w, "  call __println_str", "Print string")
			write(w, "  pushq $0", "Push dummy expr result")
			return
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
	c.emitPrintlnStr(w)
	c.emitData(w)
	write(w, ".bss")
	if c.usesPrint {
		write(w, "buffer:")
		write(w, ".space 32", "32-byte buffer for number")
	}
	c.emitBssVars(w)
	return nil
}

func (c *Compiler) compileWindows(w io.Writer) error {
	write(w, ".text")
	write(w, ".globl main")
	write(w, "")
	write(w, "main:")
	write(w, "  subq $56, %rsp", "Shadow space + alignment")
	if c.usesPrintStr {
		write(w, "  movq $65001, %rcx", "CP_UTF8")
		write(w, "  call SetConsoleOutputCP", "Switch console to UTF-8")
	}
	c.emitWindowsArgvPreamble(w)
	c.emitProgram(w)
	write(w, "  xorq %rcx, %rcx", "Exit code 0")
	write(w, "  call ExitProcess", "Exit")
	write(w, "")
	c.emitAtoi(w)
	c.emitPrintln(w)
	c.emitPrintlnStr(w)
	c.emitData(w)
	write(w, ".bss")
	if c.usesPrint {
		write(w, "buffer:")
		write(w, ".space 32", "32-byte buffer for number")
	}
	if c.usesPrint || c.usesPrintStr {
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
	write(w, "  movzbq (%rdi), %rdx", "Load first byte")
	write(w, "  cmpb $45, %dl", "'-'")
	write(w, "  jne __atoi_loop", "Not '-': skip")
	write(w, "  movq $1, %rcx", "negative")
	write(w, "  incq %rdi", "Skip '-'")
	write(w, "__atoi_loop:")
	write(w, "  movzbq (%rdi), %rdx", "Load byte")
	write(w, "  testb %dl, %dl", "Null terminator?")
	write(w, "  jz __atoi_done", "Done")
	write(w, "  subq $48, %rdx", "'0'")
	write(w, "  imulq $10, %rax", "result *= 10")
	write(w, "  addq %rdx, %rax", "result += digit")
	write(w, "  incq %rdi", "Advance")
	write(w, "  jmp __atoi_loop", "Continue")
	write(w, "__atoi_done:")
	write(w, "  testq %rcx, %rcx", "Negative?")
	write(w, "  jz __atoi_ret", "Skip negation")
	write(w, "  negq %rax", "Apply sign")
	write(w, "__atoi_ret:")
	write(w, "  ret", "Return")
	write(w, "")
}

func (c *Compiler) emitAtoiWide(w io.Writer) {
	write(w, "__atoi:")
	write(w, "  xorq %rax, %rax", "result = 0")
	write(w, "  xorq %rcx, %rcx", "sign flag = 0")
	write(w, "  movzwl (%rdi), %edx", "Load first wchar")
	write(w, "  cmpw $45, %dx", "L'-'")
	write(w, "  jne __atoi_loop", "Not '-': skip")
	write(w, "  movq $1, %rcx", "negative")
	write(w, "  addq $2, %rdi", "Skip '-'")
	write(w, "__atoi_loop:")
	write(w, "  movzwl (%rdi), %edx", "Load wchar")
	write(w, "  testw %dx, %dx", "Null terminator?")
	write(w, "  jz __atoi_done", "Done")
	write(w, "  subq $48, %rdx", "L'0'")
	write(w, "  imulq $10, %rax", "result *= 10")
	write(w, "  addq %rdx, %rax", "result += digit")
	write(w, "  addq $2, %rdi", "Advance")
	write(w, "  jmp __atoi_loop", "Continue")
	write(w, "__atoi_done:")
	write(w, "  testq %rcx, %rcx", "Negative?")
	write(w, "  jz __atoi_ret", "Skip negation")
	write(w, "  negq %rax", "Apply sign")
	write(w, "__atoi_ret:")
	write(w, "  ret", "Return")
	write(w, "")
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
	write(w, "  testq %rax, %rax", "Check sign")
	write(w, "  jns .Lpli_abs", "Non-negative: skip negation")
	write(w, "  negq %rax", "Absolute value for unsigned div")
	write(w, ".Lpli_abs:")
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
	write(w, "  testq %r10, %r10", "Original negative?")
	write(w, "  jns .Lpli_pos", "Non-negative: skip sign")
	write(w, "  movb $45, (%r9)", "'-' sign")
	write(w, "  decq %r9", "Move back")
	write(w, ".Lpli_pos:")
	write(w, "  incq %r9", "First char")
	write(w, "  movq $1, %rax", "Syscall: write")
	write(w, "  movq $1, %rdi", "Stdout")
	write(w, "  movq %r9, %rsi", "Buffer")
	write(w, "  leaq buffer+32(%rip), %rdx", "Past end of buffer")
	write(w, "  subq %r9, %rdx", "Length")
	write(w, "  syscall", "Call kernel")
	write(w, "  movq %r10, %rax", "Return original value")
	write(w, "  ret", "Return")
	write(w, "")
}

func (c *Compiler) emitPrintlnWindows(w io.Writer) {
	write(w, "__println_int:")
	write(w, "  subq $56, %rsp", "Shadow + 5th arg + spill, keep RSP aligned")
	write(w, "  movq %rcx, 40(%rsp)", "Spill input value")
	write(w, "  movq %rcx, %rax", "Value for division")
	write(w, "  testq %rax, %rax", "Check sign")
	write(w, "  jns .Lpli_abs", "Non-negative: skip negation")
	write(w, "  negq %rax", "Absolute value for unsigned div")
	write(w, ".Lpli_abs:")
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
	write(w, "  movq 40(%rsp), %rax", "Reload original")
	write(w, "  testq %rax, %rax", "Original negative?")
	write(w, "  jns .Lpli_pos", "Non-negative: skip sign")
	write(w, "  movb $45, (%r9)", "'-' sign")
	write(w, "  decq %r9", "Move back")
	write(w, ".Lpli_pos:")
	write(w, "  incq %r9", "First char")
	write(w, "  movq %r9, 48(%rsp)", "Spill buffer ptr across GetStdHandle")
	write(w, "  movq $-11, %rcx", "STD_OUTPUT_HANDLE")
	write(w, "  call GetStdHandle", "Get stdout handle")
	write(w, "  movq %rax, %rcx", "Handle")
	write(w, "  movq 48(%rsp), %rdx", "Buffer ptr")
	write(w, "  leaq buffer+32(%rip), %r8", "Past end of buffer")
	write(w, "  subq %rdx, %r8", "Length")
	write(w, "  leaq written(%rip), %r9", "Bytes written")
	write(w, "  movq $0, 32(%rsp)", "lpOverlapped = NULL")
	write(w, "  call WriteFile", "Write to stdout")
	write(w, "  movq 40(%rsp), %rax", "Return original value")
	write(w, "  addq $56, %rsp", "Restore stack")
	write(w, "  ret", "Return")
	write(w, "")
}

func (c *Compiler) emitPrintlnStr(w io.Writer) {
	if !c.usesPrintStr {
		return
	}
	if runtime.GOOS == "windows" {
		c.emitPrintlnStrWindows(w)
		return
	}
	write(w, "__println_str:")
	write(w, "  movq %rsi, %rdx", "Length to RDX (syscall arg 3)")
	write(w, "  movq %rdi, %rsi", "Buffer to RSI (syscall arg 2)")
	write(w, "  movq $1, %rdi", "Stdout")
	write(w, "  movq $1, %rax", "Syscall: write")
	write(w, "  syscall", "Call kernel")
	write(w, "  ret", "Return")
	write(w, "")
}

func (c *Compiler) emitPrintlnStrWindows(w io.Writer) {
	write(w, "__println_str:")
	write(w, "  subq $56, %rsp", "Shadow + 5th arg + spill")
	write(w, "  movq %rcx, 40(%rsp)", "Spill string ptr")
	write(w, "  movq %rdx, 48(%rsp)", "Spill length")
	write(w, "  movq $-11, %rcx", "STD_OUTPUT_HANDLE")
	write(w, "  call GetStdHandle", "Get stdout handle")
	write(w, "  movq %rax, %rcx", "Handle")
	write(w, "  movq 40(%rsp), %rdx", "Buffer ptr")
	write(w, "  movq 48(%rsp), %r8", "Length")
	write(w, "  leaq written(%rip), %r9", "Bytes written")
	write(w, "  movq $0, 32(%rsp)", "lpOverlapped = NULL")
	write(w, "  call WriteFile", "Write to stdout")
	write(w, "  addq $56, %rsp", "Restore stack")
	write(w, "  ret", "Return")
	write(w, "")
}

func (c *Compiler) emitData(w io.Writer) {
	if len(c.strLits) == 0 {
		return
	}
	write(w, ".data")
	for i, s := range c.strLits {
		write(w, fmt.Sprintf(".Lstr_%d:", i))
		write(w, fmt.Sprintf(".ascii %q", s+"\n"))
	}
}
