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

// callAligned emits a `call` instruction, padding RSP if the current expression
// stack depth would leave it 8-misaligned. Each tagged value slot is 24 bytes,
// so depth parity flips alignment.
func (c *Compiler) callAligned(w io.Writer, target, comment string) {
	if c.depth%2 != 0 {
		write(w, "  subq $8, %rsp", "Align RSP for call")
		write(w, "  call "+target, comment)
		write(w, "  addq $8, %rsp", "Restore align")
		return
	}
	write(w, "  call "+target, comment)
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
		write(w, "  popq %rax", "Pop payload")
		write(w, "  popq %rcx", "Pop len")
		write(w, "  popq %rbx", "Pop tag")
		c.depth--
		write(w, fmt.Sprintf("  movq %%rbx, var_%s(%%rip)", s.Name), "Store tag")
		write(w, fmt.Sprintf("  movq %%rcx, var_%s+8(%%rip)", s.Name), "Store len")
		write(w, fmt.Sprintf("  movq %%rax, var_%s+16(%%rip)", s.Name), "Store payload")
	case *ExprStmt:
		c.emitExpr(w, s.X)
		write(w, "  addq $24, %rsp", "Discard stmt result")
		c.depth--
	case *IfStmt:
		c.emitIf(w, s)
	case *WhileStmt:
		c.emitWhile(w, s)
	}
}

func (c *Compiler) emitWhile(w io.Writer, s *WhileStmt) {
	id := c.labelCnt
	c.labelCnt++
	write(w, fmt.Sprintf(".Lwhile_top_%d:", id))
	c.emitExpr(w, s.Cond)
	write(w, "  popq %rax", "Pop condition payload")
	write(w, "  addq $16, %rsp", "Discard len + tag")
	c.depth--
	write(w, "  testq %rax, %rax", "Test condition")
	write(w, fmt.Sprintf("  jz .Lwhile_end_%d", id), "False -> exit loop")
	for _, t := range s.Body {
		c.emitStmt(w, t)
	}
	write(w, fmt.Sprintf("  jmp .Lwhile_top_%d", id), "Loop back")
	write(w, fmt.Sprintf(".Lwhile_end_%d:", id))
}

func (c *Compiler) emitIf(w io.Writer, s *IfStmt) {
	id := c.labelCnt
	c.labelCnt++
	c.emitExpr(w, s.Cond)
	write(w, "  popq %rax", "Pop condition payload")
	write(w, "  addq $16, %rsp", "Discard len + tag")
	c.depth--
	write(w, "  testq %rax, %rax", "Test condition")
	if len(s.Else) > 0 {
		write(w, fmt.Sprintf("  jz .Lif_else_%d", id), "False -> else")
		for _, t := range s.Then {
			c.emitStmt(w, t)
		}
		write(w, fmt.Sprintf("  jmp .Lif_end_%d", id), "End of if")
		write(w, fmt.Sprintf(".Lif_else_%d:", id))
		for _, t := range s.Else {
			c.emitStmt(w, t)
		}
		write(w, fmt.Sprintf(".Lif_end_%d:", id))
	} else {
		write(w, fmt.Sprintf("  jz .Lif_end_%d", id), "False -> skip then")
		for _, t := range s.Then {
			c.emitStmt(w, t)
		}
		write(w, fmt.Sprintf(".Lif_end_%d:", id))
	}
}

