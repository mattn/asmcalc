package mame

import (
	"fmt"
	"io"
	"runtime"
	"strings"
)

const (
	commentColumn = 32
	heapSize      = 1 << 20 // 1 MiB bump heap
)

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

// Slot layout (16 bytes, every value uniform):
//   +0  tag       (0 = INT, 1 = STR)
//   +8  payload   (INT: the value; STR: ptr to heap object)
//
// Heap object (for both static literals and dynamic strings):
//   +0  refcount  (placeholder, always 0 for now; reserved for future RC/GC)
//   +8  len       (byte length)
//   +16 bytes...  (UTF-8)
//
// The 16-byte slot keeps the stack naturally 16-aligned, so internal calls
// don't need ad-hoc subq/addq dance.

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
		write(w, "  popq %rbx", "Pop tag")
		write(w, fmt.Sprintf("  movq %%rbx, var_%s(%%rip)", s.Name), "Store tag")
		write(w, fmt.Sprintf("  movq %%rax, var_%s+8(%%rip)", s.Name), "Store payload")
	case *ExprStmt:
		c.emitExpr(w, s.X)
		write(w, "  addq $16, %rsp", "Discard stmt result")
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
	write(w, "  addq $8, %rsp", "Discard tag")
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
	write(w, "  addq $8, %rsp", "Discard tag")
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
		write(w, fmt.Sprintf("  movq $%d, %%rax", e.Value), "Load number")
		write(w, "  pushq %rax", "Push value")
	case *ArgRef:
		c.emitExpr(w, e.Index)
		write(w, "  popq %rdi", "idx -> arg1")
		write(w, "  addq $8, %rsp", "Discard tag")
		write(w, "  call __arg_get", "argv[idx] -> heap obj ptr in RAX")
		write(w, "  pushq $1", "Tag = STR")
		write(w, "  pushq %rax", "Push heap obj ptr")
	case *NargExpr:
		if runtime.GOOS == "windows" {
			write(w, "  movq argc_storage(%rip), %rax", "argc")
		} else {
			write(w, "  movq (%rbp), %rax", "argc")
		}
		write(w, "  decq %rax", "Exclude program name")
		write(w, "  pushq $0", "Tag = INT")
		write(w, "  pushq %rax", "Push narg")
	case *VarRef:
		c.vars[e.Name] = true
		write(w, fmt.Sprintf("  movq var_%s(%%rip), %%rbx", e.Name), "Load tag")
		write(w, fmt.Sprintf("  movq var_%s+8(%%rip), %%rax", e.Name), "Load payload")
		write(w, "  pushq %rbx", "Push tag")
		write(w, "  pushq %rax", "Push payload")
	case *CallExpr:
		c.emitCall(w, e)
	case *StrLit:
		idx := len(c.strLits)
		c.strLits = append(c.strLits, e.Value)
		write(w, "  pushq $1", "Tag = STR")
		write(w, fmt.Sprintf("  leaq .Lstr_%d(%%rip), %%rax", idx), "Heap obj ptr")
		write(w, "  pushq %rax", "Push payload")
	case *BinOp:
		c.emitExpr(w, e.L)
		c.emitExpr(w, e.R)
		write(w, "  popq %rax", "Get R payload")
		write(w, "  addq $8, %rsp", "Discard R tag")
		write(w, "  popq %rbx", "Get L payload")
		write(w, "  addq $8, %rsp", "Discard L tag")
		switch e.Op {
		case TOK_PLUS:
			write(w, "  addq %rbx, %rax", "Add them")
		case TOK_MINUS:
			write(w, "  subq %rax, %rbx", "Subtract")
			write(w, "  movq %rbx, %rax", "Result in RAX")
		case TOK_MUL:
			write(w, "  imulq %rbx, %rax", "Multiply")
		case TOK_DIV:
			c.usesPanic = true
			write(w, "  movq %rax, %rcx", "Save divisor")
			write(w, "  testq %rcx, %rcx", "Divisor == 0?")
			write(w, "  jz __div_zero", "Panic on zero")
			write(w, "  movq %rbx, %rax", "Move dividend to RAX")
			write(w, "  xorq %rdx, %rdx", "Clear RDX for division")
			write(w, "  idivq %rcx", "Divide RDX:RAX by divisor")
		case TOK_MOD:
			c.usesPanic = true
			write(w, "  movq %rax, %rcx", "Save divisor")
			write(w, "  testq %rcx, %rcx", "Divisor == 0?")
			write(w, "  jz __div_zero", "Panic on zero")
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
		write(w, "  pushq %rax", "Save result value")
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
	case "int":
		if len(e.Args) != 1 {
			panic(fmt.Sprintf("int takes 1 arg, got %d", len(e.Args)))
		}
		c.emitExpr(w, e.Args[0])
		write(w, "  popq %rax", "Pop heap obj ptr")
		write(w, "  addq $8, %rsp", "Discard tag")
		write(w, "  movq 8(%rax), %rsi", "len from header")
		write(w, "  leaq 16(%rax), %rdi", "bytes ptr from header")
		write(w, "  call __str_to_int", "Parse as integer")
		write(w, "  pushq $0", "Tag = INT")
		write(w, "  pushq %rax", "Push value")
		return
	case "str":
		if len(e.Args) != 1 {
			panic(fmt.Sprintf("str takes 1 arg, got %d", len(e.Args)))
		}
		c.emitExpr(w, e.Args[0])
		write(w, "  popq %rdi", "value -> arg1")
		write(w, "  addq $8, %rsp", "Discard tag")
		write(w, "  call __int_to_str", "Convert int -> heap STR obj in RAX")
		write(w, "  pushq $1", "Tag = STR")
		write(w, "  pushq %rax", "Push heap obj ptr")
		return
	case "len":
		if len(e.Args) != 1 {
			panic(fmt.Sprintf("len takes 1 arg, got %d", len(e.Args)))
		}
		c.emitExpr(w, e.Args[0])
		write(w, "  popq %rax", "Pop heap obj ptr")
		write(w, "  addq $8, %rsp", "Discard tag")
		write(w, "  movq 8(%rax), %rax", "len from header")
		write(w, "  pushq $0", "Tag = INT")
		write(w, "  pushq %rax", "Push len")
		return
	case "panic":
		if len(e.Args) != 1 {
			panic(fmt.Sprintf("panic takes 1 arg, got %d", len(e.Args)))
		}
		c.usesPanic = true
		c.emitExpr(w, e.Args[0])
		write(w, "  popq %rax", "Pop heap obj ptr")
		write(w, "  addq $8, %rsp", "Discard tag")
		if runtime.GOOS == "windows" {
			write(w, "  movq 8(%rax), %rdx", "len -> arg2")
			write(w, "  leaq 16(%rax), %rcx", "bytes -> arg1")
		} else {
			write(w, "  movq 8(%rax), %rsi", "len -> arg2")
			write(w, "  leaq 16(%rax), %rdi", "bytes -> arg1")
		}
		write(w, "  call __panic", "Panic (no return)")
		write(w, "  pushq $0", "Tag = INT (unreachable)")
		write(w, "  pushq $0", "Dummy result")
		return
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
			write(w, "  pushq $0", "Dummy result")
			return
		}
		c.emitDynamicPrint(w, e.Args[0], e.Name == "println")
	default:
		panic(fmt.Sprintf("unknown function: %s", e.Name))
	}
}

