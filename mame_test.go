package mame

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestLex(t *testing.T) {
	tests := []struct {
		input string
		want  []TokenType
	}{
		{"==", []TokenType{TOK_EQ, TOK_EOF}},
		{"!=", []TokenType{TOK_NE, TOK_EOF}},
		{"<", []TokenType{TOK_LT, TOK_EOF}},
		{"<=", []TokenType{TOK_LE, TOK_EOF}},
		{">", []TokenType{TOK_GT, TOK_EOF}},
		{">=", []TokenType{TOK_GE, TOK_EOF}},
		{"=", []TokenType{TOK_ASSIGN, TOK_EOF}},
		{"a==b", []TokenType{TOK_IDENT, TOK_EQ, TOK_IDENT, TOK_EOF}},
		{"x=1", []TokenType{TOK_IDENT, TOK_ASSIGN, TOK_NUM, TOK_EOF}},
		{"1<=2", []TokenType{TOK_NUM, TOK_LE, TOK_NUM, TOK_EOF}},
		{"a>=b", []TokenType{TOK_IDENT, TOK_GE, TOK_IDENT, TOK_EOF}},
		{"-1", []TokenType{TOK_NUM, TOK_EOF}},
		{"- -1", []TokenType{TOK_MINUS, TOK_NUM, TOK_EOF}},
		{"0--1", []TokenType{TOK_NUM, TOK_MINUS, TOK_MINUS, TOK_NUM, TOK_EOF}},
		{"0- -1", []TokenType{TOK_NUM, TOK_MINUS, TOK_NUM, TOK_EOF}},
		{"(-1)", []TokenType{TOK_LPAREN, TOK_NUM, TOK_RPAREN, TOK_EOF}},
		{"x=-1", []TokenType{TOK_IDENT, TOK_ASSIGN, TOK_NUM, TOK_EOF}},
		{"a-1", []TokenType{TOK_IDENT, TOK_MINUS, TOK_NUM, TOK_EOF}},
		{`"Fizz"`, []TokenType{TOK_STRING, TOK_EOF}},
		{`"a\nb"`, []TokenType{TOK_STRING, TOK_EOF}},
		{`println("Fizz")`, []TokenType{TOK_IDENT, TOK_LPAREN, TOK_STRING, TOK_RPAREN, TOK_EOF}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			c := NewCompiler(tt.input)
			c.Lex()
			got := make([]TokenType, len(c.tokens))
			for i, tok := range c.tokens {
				got[i] = tok.Type
			}
			if len(got) != len(tt.want) {
				t.Fatalf("Lex(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Lex(%q)[%d] = %d, want %d", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestLexString(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`"Fizz"`, "Fizz"},
		{`"a\nb"`, "a\nb"},
		{`"\\"`, `\`},
		{`"\""`, `"`},
		{`"\t"`, "\t"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			c := NewCompiler(tt.input)
			c.Lex()
			if c.tokens[0].Type != TOK_STRING {
				t.Fatalf("Lex(%q) first token = %s, want TOK_STRING", tt.input, c.tokens[0].Type)
			}
			if c.tokens[0].Name != tt.want {
				t.Errorf("Lex(%q) string = %q, want %q", tt.input, c.tokens[0].Name, tt.want)
			}
		})
	}
}

func TestEval(t *testing.T) {
	tests := []struct {
		expr string
		args []int
		want int
	}{
		{"1+2", nil, 3},
		{"2+3*4", nil, 14},
		{"(2+3)*4", nil, 20},
		{"20/4+2", nil, 7},
		{"1+2-3+4", nil, 4},
		{"10-2*3", nil, 4},
		{"10%3", nil, 1},
		{"15%5", nil, 0},
		{"100%7+1", nil, 3},
		{"x=10;x+5", nil, 15},
		{"x=2;y=3;x*y+1", nil, 7},
		{"x=5;x=x+1;x*2", nil, 12},
		{"$1+5", []int{10}, 15},
		{"$1*$2", []int{3, 4}, 12},
		{"x=$1;x*2+1", []int{7}, 15},
		{"x=$1\ny=$2\nx*y+1\n", []int{6, 7}, 43},
		{`println("Fizz")`, nil, 0},
		{"1==1", nil, 1},
		{"1==2", nil, 0},
		{"1!=2", nil, 1},
		{"3<5", nil, 1},
		{"5<=5", nil, 1},
		{"5>5", nil, 0},
		{"5>=5", nil, 1},
		{"15%3==0", nil, 1},
		{"15%4==0", nil, 0},
		{"1+2==3", nil, 1},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			compiler := NewCompiler(tt.expr)
			compiler.Lex()
			got := compiler.Eval(tt.args...)
			if got != tt.want {
				t.Errorf("Eval(%q) = %d, want %d", tt.expr, got, tt.want)
			}
		})
	}
}

func TestCompile(t *testing.T) {
	tests := []struct {
		expr string
		args []string
		want int
	}{
		{"println(1+2)", nil, 3},
		{"println(2+3*4)", nil, 14},
		{"println((2+3)*4)", nil, 20},
		{"println(20/4+2)", nil, 7},
		{"println(1+2-3+4)", nil, 4},
		{"println(10-2*3)", nil, 4},
		{"println(10%3)", nil, 1},
		{"println(15%5)", nil, 0},
		{"println(100%7+1)", nil, 3},
		{"x=10;println(x+5)", nil, 15},
		{"x=2;y=3;println(x*y+1)", nil, 7},
		{"x=5;x=x+1;println(x*2)", nil, 12},
		{"println($1+5)", []string{"10"}, 15},
		{"println($1*$2)", []string{"3", "4"}, 12},
		{"x=$1;println(x*2+1)", []string{"7"}, 15},
		{"x=$1\ny=$2\nprintln(x*y+1)\n", []string{"6", "7"}, 43},
		{"println(1==1)", nil, 1},
		{"println(1==2)", nil, 0},
		{"println(15%3==0)", nil, 1},
		{"println(7<5)", nil, 0},
		{"println(3>=3)", nil, 1},
	}

	tmpDir := t.TempDir()

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			compiler := NewCompiler(tt.expr)
			compiler.Lex()

			var buf bytes.Buffer
			if err := compiler.Compile(&buf); err != nil {
				t.Fatalf("Compile failed: %v", err)
			}

			asmFile := filepath.Join(tmpDir, "test.s")
			objFile := filepath.Join(tmpDir, "test.o")
			exeFile := filepath.Join(tmpDir, "test")
			if runtime.GOOS == "windows" {
				exeFile += ".exe"
			}

			defer func() {
				os.Remove(asmFile)
				os.Remove(objFile)
				os.Remove(exeFile)
			}()

			if err := os.WriteFile(asmFile, buf.Bytes(), 0644); err != nil {
				t.Fatalf("WriteFile failed: %v", err)
			}

			asCmd := exec.Command("as", "-64", asmFile, "-o", objFile)
			if out, err := asCmd.CombinedOutput(); err != nil {
				t.Fatalf("as failed: %v\n%s", err, out)
			}

			var ldCmd *exec.Cmd
			if runtime.GOOS == "windows" {
				ldCmd = exec.Command("ld", objFile, "-o", exeFile, "-lkernel32", "-lshell32")
			} else {
				ldCmd = exec.Command("ld", objFile, "-o", exeFile)
			}
			if out, err := ldCmd.CombinedOutput(); err != nil {
				t.Fatalf("ld failed: %v\n%s", err, out)
			}

			runCmd := exec.Command(exeFile, tt.args...)
			out, err := runCmd.CombinedOutput()
			if err != nil {
				t.Fatalf("execution failed: %v\n%s", err, out)
			}

			result := strings.TrimSpace(string(out))
			got, err := strconv.Atoi(result)
			if err != nil {
				t.Fatalf("failed to parse output %q: %v", result, err)
			}

			if got != tt.want {
				t.Errorf("Compile(%q) = %d, want %d", tt.expr, got, tt.want)
			}
		})
	}
}
