package mame

import (
	"fmt"
	"io"
	"runtime"
)

type Compiler struct {
	input        string
	pos          int
	tokens       []Token
	tokenPos     int
	program      *Program
	vars         map[string]bool
	varValues    map[string]int
	args         []int
	usesArg      bool
	usesPrint    bool
	usesPrintStr bool
	strLits      []string
}

func NewCompiler(input string) *Compiler {
	return &Compiler{
		input:  input,
		tokens: make([]Token, 0),
	}
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
		case "print":
			if len(e.Args) != 1 {
				panic(fmt.Sprintf("print takes 1 arg, got %d", len(e.Args)))
			}
			if str, ok := e.Args[0].(*StrLit); ok {
				fmt.Print(str.Value)
				return 0
			}
			v := c.evalExpr(e.Args[0])
			fmt.Print(v)
			return v
		case "println":
			if len(e.Args) != 1 {
				panic(fmt.Sprintf("println takes 1 arg, got %d", len(e.Args)))
			}
			if str, ok := e.Args[0].(*StrLit); ok {
				fmt.Println(str.Value)
				return 0
			}
			v := c.evalExpr(e.Args[0])
			fmt.Println(v)
			return v
		}
		panic(fmt.Sprintf("unknown function: %s", e.Name))
	case *StrLit:
		panic("string literal can only appear as a println argument")
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
		case TOK_EQ:
			if l == r {
				return 1
			}
			return 0
		case TOK_NE:
			if l != r {
				return 1
			}
			return 0
		case TOK_LT:
			if l < r {
				return 1
			}
			return 0
		case TOK_LE:
			if l <= r {
				return 1
			}
			return 0
		case TOK_GT:
			if l > r {
				return 1
			}
			return 0
		case TOK_GE:
			if l >= r {
				return 1
			}
			return 0
		}
	}
	panic("unknown expr")
}

func (c *Compiler) Compile(w io.Writer) error {
	if c.program == nil {
		c.Parse()
	}
	c.vars = map[string]bool{}
	c.strLits = nil

	if runtime.GOOS == "windows" {
		return c.compileWindows(w)
	}
	return c.compileLinux(w)
}