// emitDynamicPrint evaluates the single argument, pops the tagged slot, then
// dispatches at runtime: STR routes to __print_str(bytes_ptr, len) reading
// from the heap header; INT routes to __print_int(value).
func (c *Compiler) emitDynamicPrint(w io.Writer, arg Expr, isPrintln bool) {
	c.usesPrint = true
	c.usesPrintStr = true
	c.emitExpr(w, arg)
	write(w, "  popq %rax", "Pop payload (value or heap ptr)")
	write(w, "  popq %rbx", "Pop tag")
	id := c.labelCnt
	c.labelCnt++
	write(w, "  testq %rbx, %rbx", "Tag == INT?")
	write(w, fmt.Sprintf("  jz .Lprint_int_%d", id), "INT path")
	// STR path: rax = heap obj ptr
	if runtime.GOOS == "windows" {
		write(w, "  movq 8(%rax), %rdx", "len -> arg2")
		write(w, "  leaq 16(%rax), %rcx", "bytes -> arg1")
	} else {
		write(w, "  movq 8(%rax), %rsi", "len -> arg2")
		write(w, "  leaq 16(%rax), %rdi", "bytes -> arg1")
	}
	write(w, "  call __print_str", "Print string")
	write(w, "  xorq %rax, %rax", "STR path: result = 0")
	write(w, fmt.Sprintf("  jmp .Lprint_done_%d", id), "Done")
	write(w, fmt.Sprintf(".Lprint_int_%d:", id))
	if runtime.GOOS == "windows" {
		write(w, "  movq %rax, %rcx", "value -> arg1")
	} else {
		write(w, "  movq %rax, %rdi", "value -> arg1")
	}
	write(w, "  call __print_int", "Print int (returns value in RAX)")
	write(w, fmt.Sprintf(".Lprint_done_%d:", id))
	write(w, "  pushq $0", "Tag = INT")
	write(w, "  pushq %rax", "Push result")
	if isPrintln {
		c.emitPrintNl(w)
	}
}