func (c *Compiler) emitExpr(w io.Writer, e Expr) {
	switch e := e.(type) {
	case *NumLit:
		write(w, "  pushq $0", "Tag = INT")
		write(w, "  pushq $0", "Len = 0 (unused)")
		write(w, fmt.Sprintf("  movq $%d, %%rax", e.Value), "Load number")
		write(w, "  pushq %rax", "Push value")
		c.depth++
	case *ArgRef:
		c.emitExpr(w, e.Index)
		write(w, "  popq %rax", "Pop arg index value")
		write(w, "  addq $16, %rsp", "Discard len + tag")
		c.depth--
		if runtime.GOOS == "windows" {
			write(w, "  movq argv_ptr(%rip), %rcx", "argv base")
			write(w, "  movq (%rcx,%rax,8), %rdi", "argv[N]")
		} else {
			write(w, "  incq %rax", "N+1 (skip argc slot)")
			write(w, "  movq (%rbp,%rax,8), %rdi", "argv[N]")
		}
		c.callAligned(w, "__atoi", "Parse as integer")
		write(w, "  pushq $0", "Tag = INT")
		write(w, "  pushq $0", "Len = 0")
		write(w, "  pushq %rax", "Push value")
		c.depth++
	case *NargExpr:
		if runtime.GOOS == "windows" {
			write(w, "  movq argc_storage(%rip), %rax", "argc")
		} else {
			write(w, "  movq (%rbp), %rax", "argc")
		}
		write(w, "  decq %rax", "Exclude program name")
		write(w, "  pushq $0", "Tag = INT")
		write(w, "  pushq $0", "Len = 0")
		write(w, "  pushq %rax", "Push narg")
		c.depth++
	case *VarRef:
		c.vars[e.Name] = true
		write(w, fmt.Sprintf("  movq var_%s(%%rip), %%rbx", e.Name), "Load tag")
		write(w, fmt.Sprintf("  movq var_%s+8(%%rip), %%rcx", e.Name), "Load len")
		write(w, fmt.Sprintf("  movq var_%s+16(%%rip), %%rax", e.Name), "Load payload")
		write(w, "  pushq %rbx", "Push tag")
		write(w, "  pushq %rcx", "Push len")
		write(w, "  pushq %rax", "Push payload")
		c.depth++
	case *CallExpr:
		c.emitCall(w, e)
	case *StrLit:
		idx := len(c.strLits)
		c.strLits = append(c.strLits, e.Value)
		c.usesPrintStr = true
		write(w, "  pushq $1", "Tag = STR")
		write(w, fmt.Sprintf("  pushq $%d", len(e.Value)), "Length")
		write(w, fmt.Sprintf("  leaq .Lstr_%d(%%rip), %%rax", idx), "String ptr")
		write(w, "  pushq %rax", "Push ptr")
		c.depth++
	case *BinOp:
		c.emitExpr(w, e.L)
		c.emitExpr(w, e.R)
		write(w, "  popq %rax", "Get R payload")
		write(w, "  addq $16, %rsp", "Discard R len + tag")
		c.depth--
		write(w, "  popq %rbx", "Get L payload")
		write(w, "  addq $16, %rsp", "Discard L len + tag")
		c.depth--
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
		case TOK_EQ:
			c.emitCmpSet(w, "sete", "L == R")
		case TOK_NE:
			c.emitCmpSet(w, "setne", "L != R")
		case TOK_LT:
			c.emitCmpSet(w, "setl", "L < R")
		case TOK_LE:
			c.emitCmpSet(w, "setle", "L <= R")
		case TOK_GT:
			c.emitCmpSet(w, "setg", "L > R")
		case TOK_GE:
			c.emitCmpSet(w, "setge", "L >= R")
		}
		write(w, "  pushq $0", "Tag = INT")
		write(w, "  pushq $0", "Len = 0")
		write(w, "  pushq %rax", "Save result value")
		c.depth++
	default:
		panic("unknown expr")
	}
}

func (c *Compiler) emitCmpSet(w io.Writer, setcc, comment string) {
	write(w, "  cmpq %rax, %rbx", "Compare L vs R")
	write(w, "  "+setcc+" %al", comment)
	write(w, "  movzbq %al, %rax", "Zero-extend to 64-bit")
}

