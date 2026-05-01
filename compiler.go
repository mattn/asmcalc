package mame

import (
	"fmt"
	"io"
	"runtime"
	"strconv"
)

type Compiler struct {
	input          string
	pos            int
	tokens         []Token
	tokenPos       int
	program        *Program
	vars           map[string]bool
	varValues      map[string]Value
	args           []string
	usesArg        bool
	usesPrint      bool
	usesPrintStr   bool
	usesPrintFloat bool
	usesStrToInt   bool
	usesIntToStr   bool
	usesStrToFloat bool
	usesFloatToStr bool
	usesPanic      bool
	usesOpTypeErr  bool
	strLits        []string
	labelCnt       int
	loopEndLabels  []int
}

type breakSignal struct{}

func (c *Compiler) usesHeap() bool {
	return c.usesArg || c.usesIntToStr || c.usesFloatToStr
}

func (c *Compiler) usesFloatRender() bool {
	return c.usesPrintFloat || c.usesFloatToStr
}

func NewCompiler(input string) *Compiler {
	return &Compiler{
		input:  input,
		tokens: make([]Token, 0),
	}
}

func (c *Compiler) Eval(args ...string) int {
	if c.program == nil {
		c.Parse()
	}
	c.varValues = map[string]Value{}
	c.args = args
	var result Value
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(breakSignal); ok {
				panic("break outside of loop")
			}
			panic(r)
		}
	}()
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
		func() {
			defer func() {
				if r := recover(); r != nil {
					if _, ok := r.(breakSignal); !ok {
						panic(r)
					}
				}
			}()
			for c.evalExpr(s.Cond).I != 0 {
				for _, t := range s.Body {
					result = c.evalStmt(t)
				}
			}
		}()
		return result
	case *BreakStmt:
		panic(breakSignal{})
	}
	panic("unknown stmt")
}

