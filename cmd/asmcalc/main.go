package main

import (
	"github.com/mattn/asmcalc"
	"os"
)

func main() {
	compiler := asmcalc.NewCompiler(os.Args[1])
	compiler.Lex()
	compiler.Compile(os.Stdout)
}
