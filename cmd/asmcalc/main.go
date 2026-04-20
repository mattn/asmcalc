package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mattn/asmcalc"
)

func main() {
	filename := flag.String("f", "", "read expression from file")
	flag.Parse()

	var expr string
	if *filename != "" {
		b, err := os.ReadFile(*filename)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		expr = string(b)
	} else {
		if flag.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "usage: asmcalc [-f file] expr")
			os.Exit(1)
		}
		expr = flag.Arg(0)
	}

	compiler := asmcalc.NewCompiler(expr)
	compiler.Lex()
	compiler.Compile(os.Stdout)
}
