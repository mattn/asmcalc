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
	varValues    map[string]Value
	args         []int
	usesArg      bool
	usesAtoi     bool
	usesPrint    bool
	usesPrintStr bool
	strLits      []string
	labelCnt     int
	depth        int
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
	c.varValues = map[string]Value{}
	c.args = args
	var result Value
	for _, stmt := range c.program.Stmts {
		result = c.evalStmt(stmt)
	}
	return result.I
}

func (c *Compiler) evalStmt(s Stmt) Value {
	switch s := s.(type) {
	case *AssignStmt:
		v := c.evalExpr(s.Value)
		c.varValues[s.Name] = v
		return v
	case *ExprStmt:
		return c.evalExpr(s.X)
	case *IfStmt:
		var result Value
		var branch []Stmt
		if c.evalExpr(s.Cond).I != 0 {
			branch = s.Then
		} else {
			branch = s.Else
		}
		for _, t := range branch {
			result = c.evalStmt(t)
		}
		return result
	case *WhileStmt:
		var result Value
		for c.evalExpr(s.Cond).I != 0 {
			for _, t := range s.Body {
				result = c.evalStmt(t)
			}
		}
		return result
	}
	panic("unknown stmt")
}

func (c *Compiler) evalExpr(e Expr) Value {
	switch e := e.(type) {
	case *NumLit:
		return intVal(e.Value)
	case *ArgRef:
		idx := c.evalExpr(e.Index).I
		if idx < 1 || idx > len(c.args) {
			panic(fmt.Sprintf("arg(%d) not provided", idx))
		}
		return intVal(c.args[idx-1])
	case *NargExpr:
		return intVal(len(c.args))
	case *VarRef:
		return c.varValues[e.Name]
	case *CallExpr:
		switch e.Name {
		case "print":
			if len(e.Args) != 1 {
				panic(fmt.Sprintf("print takes 1 arg, got %d", len(e.Args)))
			}
			v := c.evalExpr(e.Args[0])
			if v.Tag == TagStr {
				fmt.Print(v.S)
			} else {
				fmt.Print(v.I)
			}
			return v
		case "println":
			if len(e.Args) != 1 {
				panic(fmt.Sprintf("println takes 1 arg, got %d", len(e.Args)))
			}
			v := c.evalExpr(e.Args[0])
			if v.Tag == TagStr {
				fmt.Println(v.S)
			} else {
				fmt.Println(v.I)
			}
			return v
		}
		panic(fmt.Sprintf("unknown function: %s", e.Name))
	case *StrLit:
		return strVal(e.Value)
	case *BinOp:
		l := c.evalExpr(e.L).I
		r := c.evalExpr(e.R).I
		switch e.Op {
		case TOK_PLUS:
			return intVal(l + r)
		case TOK_MINUS:
			return intVal(l - r)
		case TOK_MUL:
			return intVal(l * r)
		case TOK_DIV:
			return intVal(l / r)
		case TOK_MOD:
			return intVal(l % r)
		case TOK_EQ:
			if l == r {
				return intVal(1)
			}
			return intVal(0)
		case TOK_NE:
			if l != r {
				return intVal(1)
			}
			return intVal(0)
		case TOK_LT:
			if l < r {
				return intVal(1)
			}
			return intVal(0)
		case TOK_LE:
			if l <= r {
				return intVal(1)
			}
			return intVal(0)
		case TOK_GT:
			if l > r {
				return intVal(1)
			}
			return intVal(0)
		case TOK_GE:
			if l >= r {
				return intVal(1)
			}
			return intVal(0)
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
	c.labelCnt = 0
	c.depth = 0

	if runtime.GOOS == "windows" {
		return c.compileWindows(w)
	}
	return c.compileLinux(w)
}
