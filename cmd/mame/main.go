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
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	sub := os.Args[1]
	args := os.Args[2:]

	switch sub {
	case "asm":
		cmdAsm(args)
	case "run":
		cmdRun(args)
	case "eval":
		cmdEval(args)
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  mame asm  [-f file] expr")
	fmt.Fprintln(os.Stderr, "  mame run  [-f file] expr [args...]")
	fmt.Fprintln(os.Stderr, "  mame eval [-f file] expr [args...]")
}

func loadCompiler(name string, args []string) (*mame.Compiler, []string) {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	filename := fs.String("f", "", "read expression from file")
	fs.Parse(args)

	var expr string
	var runtimeArgs []string
	if *filename != "" {
		b, err := os.ReadFile(*filename)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		expr = string(b)
		runtimeArgs = fs.Args()
	} else {
		if fs.NArg() < 1 {
			fmt.Fprintf(os.Stderr, "mame %s: expression required\n", name)
			os.Exit(1)
		}
		expr = fs.Arg(0)
		runtimeArgs = fs.Args()[1:]
	}

	compiler := mame.NewCompiler(expr)
	compiler.Lex()
	return compiler, runtimeArgs
}

func cmdAsm(args []string) {
	compiler, _ := loadCompiler("asm", args)
	compiler.Compile(os.Stdout)
}

func cmdEval(args []string) {
	compiler, runtimeArgs := loadCompiler("eval", args)
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
}

func cmdRun(args []string) {
	compiler, runtimeArgs := loadCompiler("run", args)
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