func (c *Compiler) evalExpr(e Expr) Value {
	switch e := e.(type) {
	case *NumLit:
		return intVal(e.Value)
	case *FloatLit:
		return floatVal(e.Value)
	case *ArgRef:
		idx := c.evalExpr(e.Index).I
		if idx < 1 || idx > len(c.args) {
			panic(fmt.Sprintf("arg(%d) not provided", idx))
		}
		return strVal(c.args[idx-1])
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
			switch v.Tag {
			case TagStr:
				fmt.Print(v.S)
			case TagFloat:
				fmt.Printf("%.6f", v.F)
			default:
				fmt.Print(v.I)
			}
			return v
		case "println":
			if len(e.Args) != 1 {
				panic(fmt.Sprintf("println takes 1 arg, got %d", len(e.Args)))
			}
			v := c.evalExpr(e.Args[0])
			switch v.Tag {
			case TagStr:
				fmt.Println(v.S)
			case TagFloat:
				fmt.Printf("%.6f\n", v.F)
			default:
				fmt.Println(v.I)
			}
			return v
		case "int":
			if len(e.Args) != 1 {
				panic(fmt.Sprintf("int takes 1 arg, got %d", len(e.Args)))
			}
			v := c.evalExpr(e.Args[0])
			switch v.Tag {
			case TagInt:
				return v
			case TagFloat:
				return intVal(int(v.F))
			case TagStr:
				n, err := strconv.Atoi(v.S)
				if err != nil {
					panic(fmt.Sprintf("int(%q): %v", v.S, err))
				}
				return intVal(n)
			}
			panic("int(): unknown tag")
		case "float":
			if len(e.Args) != 1 {
				panic(fmt.Sprintf("float takes 1 arg, got %d", len(e.Args)))
			}
			v := c.evalExpr(e.Args[0])
			switch v.Tag {
			case TagFloat:
				return v
			case TagInt:
				return floatVal(float64(v.I))
			case TagStr:
				f, ok := atofMame(v.S)
				if !ok {
					panic(fmt.Sprintf("float(%q): invalid syntax", v.S))
				}
				return floatVal(f)
			}
			panic("float(): unknown tag")
		case "str":
			if len(e.Args) != 1 {
				panic(fmt.Sprintf("str takes 1 arg, got %d", len(e.Args)))
			}
			v := c.evalExpr(e.Args[0])
			switch v.Tag {
			case TagStr:
				return v
			case TagInt:
				return strVal(strconv.Itoa(v.I))
			case TagFloat:
				return strVal(fmt.Sprintf("%.6f", v.F))
			}
			panic("str(): unknown tag")
		case "len":
			if len(e.Args) != 1 {
				panic(fmt.Sprintf("len takes 1 arg, got %d", len(e.Args)))
			}
			v := c.evalExpr(e.Args[0])
			if v.Tag != TagStr {
				panic("len() expects a string")
			}
			return intVal(len(v.S))
		case "panic":
			if len(e.Args) != 1 {
				panic(fmt.Sprintf("panic takes 1 arg, got %d", len(e.Args)))
			}
			v := c.evalExpr(e.Args[0])
			if v.Tag != TagStr {
				panic("panic() expects a string")
			}
			panic(v.S)
		}
		panic(fmt.Sprintf("unknown function: %s", e.Name))
	case *StrLit:
		return strVal(e.Value)
	case *BinOp:
		l := c.evalExpr(e.L)
		r := c.evalExpr(e.R)
		if l.Tag == TagStr || r.Tag == TagStr {
			panic("invalid operand types")
		}
		if e.Op == TOK_MOD {
			if l.Tag != TagInt || r.Tag != TagInt {
				panic("invalid operand types")
			}
			if r.I == 0 {
				panic("division by zero")
			}
			return intVal(l.I % r.I)
		}
		if l.Tag == TagFloat || r.Tag == TagFloat {
			lf := l.F
			if l.Tag == TagInt {
				lf = float64(l.I)
			}
			rf := r.F
			if r.Tag == TagInt {
				rf = float64(r.I)
			}
			switch e.Op {
			case TOK_PLUS:
				return floatVal(lf + rf)
			case TOK_MINUS:
				return floatVal(lf - rf)
			case TOK_MUL:
				return floatVal(lf * rf)
			case TOK_DIV:
				return floatVal(lf / rf)
			case TOK_EQ:
				return boolVal(lf == rf)
			case TOK_NE:
				return boolVal(lf != rf)
			case TOK_LT:
				return boolVal(lf < rf)
			case TOK_LE:
				return boolVal(lf <= rf)
			case TOK_GT:
				return boolVal(lf > rf)
			case TOK_GE:
				return boolVal(lf >= rf)
			}
		}
		li := l.I
		ri := r.I
		switch e.Op {
		case TOK_PLUS:
			return intVal(li + ri)
		case TOK_MINUS:
			return intVal(li - ri)
		case TOK_MUL:
			return intVal(li * ri)
		case TOK_DIV:
			if ri == 0 {
				panic("division by zero")
			}
			return intVal(li / ri)
		case TOK_EQ:
			return boolVal(li == ri)
		case TOK_NE:
			return boolVal(li != ri)
		case TOK_LT:
			return boolVal(li < ri)
		case TOK_LE:
			return boolVal(li <= ri)
		case TOK_GT:
			return boolVal(li > ri)
		case TOK_GE:
			return boolVal(li >= ri)
		}
	}
	panic("unknown expr")
}

func boolVal(b bool) Value {
	if b {
		return intVal(1)
	}
	return intVal(0)
}

// atofMame mirrors __str_to_float (draft/atof.s) so eval and compile reject
// the same inputs.
func atofMame(s string) (float64, bool) {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	neg := false
	if i < len(s) {
		switch s[i] {
		case '-':
			neg = true
			i++
		case '+':
			i++
		}
	}
	var result float64
	digit := false
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		result = result*10 + float64(s[i]-'0')
		digit = true
		i++
	}
	if i < len(s) && s[i] == '.' {
		i++
		scale := 1.0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			scale /= 10
			result += float64(s[i]-'0') * scale
			digit = true
			i++
		}
	}
	if !digit || i != len(s) {
		return 0, false
	}
	if neg {
		result = -result
	}
	return result, true
}

func (c *Compiler) Compile(w io.Writer) error {
	if c.program == nil {
		c.Parse()
	}
	c.vars = map[string]bool{}
	c.strLits = nil
	c.labelCnt = 0

	if runtime.GOOS == "windows" {
		return c.compileWindows(w)
	}
	return c.compileLinux(w)
}
