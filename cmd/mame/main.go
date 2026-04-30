package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/mattn/mame"
)

const name = "mame"

const version = "0.0.0"

var revision = "HEAD"

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
	case "compile":
		cmdCompile(args)
	case "version":
		cmdVersion()
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  mame asm     [-e expr] [file]")
	fmt.Fprintln(os.Stderr, "  mame compile [-o out] [-e expr] [file]")
	fmt.Fprintln(os.Stderr, "  mame run     [-e expr] [file] [args...]")
	fmt.Fprintln(os.Stderr, "  mame eval    [-e expr] [file] [args...]")
	fmt.Fprintln(os.Stderr, "  mame version")
}

func cmdVersion() {
	fmt.Printf("%s %s (rev: %s)\n", name, version, revision)
}

func loadCompiler(fs *flag.FlagSet, args []string) (*mame.Compiler, []string) {
	expr := fs.String("e", "", "evaluate expression instead of reading a file")
	fs.Parse(args)

	var src string
	var runtimeArgs []string
	if *expr != "" {
		src = *expr
		runtimeArgs = fs.Args()
	} else {
		if fs.NArg() < 1 {
			fmt.Fprintf(os.Stderr, "mame %s: file or -e required\n", fs.Name())
			os.Exit(1)
		}
		b, err := os.ReadFile(fs.Arg(0))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		src = string(b)
		runtimeArgs = fs.Args()[1:]
	}

	compiler := mame.NewCompiler(src)
	compiler.Lex()
	return compiler, runtimeArgs
}

func cmdAsm(args []string) {
	fs := flag.NewFlagSet("asm", flag.ExitOnError)
	compiler, _ := loadCompiler(fs, args)
	compiler.Compile(os.Stdout)
}

func cmdEval(args []string) {
	fs := flag.NewFlagSet("eval", flag.ExitOnError)
	compiler, runtimeArgs := loadCompiler(fs, args)
	compiler.Eval(runtimeArgs...)
}

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	compiler, runtimeArgs := loadCompiler(fs, args)

	tmpDir, err := os.MkdirTemp("", "mame-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	exeFile := filepath.Join(tmpDir, "out")
	if runtime.GOOS == "windows" {
		exeFile += ".exe"
	}

	if err := buildExe(compiler, exeFile, tmpDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	cmd := exec.Command(exeFile, runtimeArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func cmdCompile(args []string) {
	fs := flag.NewFlagSet("compile", flag.ExitOnError)
	output := fs.String("o", defaultOutputName(), "output executable path")
	compiler, _ := loadCompiler(fs, args)

	tmpDir, err := os.MkdirTemp("", "mame-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	if err := buildExe(compiler, *output, tmpDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func defaultOutputName() string {
	if runtime.GOOS == "windows" {
		return "a.exe"
	}
	return "a.out"
}

func buildExe(compiler *mame.Compiler, exePath, tmpDir string) error {
	asmFile := filepath.Join(tmpDir, "out.s")
	objFile := filepath.Join(tmpDir, "out.o")

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

	ldArgs := []string{objFile, "-o", exePath}
	if runtime.GOOS == "windows" {
		ldArgs = append(ldArgs, "-lkernel32", "-lshell32")
	}
	if out, err := exec.Command("ld", ldArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("ld failed: %v\n%s", err, out)
	}

	return nil
}