func (c *Compiler) emitCall(w io.Writer, e *CallExpr) {
	switch e.Name {
	case "print", "println":
		if len(e.Args) != 1 {
			panic(fmt.Sprintf("%s takes 1 arg, got %d", e.Name, len(e.Args)))
		}
		if str, ok := e.Args[0].(*StrLit); ok {
			idx := len(c.strLits)
			c.strLits = append(c.strLits, str.Value)
			c.usesPrintStr = true
			label := fmt.Sprintf(".Lstr_%d", idx)
			c.emitPrintStrCall(w, label, len(str.Value))
			if e.Name == "println" {
				c.emitPrintNl(w)
			}
			write(w, "  pushq $0", "Tag = INT")
			write(w, "  pushq $0", "Len = 0")
			write(w, "  pushq $0", "Dummy result")
			c.depth++
			return
		}
		c.emitDynamicPrint(w, e.Args[0], e.Name == "println")
	default:
		panic(fmt.Sprintf("unknown function: %s", e.Name))
	}
}

// emitDynamicPrint evaluates the single argument, pops the tagged value, then
// dispatches at runtime: STR routes to __print_str(ptr,len), INT routes to
// __print_int(value). Pushes a tagged INT result (the int value, or 0 if STR).
func (c *Compiler) emitDynamicPrint(w io.Writer, arg Expr, isPrintln bool) {
	c.usesPrint = true
	c.usesPrintStr = true
	c.emitExpr(w, arg)
	write(w, "  popq %rax", "Pop payload (value or ptr)")
	write(w, "  popq %rsi", "Pop len")
	write(w, "  popq %rbx", "Pop tag")
	c.depth--
	id := c.labelCnt
	c.labelCnt++
	write(w, "  testq %rbx, %rbx", "Tag == INT?")
	write(w, fmt.Sprintf("  jz .Lprint_int_%d", id), "INT path")
	if runtime.GOOS == "windows" {
		write(w, "  movq %rax, %rcx", "ptr -> arg1")
		write(w, "  movq %rsi, %rdx", "len -> arg2")
	} else {
		write(w, "  movq %rax, %rdi", "ptr -> arg1")
	}
	c.callAligned(w, "__print_str", "Print string")
	write(w, "  xorq %rax, %rax", "STR path: result = 0")
	write(w, fmt.Sprintf("  jmp .Lprint_done_%d", id), "Done")
	write(w, fmt.Sprintf(".Lprint_int_%d:", id))
	if runtime.GOOS == "windows" {
		write(w, "  movq %rax, %rcx", "value -> arg1")
	} else {
		write(w, "  movq %rax, %rdi", "value -> arg1")
	}
	c.callAligned(w, "__print_int", "Print int (returns value in RAX)")
	write(w, fmt.Sprintf(".Lprint_done_%d:", id))
	write(w, "  pushq $0", "Tag = INT")
	write(w, "  pushq $0", "Len = 0")
	write(w, "  pushq %rax", "Push result")
	c.depth++
	if isPrintln {
		c.emitPrintNl(w)
	}
}

func (c *Compiler) emitPrintStrCall(w io.Writer, label string, length int) {
	if runtime.GOOS == "windows" {
		write(w, fmt.Sprintf("  leaq %s(%%rip), %%rcx", label), "String ptr")
		write(w, fmt.Sprintf("  movq $%d, %%rdx", length), "Length")
	} else {
		write(w, fmt.Sprintf("  leaq %s(%%rip), %%rdi", label), "String ptr")
		write(w, fmt.Sprintf("  movq $%d, %%rsi", length), "Length")
	}
	c.callAligned(w, "__print_str", "Print string")
}

func (c *Compiler) emitPrintNl(w io.Writer) {
	if runtime.GOOS == "windows" {
		write(w, "  leaq .Lnl(%rip), %rcx", "Newline ptr")
		write(w, "  movq $1, %rdx", "Length 1")
	} else {
		write(w, "  leaq .Lnl(%rip), %rdi", "Newline ptr")
		write(w, "  movq $1, %rsi", "Length 1")
	}
	c.callAligned(w, "__print_str", "Print newline")
}

