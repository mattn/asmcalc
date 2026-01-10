package asmcalc

import (
	"fmt"
	"os"
	"strings"
	"unicode"
)

type TokenType int

const (
	TOK_NUM TokenType = iota
	TOK_PLUS
	TOK_MINUS
	TOK_MUL
	TOK_DIV
	TOK_LPAREN
	TOK_RPAREN
	TOK_IDENT
	TOK_ASSIGN
	TOK_EOF
)

type Token struct {
	Type  TokenType
	Value int
	Name  string
}

type Compiler struct {
	input    string
	pos      int
	tokens   []Token
	tokenPos int
}

func lex(input []byte) ([]Token, error) {
	tokens := make([]Token, 0)
	pos := 0

	for pos < len(input) {
		ch := input[pos]
		if unicode.IsSpace(rune(ch)) {
			pos++
			continue
		}
		if unicode.IsDigit(rune(ch)) {
			value := 0
			for pos < len(input) && unicode.IsDigit(rune(input[pos])) {
				value = value*10 + int(input[pos]-'0')
				pos++
			}
			tokens = append(tokens, Token{Type: TOK_NUM, Value: value})
			continue
		}
		if unicode.IsLetter(rune(ch)) {
			var name strings.Builder
			for pos < len(input) && unicode.IsLetter(rune(input[pos])) {
				name.WriteByte(input[pos])
				pos++
			}
			nameStr := name.String()
			if nameStr == "set" {
				tokens = append(tokens, Token{Type: TOK_ASSIGN})
			} else {
				tokens = append(tokens, Token{Type: TOK_IDENT, Name: nameStr})
			}
			continue
		}
		if ch == '+' {
			tokens = append(tokens, Token{Type: TOK_PLUS})
			pos++
			continue
		}
		if ch == '-' {
			tokens = append(tokens, Token{Type: TOK_MINUS})
			pos++
			continue
		}
		if ch == '*' {
			tokens = append(tokens, Token{Type: TOK_MUL})
			pos++
			continue
		}
		if ch == '/' {
			tokens = append(tokens, Token{Type: TOK_DIV})
			pos++
			continue
		}
		if ch == '(' {
			tokens = append(tokens, Token{Type: TOK_LPAREN})
			pos++
			continue
		}
		if ch == ')' {
			tokens = append(tokens, Token{Type: TOK_RPAREN})
			pos++
			continue
		}
		return nil, fmt.Errorf("unknown char: %c", ch)
	}
	tokens = append(tokens, Token{Type: TOK_EOF})

	return tokens, nil
}

func compile(input []byte) error {
	out := os.Stdout
	fmt.Fprintln(out, ".text")
	fmt.Fprintln(out, ".globl _start")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "_start:")

	return nil
}
