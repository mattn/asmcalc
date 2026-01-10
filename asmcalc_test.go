package asmcalc

import (
	"fmt"
	"testing"
)

func TestCompile(t *testing.T) {
	compiler := NewCompiler("3 + 5 * (2 - 8)")
	compiler.Lex()
	for _, tok := range compiler.tokens {
		switch tok.Type {
		case TOK_NUM:
			fmt.Print(tok.Value)
		case TOK_PLUS:
			fmt.Printf("+")
		case TOK_MINUS:
			fmt.Printf("-")
		case TOK_MUL:
			fmt.Printf("*")
		case TOK_DIV:
			fmt.Printf("/")
		case TOK_LPAREN:
			fmt.Printf("(")
		case TOK_RPAREN:
			fmt.Printf(")")
		case TOK_IDENT:
			fmt.Print(tok.Name)
		case TOK_ASSIGN:
			fmt.Printf("=")
		case TOK_EOF:
			fmt.Println("")
		default:
			println(tok.Type)
			fmt.Printf("UNKNOWN ")
		}
	}
}
