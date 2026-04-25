package mame

import (
	"fmt"
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
	TOK_MOD
	TOK_LPAREN
	TOK_RPAREN
	TOK_IDENT
	TOK_ASSIGN
	TOK_EQ
	TOK_NE
	TOK_LT
	TOK_LE
	TOK_GT
	TOK_GE
	TOK_SEMI
	TOK_ARG
	TOK_EOF
)

type Token struct {
	Type  TokenType
	Value int
	Name  string
}

func (c *Compiler) Lex() {
	c.tokens = make([]Token, 0)
	c.pos = 0

	for c.pos < len(c.input) {
		ch := c.input[c.pos]
		if ch == '\n' {
			c.tokens = append(c.tokens, Token{Type: TOK_SEMI})
			c.pos++
			continue
		}
		if unicode.IsSpace(rune(ch)) {
			c.pos++
			continue
		}
		if unicode.IsDigit(rune(ch)) {
			value := 0
			for c.pos < len(c.input) && unicode.IsDigit(rune(c.input[c.pos])) {
				value = value*10 + int(c.input[c.pos]-'0')
				c.pos++
			}
			c.tokens = append(c.tokens, Token{Type: TOK_NUM, Value: value})
			continue
		}
		if unicode.IsLetter(rune(ch)) {
			var name strings.Builder
			for c.pos < len(c.input) && unicode.IsLetter(rune(c.input[c.pos])) {
				name.WriteByte(c.input[c.pos])
				c.pos++
			}
			c.tokens = append(c.tokens, Token{Type: TOK_IDENT, Name: name.String()})
			continue
		}
		if ch == '+' {
			c.tokens = append(c.tokens, Token{Type: TOK_PLUS})
			c.pos++
			continue
		}
		if ch == '-' {
			if c.pos+1 < len(c.input) && unicode.IsDigit(rune(c.input[c.pos+1])) {
				canNeg := c.pos == 0
				if !canNeg {
					prev := c.input[c.pos-1]
					canNeg = !unicode.IsDigit(rune(prev)) && !unicode.IsLetter(rune(prev)) && prev != ')' && prev != '-'
				}
				if canNeg {
					c.pos++
					value := 0
					for c.pos < len(c.input) && unicode.IsDigit(rune(c.input[c.pos])) {
						value = value*10 + int(c.input[c.pos]-'0')
						c.pos++
					}
					c.tokens = append(c.tokens, Token{Type: TOK_NUM, Value: -value})
					continue
				}
			}
			c.tokens = append(c.tokens, Token{Type: TOK_MINUS})
			c.pos++
			continue
		}
		if ch == '*' {
			c.tokens = append(c.tokens, Token{Type: TOK_MUL})
			c.pos++
			continue
		}
		if ch == '/' {
			c.tokens = append(c.tokens, Token{Type: TOK_DIV})
			c.pos++
			continue
		}
		if ch == '%' {
			c.tokens = append(c.tokens, Token{Type: TOK_MOD})
			c.pos++
			continue
		}
		if ch == '(' {
			c.tokens = append(c.tokens, Token{Type: TOK_LPAREN})
			c.pos++
			continue
		}
		if ch == ')' {
			c.tokens = append(c.tokens, Token{Type: TOK_RPAREN})
			c.pos++
			continue
		}
		if ch == '=' {
			if c.pos+1 < len(c.input) && c.input[c.pos+1] == '=' {
				c.tokens = append(c.tokens, Token{Type: TOK_EQ})
				c.pos += 2
				continue
			}
			c.tokens = append(c.tokens, Token{Type: TOK_ASSIGN})
			c.pos++
			continue
		}
		if ch == '!' && c.pos+1 < len(c.input) && c.input[c.pos+1] == '=' {
			c.tokens = append(c.tokens, Token{Type: TOK_NE})
			c.pos += 2
			continue
		}
		if ch == '<' {
			if c.pos+1 < len(c.input) && c.input[c.pos+1] == '=' {
				c.tokens = append(c.tokens, Token{Type: TOK_LE})
				c.pos += 2
				continue
			}
			c.tokens = append(c.tokens, Token{Type: TOK_LT})
			c.pos++
			continue
		}
		if ch == '>' {
			if c.pos+1 < len(c.input) && c.input[c.pos+1] == '=' {
				c.tokens = append(c.tokens, Token{Type: TOK_GE})
				c.pos += 2
				continue
			}
			c.tokens = append(c.tokens, Token{Type: TOK_GT})
			c.pos++
			continue
		}
		if ch == ';' {
			c.tokens = append(c.tokens, Token{Type: TOK_SEMI})
			c.pos++
			continue
		}
		if ch == '$' {
			c.pos++
			value := 0
			for c.pos < len(c.input) && unicode.IsDigit(rune(c.input[c.pos])) {
				value = value*10 + int(c.input[c.pos]-'0')
				c.pos++
			}
			c.tokens = append(c.tokens, Token{Type: TOK_ARG, Value: value})
			continue
		}
		panic(fmt.Sprintf("unknown char: %c", ch))
	}
	c.tokens = append(c.tokens, Token{Type: TOK_EOF})
}

func (c *Compiler) peek() Token {
	if c.tokenPos >= len(c.tokens) {
		return Token{Type: TOK_EOF}
	}
	return c.tokens[c.tokenPos]
}

func (c *Compiler) consume(typ TokenType) Token {
	if c.peek().Type == typ {
		tok := c.tokens[c.tokenPos]
		c.tokenPos++
		return tok
	}
	panic(fmt.Sprintf("expected token type %d", typ))
}