func (c *Compiler) emitBssVars(w io.Writer) {
	for name := range c.vars {
		write(w, fmt.Sprintf("var_%s:", name))
		write(w, ".space 24", "Variable storage (tag + len + payload)")
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
	if !c.usesAtoi {
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
	write(w, "__print_int:")
	write(w, "  movq %rdi, %r10", "Save input value (preserved across syscall)")
	write(w, "  movq %rdi, %rax", "Value for division")
	write(w, "  testq %rax, %rax", "Check sign")
	write(w, "  jns .Lpi_abs", "Non-negative: skip negation")
	write(w, "  negq %rax", "Absolute value for unsigned div")
	write(w, ".Lpi_abs:")
	write(w, "  movq $10, %r8", "Base 10")
	write(w, "  leaq buffer+31(%rip), %r9", "Last byte of buffer")
	write(w, ".Lpi_conv:")
	write(w, "  xorq %rdx, %rdx", "Clear RDX for division")
	write(w, "  divq %r8", "RAX / 10")
	write(w, "  addb $48, %dl", "Digit to ASCII")
	write(w, "  movb %dl, (%r9)", "Store digit")
	write(w, "  decq %r9", "Move back")
	write(w, "  testq %rax, %rax", "More digits?")
	write(w, "  jnz .Lpi_conv", "Continue")
	write(w, "  testq %r10, %r10", "Original negative?")
	write(w, "  jns .Lpi_pos", "Non-negative: skip sign")
	write(w, "  movb $45, (%r9)", "'-' sign")
	write(w, "  decq %r9", "Move back")
	write(w, ".Lpi_pos:")
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
	write(w, "__print_int:")
	write(w, "  subq $56, %rsp", "Shadow + 5th arg + spill, keep RSP aligned")
	write(w, "  movq %rcx, 40(%rsp)", "Spill input value")
	write(w, "  movq %rcx, %rax", "Value for division")
	write(w, "  testq %rax, %rax", "Check sign")
	write(w, "  jns .Lpi_abs", "Non-negative: skip negation")
	write(w, "  negq %rax", "Absolute value for unsigned div")
	write(w, ".Lpi_abs:")
	write(w, "  movq $10, %r8", "Base 10")
	write(w, "  leaq buffer+31(%rip), %r9", "Last byte of buffer")
	write(w, ".Lpi_conv:")
	write(w, "  xorq %rdx, %rdx", "Clear RDX for division")
	write(w, "  divq %r8", "RAX / 10")
	write(w, "  addb $48, %dl", "Digit to ASCII")
	write(w, "  movb %dl, (%r9)", "Store digit")
	write(w, "  decq %r9", "Move back")
	write(w, "  testq %rax, %rax", "More digits?")
	write(w, "  jnz .Lpi_conv", "Continue")
	write(w, "  movq 40(%rsp), %rax", "Reload original")
	write(w, "  testq %rax, %rax", "Original negative?")
	write(w, "  jns .Lpi_pos", "Non-negative: skip sign")
	write(w, "  movb $45, (%r9)", "'-' sign")
	write(w, "  decq %r9", "Move back")
	write(w, ".Lpi_pos:")
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
	write(w, "__print_str:")
	write(w, "  movq %rsi, %rdx", "Length to RDX (syscall arg 3)")
	write(w, "  movq %rdi, %rsi", "Buffer to RSI (syscall arg 2)")
	write(w, "  movq $1, %rdi", "Stdout")
	write(w, "  movq $1, %rax", "Syscall: write")
	write(w, "  syscall", "Call kernel")
	write(w, "  ret", "Return")
	write(w, "")
}

func (c *Compiler) emitPrintlnStrWindows(w io.Writer) {
	write(w, "__print_str:")
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
	if !c.usesPrintStr {
		return
	}
	write(w, ".data")
	for i, s := range c.strLits {
		write(w, fmt.Sprintf(".Lstr_%d:", i))
		write(w, fmt.Sprintf(".ascii %q", s))
	}
	write(w, ".Lnl:")
	write(w, `.ascii "\n"`)
}
