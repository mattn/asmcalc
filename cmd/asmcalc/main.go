package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/mattn/asmcalc"
)

func main() {
	filename := flag.String("f", "", "read expression from file")
	run := flag.Bool("run", false, "compile and run the expression")
	flag.Parse()

	var expr string
	var runtimeArgs []string
	if *filename != "" {
		b, err := os.ReadFile(*filename)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		expr = string(b)
		runtimeArgs = flag.Args()
	} else {
		if flag.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "usage: asmcalc [-run] [-f file] expr [args...]")
			os.Exit(1)
		}
		expr = flag.Arg(0)
		runtimeArgs = flag.Args()[1:]
	}

	compiler := asmcalc.NewCompiler(expr)
	compiler.Lex()

	if !*run {
		compiler.Compile(os.Stdout)
		return
	}

	if err := runExpr(compiler, runtimeArgs); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runExpr(compiler *asmcalc.Compiler, args []string) error {
	tmpDir, err := os.MkdirTemp("", "asmcalc-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	asmFile := filepath.Join(tmpDir, "out.s")
	objFile := filepath.Join(tmpDir, "out.o")
	exeFile := filepath.Join(tmpDir, "out")
	if runtime.GOOS == "windows" {
		exeFile += ".exe"
	}

	f, err := os.Create(asmFile)
	if err != nil {
		return err
	}
	if err := compiler.Compile(f); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	if out, err := exec.Command("as", "-64", asmFile, "-o", objFile).CombinedOutput(); err != nil {
		return fmt.Errorf("as failed: %v\n%s", err, out)
	}

	ldArgs := []string{objFile, "-o", exeFile}
	if runtime.GOOS == "windows" {
		ldArgs = append(ldArgs, "-lkernel32", "-lshell32")
	}
	if out, err := exec.Command("ld", ldArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("ld failed: %v\n%s", err, out)
	}

	cmd := exec.Command(exeFile, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
