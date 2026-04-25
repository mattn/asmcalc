package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/mattn/mame"
)

func main() {
	filename := flag.String("f", "", "read expression from file")
	run := flag.Bool("run", false, "compile and run the expression")
	eval := flag.Bool("eval", false, "evaluate the expression in-process (no asm pipeline)")
	flag.Parse()

	if *run && *eval {
		fmt.Fprintln(os.Stderr, "-run and -eval are mutually exclusive")
		os.Exit(1)
	}

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
			fmt.Fprintln(os.Stderr, "usage: mame [-run|-eval] [-f file] expr [args...]")
			os.Exit(1)
		}
		expr = flag.Arg(0)
		runtimeArgs = flag.Args()[1:]
	}

	compiler := mame.NewCompiler(expr)
	compiler.Lex()

	if *eval {
		intArgs := make([]int, len(runtimeArgs))
		for i, a := range runtimeArgs {
			n, err := strconv.Atoi(a)
			if err != nil {
				fmt.Fprintf(os.Stderr, "arg $%d: %v\n", i+1, err)
				os.Exit(1)
			}
			intArgs[i] = n
		}
		compiler.Eval(intArgs...)
		return
	}

	if !*run {
		compiler.Compile(os.Stdout)
		return
	}

	if err := runExpr(compiler, runtimeArgs); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runExpr(compiler *mame.Compiler, args []string) error {
	tmpDir, err := os.MkdirTemp("", "mame-*")
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