func (c *Compiler) emitPrintStrCall(w io.Writer, label string, length int) {
	if runtime.GOOS == "windows" {
		write(w, fmt.Sprintf("  leaq %s+16(%%rip), %%rcx", label), "Bytes ptr (skip header)")
		write(w, fmt.Sprintf("  movq $%d, %%rdx", length), "Length")
	} else {
		write(w, fmt.Sprintf("  leaq %s+16(%%rip), %%rdi", label), "Bytes ptr (skip header)")
		write(w, fmt.Sprintf("  movq $%d, %%rsi", length), "Length")
	}
	write(w, "  call __print_str", "Print string")
}

func (c *Compiler) emitPrintNl(w io.Writer) {
	if runtime.GOOS == "windows" {
		write(w, "  leaq .Lnl(%rip), %rcx", "Newline ptr")
		write(w, "  movq $1, %rdx", "Length 1")
	} else {
		write(w, "  leaq .Lnl(%rip), %rdi", "Newline ptr")
		write(w, "  movq $1, %rsi", "Length 1")
	}
	write(w, "  call __print_str", "Print newline")
}

func (c *Compiler) emitBssVars(w io.Writer) {
	for name := range c.vars {
		write(w, fmt.Sprintf("var_%s:", name))
		write(w, ".space 16", "Variable storage (tag + payload)")
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
	c.emitStrToInt(w)
	c.emitArgGet(w)
	c.emitArgOob(w)
	c.emitPanic(w)
	c.emitAlloc(w)
	c.emitIntToStr(w)
	c.emitPrintln(w)
	c.emitPrintlnStr(w)
	c.emitData(w)
	write(w, ".bss")
	if c.usesPrint {
		write(w, "buffer:")
		write(w, ".space 32", "32-byte buffer for number")
	}
	c.emitHeapBss(w)
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
	c.emitStrToInt(w)
	c.emitArgGet(w)
	c.emitArgOob(w)
	c.emitPanic(w)
	c.emitAlloc(w)
	c.emitIntToStr(w)
	c.emitPrintln(w)
	c.emitPrintlnStr(w)
	c.emitData(w)
	write(w, ".bss")
	if c.usesPrint {
		write(w, "buffer:")
		write(w, ".space 32", "32-byte buffer for number")
	}
	if c.usesPrint || c.usesPrintStr || c.usesArg || c.usesPanic {
		write(w, "written:")
		write(w, ".space 8", "Bytes written")
	}
	if c.usesArg {
		write(w, "argv_ptr:")
		write(w, ".space 8", "LPWSTR* argv")
		write(w, "argc_storage:")
		write(w, ".space 8", "int argc")
	}
	c.emitHeapBss(w)
	c.emitBssVars(w)
	return nil
}

func (c *Compiler) emitHeapBss(w io.Writer) {
	if !c.usesHeap() {
		return
	}
	write(w, "__heap_off:")
	write(w, ".space 8", "current bump offset (init 0)")
	write(w, "__heap:")
	write(w, fmt.Sprintf(".space %d", heapSize), "bump heap")
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

func (c *Compiler) emitStrToInt(w io.Writer) {
	if !c.usesStrToInt {
		return
	}
	write(w, "__str_to_int:")
	write(w, "  xorq %rax, %rax", "result = 0")
	write(w, "  xorq %rcx, %rcx", "sign flag = 0")
	write(w, "  testq %rsi, %rsi", "Empty?")
	write(w, "  jz __str_to_int_ret", "Done")
	write(w, "  movzbq (%rdi), %rdx", "Load first byte")
	write(w, "  cmpb $45, %dl", "'-'")
	write(w, "  jne __str_to_int_loop", "Not '-': skip")
	write(w, "  movq $1, %rcx", "negative")
	write(w, "  incq %rdi", "Skip '-'")
	write(w, "  decq %rsi", "len--")
	write(w, "__str_to_int_loop:")
	write(w, "  testq %rsi, %rsi", "End?")
	write(w, "  jz __str_to_int_done", "Done")
	write(w, "  movzbq (%rdi), %rdx", "Load byte")
	write(w, "  subq $48, %rdx", "'0'")
	write(w, "  imulq $10, %rax", "result *= 10")
	write(w, "  addq %rdx, %rax", "result += digit")
	write(w, "  incq %rdi", "Advance")
	write(w, "  decq %rsi", "len--")
	write(w, "  jmp __str_to_int_loop", "Continue")
	write(w, "__str_to_int_done:")
	write(w, "  testq %rcx, %rcx", "Negative?")
	write(w, "  jz __str_to_int_ret", "Skip negation")
	write(w, "  negq %rax", "Apply sign")
	write(w, "__str_to_int_ret:")
	write(w, "  ret", "Return")
	write(w, "")
}

// __alloc(rdi=size) -> rax=ptr. BSS bump allocator. Leaf, never frees.
func (c *Compiler) emitAlloc(w io.Writer) {
	if !c.usesHeap() {
		return
	}
	write(w, "__alloc:")
	write(w, "  movq __heap_off(%rip), %rax", "current offset")
	write(w, "  leaq __heap(%rip), %rdx", "heap base")
	write(w, "  addq %rdx, %rax", "ptr = base + offset")
	write(w, "  addq %rdi, __heap_off(%rip)", "advance offset by size")
	write(w, "  ret", "Return")
	write(w, "")
}

// __int_to_str(rdi=value) -> rax=ptr to heap obj. Renders the int as ASCII
// into a stack buffer, then bump-allocates a [refcount,len,bytes] object and
// copies. Frame: 32B buffer + 24B spills = 56 (keeps RSP 16-aligned).
func (c *Compiler) emitIntToStr(w io.Writer) {
	if !c.usesIntToStr {
		return
	}
	write(w, "__int_to_str:")
	write(w, "  subq $56, %rsp", "32B buffer + 24B spills")
	write(w, "  movq %rdi, %r10", "Save original (for sign check)")
	write(w, "  movq %rdi, %rax", "value")
	write(w, "  testq %rax, %rax", "Check sign")
	write(w, "  jns .Lits_abs", "Non-negative: skip negation")
	write(w, "  negq %rax", "Absolute value")
	write(w, ".Lits_abs:")
	write(w, "  movq $10, %r8", "Base 10")
	write(w, "  leaq 31(%rsp), %r9", "Last byte of buffer")
	write(w, ".Lits_conv:")
	write(w, "  xorq %rdx, %rdx", "Clear RDX")
	write(w, "  divq %r8", "RAX / 10")
	write(w, "  addb $48, %dl", "Digit to ASCII")
	write(w, "  movb %dl, (%r9)", "Store digit")
	write(w, "  decq %r9", "Move back")
	write(w, "  testq %rax, %rax", "More digits?")
	write(w, "  jnz .Lits_conv", "Continue")
	write(w, "  testq %r10, %r10", "Original negative?")
	write(w, "  jns .Lits_pos", "Non-negative: skip sign")
	write(w, "  movb $45, (%r9)", "'-' sign")
	write(w, "  decq %r9", "Move back")
	write(w, ".Lits_pos:")
	write(w, "  incq %r9", "First char")
	write(w, "  leaq 32(%rsp), %rcx", "Past end of buffer")
	write(w, "  subq %r9, %rcx", "rcx = length")
	write(w, "  movq %r9, 32(%rsp)", "Spill chars ptr")
	write(w, "  movq %rcx, 40(%rsp)", "Spill length")
	write(w, "  movq %rcx, %rdi", "size = length")
	write(w, "  addq $16, %rdi", "+ header")
	write(w, "  call __alloc", "rax = heap obj ptr")
	write(w, "  movq %rax, 48(%rsp)", "Spill heap obj ptr")
	write(w, "  movq $0, (%rax)", "header.refcount = 0")
	write(w, "  movq 40(%rsp), %rcx", "length")
	write(w, "  movq %rcx, 8(%rax)", "header.len")
	write(w, "  movq 32(%rsp), %rsi", "src = chars ptr")
	write(w, "  leaq 16(%rax), %rdi", "dst = bytes start")
	write(w, "  cld", "DF = 0")
	write(w, "  rep movsb", "copy rcx bytes")
	write(w, "  movq 48(%rsp), %rax", "heap obj ptr")
	write(w, "  addq $56, %rsp", "Restore stack")
	write(w, "  ret", "Return")
	write(w, "")
}

// __arg_get(rdi=idx) -> rax=ptr to heap object [refcount, len, bytes].
// Linux: copies argv[idx] (UTF-8) into freshly bump-allocated heap object.
// Windows: WideCharToMultiByte the wide argv[idx] into bump-allocated object.
func (c *Compiler) emitArgGet(w io.Writer) {
	if !c.usesArg {
		return
	}
	if runtime.GOOS == "windows" {
		c.emitArgGetWindows(w)
		return
	}
	write(w, "__arg_get:")
	write(w, "  cmpq $1, %rdi", "idx >= 1?")
	write(w, "  jl __arg_oob", "out of range")
	write(w, "  movq (%rbp), %rax", "argc")
	write(w, "  cmpq %rax, %rdi", "idx < argc?")
	write(w, "  jge __arg_oob", "out of range")
	write(w, "  incq %rdi", "N+1 (skip argc slot in argv frame)")
	write(w, "  movq (%rbp,%rdi,8), %rsi", "argv[N] (UTF-8 ptr)")
	write(w, "  movq %rsi, %rcx", "Walk from start")
	write(w, "__arg_get_len:")
	write(w, "  cmpb $0, (%rcx)", "Null byte?")
	write(w, "  je __arg_get_len_done", "Done")
	write(w, "  incq %rcx", "Next byte")
	write(w, "  jmp __arg_get_len", "Loop")
	write(w, "__arg_get_len_done:")
	write(w, "  subq %rsi, %rcx", "rcx = len")
	write(w, "  pushq %rsi", "save src ptr")
	write(w, "  pushq %rcx", "save len")
	write(w, "  movq %rcx, %rdi", "size = len")
	write(w, "  addq $16, %rdi", "+ header (refcount + len)")
	write(w, "  call __alloc", "rax = heap obj ptr")
	write(w, "  popq %rcx", "len")
	write(w, "  popq %rsi", "src ptr")
	write(w, "  movq $0, (%rax)", "header.refcount = 0")
	write(w, "  movq %rcx, 8(%rax)", "header.len")
	write(w, "  leaq 16(%rax), %rdi", "dst = bytes start")
	write(w, "  pushq %rax", "save heap obj ptr")
	write(w, "  cld", "DF = 0 for forward copy")
	write(w, "  rep movsb", "copy rcx bytes from rsi to rdi")
	write(w, "  popq %rax", "restore heap obj ptr")
	write(w, "  ret", "Return")
	write(w, "")
}

func (c *Compiler) emitArgGetWindows(w io.Writer) {
	// Frame: shadow(32) + 4 stack args(32) + 3 spills(24) = 88 bytes (16-aligned).
	write(w, "__arg_get:")
	write(w, "  cmpq $1, %rdi", "idx >= 1?")
	write(w, "  jl __arg_oob", "out of range")
	write(w, "  movq argc_storage(%rip), %rax", "argc")
	write(w, "  cmpq %rax, %rdi", "idx < argc?")
	write(w, "  jge __arg_oob", "out of range")
	write(w, "  subq $88, %rsp", "frame")
	write(w, "  movq argv_ptr(%rip), %rax", "argv base")
	write(w, "  movq (%rax,%rdi,8), %r10", "wide ptr argv[idx]")
	write(w, "  movq %r10, 64(%rsp)", "spill wide ptr")
	// 1st WideCharToMultiByte: query byte size (incl null terminator).
	write(w, "  movq $65001, %rcx", "CP_UTF8")
	write(w, "  xorq %rdx, %rdx", "dwFlags")
	write(w, "  movq %r10, %r8", "lpWideCharStr")
	write(w, "  movq $-1, %r9", "cchWideChar = -1 (null-term)")
	write(w, "  movq $0, 32(%rsp)", "lpMultiByteStr = NULL")
	write(w, "  movq $0, 40(%rsp)", "cbMultiByte = 0 (query)")
	write(w, "  movq $0, 48(%rsp)", "lpDefaultChar = NULL")
	write(w, "  movq $0, 56(%rsp)", "lpUsedDefaultChar = NULL")
	write(w, "  call WideCharToMultiByte", "RAX = byte count incl null")
	write(w, "  movq %rax, 72(%rsp)", "spill byte count")
	// __alloc(size = 16 (header) + (byteCount - 1) (bytes excl null))
	//                = 15 + byteCount
	write(w, "  leaq 15(%rax), %rdi", "alloc size = 16 + byteCount - 1")
	write(w, "  call __alloc", "rax = heap obj ptr")
	write(w, "  movq %rax, 80(%rsp)", "spill heap obj ptr")
	write(w, "  movq $0, (%rax)", "header.refcount = 0")
	write(w, "  movq 72(%rsp), %rdx", "byte count")
	write(w, "  decq %rdx", "len = byteCount - 1")
	write(w, "  movq %rdx, 8(%rax)", "header.len")
	// 2nd WideCharToMultiByte: convert into heap+16
	write(w, "  movq $65001, %rcx", "CP_UTF8")
	write(w, "  xorq %rdx, %rdx", "dwFlags")
	write(w, "  movq 64(%rsp), %r8", "lpWideCharStr")
	write(w, "  movq $-1, %r9", "cchWideChar = -1")
	write(w, "  movq 80(%rsp), %r10", "heap obj ptr")
	write(w, "  addq $16, %r10", "bytes start")
	write(w, "  movq %r10, 32(%rsp)", "lpMultiByteStr")
	write(w, "  movq 72(%rsp), %r10", "byte count")
	write(w, "  movq %r10, 40(%rsp)", "cbMultiByte")
	write(w, "  movq $0, 48(%rsp)", "lpDefaultChar = NULL")
	write(w, "  movq $0, 56(%rsp)", "lpUsedDefaultChar = NULL")
	write(w, "  call WideCharToMultiByte", "Convert into buf")
	write(w, "  movq 80(%rsp), %rax", "heap obj ptr")
	write(w, "  addq $88, %rsp", "Restore stack")
	write(w, "  ret", "Return")
	write(w, "")
}

// __arg_oob writes "arg out of range\n" to stderr and exits 1.
func (c *Compiler) emitArgOob(w io.Writer) {
	if !c.usesArg {
		return
	}
	if runtime.GOOS == "windows" {
		write(w, "__arg_oob:")
		write(w, "  subq $40, %rsp", "shadow + 5th arg (no return path)")
		write(w, "  movq $-12, %rcx", "STD_ERROR_HANDLE")
		write(w, "  call GetStdHandle", "RAX = stderr")
		write(w, "  movq %rax, %rcx", "Handle")
		write(w, "  leaq .Lerr_oob(%rip), %rdx", "Buffer")
		write(w, "  movq $17, %r8", "Length")
		write(w, "  leaq written(%rip), %r9", "Bytes written")
		write(w, "  movq $0, 32(%rsp)", "lpOverlapped = NULL")
		write(w, "  call WriteFile", "Write to stderr")
		write(w, "  movq $1, %rcx", "Exit code 1")
		write(w, "  call ExitProcess", "Exit (no return)")
		write(w, "")
		return
	}
	write(w, "__arg_oob:")
	write(w, "  movq $1, %rax", "Syscall: write")
	write(w, "  movq $2, %rdi", "stderr")
	write(w, "  leaq .Lerr_oob(%rip), %rsi", "Buffer")
	write(w, "  movq $17, %rdx", "Length")
	write(w, "  syscall", "Call kernel")
	write(w, "  movq $60, %rax", "Syscall: exit")
	write(w, "  movq $1, %rdi", "Status 1")
	write(w, "  syscall", "Call kernel (no return)")
	write(w, "")
}

// __panic(bytes_ptr, len) writes the message to stderr and exits 1. Never
// returns. Linux args: rdi=ptr, rsi=len. Windows args: rcx=ptr, rdx=len.
// __div_zero is a small trampoline used by div/mod runtime checks.
func (c *Compiler) emitPanic(w io.Writer) {
	if !c.usesPanic {
		return
	}
	if runtime.GOOS == "windows" {
		write(w, "__panic:")
		write(w, "  subq $56, %rsp", "Shadow + 5th arg + spill")
		write(w, "  movq %rcx, 40(%rsp)", "Spill bytes ptr")
		write(w, "  movq %rdx, 48(%rsp)", "Spill len")
		write(w, "  movq $-12, %rcx", "STD_ERROR_HANDLE")
		write(w, "  call GetStdHandle", "RAX = stderr")
		write(w, "  movq %rax, %r12", "Save handle (callee-saved)")
		write(w, "  movq %r12, %rcx", "Handle")
		write(w, "  movq 40(%rsp), %rdx", "Buffer ptr")
		write(w, "  movq 48(%rsp), %r8", "Length")
		write(w, "  leaq written(%rip), %r9", "Bytes written")
		write(w, "  movq $0, 32(%rsp)", "lpOverlapped = NULL")
		write(w, "  call WriteFile", "Write message")
		write(w, "  movq %r12, %rcx", "Handle")
		write(w, "  leaq .Lnl(%rip), %rdx", "Newline ptr")
		write(w, "  movq $1, %r8", "Length 1")
		write(w, "  leaq written(%rip), %r9", "Bytes written")
		write(w, "  movq $0, 32(%rsp)", "lpOverlapped = NULL")
		write(w, "  call WriteFile", "Write trailing newline")
		write(w, "  movq $1, %rcx", "Exit code 1")
		write(w, "  call ExitProcess", "Exit (no return)")
		write(w, "")
		write(w, "__div_zero:")
		write(w, "  leaq .Lerr_div(%rip), %rcx", "Buffer")
		write(w, "  movq $16, %rdx", "Length")
		write(w, "  jmp __panic", "Tail-call panic")
		write(w, "")
		return
	}
	write(w, "__panic:")
	write(w, "  movq %rsi, %rdx", "Length to RDX (syscall arg 3)")
	write(w, "  movq %rdi, %rsi", "Buffer to RSI (syscall arg 2)")
	write(w, "  movq $2, %rdi", "stderr")
	write(w, "  movq $1, %rax", "Syscall: write")
	write(w, "  syscall", "Call kernel")
	write(w, "  movq $1, %rax", "Syscall: write")
	write(w, "  movq $2, %rdi", "stderr")
	write(w, "  leaq .Lnl(%rip), %rsi", "Newline ptr")
	write(w, "  movq $1, %rdx", "Length 1")
	write(w, "  syscall", "Write trailing newline")
	write(w, "  movq $60, %rax", "Syscall: exit")
	write(w, "  movq $1, %rdi", "Status 1")
	write(w, "  syscall", "Call kernel (no return)")
	write(w, "")
	write(w, "__div_zero:")
	write(w, "  leaq .Lerr_div(%rip), %rdi", "Buffer")
	write(w, "  movq $16, %rsi", "Length")
	write(w, "  jmp __panic", "Tail-call panic")
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
	if len(c.strLits) == 0 && !c.usesPrintStr && !c.usesArg && !c.usesPanic {
		return
	}
	write(w, ".data")
	for i, s := range c.strLits {
		write(w, fmt.Sprintf(".Lstr_%d:", i))
		write(w, ".quad 0", "refcount placeholder")
		write(w, fmt.Sprintf(".quad %d", len(s)), "len")
		write(w, fmt.Sprintf(".ascii %q", s))
	}
	if c.usesPrintStr || c.usesPanic {
		write(w, ".Lnl:")
		write(w, `.ascii "\n"`)
	}
	if c.usesArg {
		write(w, ".Lerr_oob:")
		write(w, `.ascii "arg out of range\n"`)
	}
	if c.usesPanic {
		write(w, ".Lerr_div:")
		write(w, `.ascii "division by zero"`)
	}
}
