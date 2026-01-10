package asmcalc

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestEval(t *testing.T) {
	tests := []struct {
		expr string
		want int
	}{
		{"1+2", 3},
		{"2+3*4", 14},
		{"(2+3)*4", 20},
		{"20/4+2", 7},
		{"1+2-3+4", 4},
		{"10-2*3", 4},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			compiler := NewCompiler(tt.expr)
			compiler.Lex()
			got := compiler.Eval()
			if got != tt.want {
				t.Errorf("Eval(%q) = %d, want %d", tt.expr, got, tt.want)
			}
		})
	}
}

func TestCompile(t *testing.T) {
	tests := []struct {
		expr string
		want int
	}{
		{"1+2", 3},
		{"2+3*4", 14},
		{"(2+3)*4", 20},
		{"20/4+2", 7},
		{"1+2-3+4", 4},
		{"10-2*3", 4},
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

			ldCmd := exec.Command("ld", objFile, "-o", exeFile)
			if out, err := ldCmd.CombinedOutput(); err != nil {
				t.Fatalf("ld failed: %v\n%s", err, out)
			}

			runCmd := exec.Command(exeFile)
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
