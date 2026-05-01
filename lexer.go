package mame

import (
	"fmt"
	"strings"
	"unicode"
)

//go:generate stringer -type=TokenType
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
	TOK_LBRACE
	TOK_RBRACE
	TOK_IDENT
	TOK_ASSIGN
	TOK_EQ
	TOK_NE
	TOK_LT
	TOK_LE
	TOK_GT
	TOK_GE
	TOK_SEMI
	TOK_STRING
	TOK_FNUM
	TOK_EOF
)

type Token struct {
	Type  TokenType
	Value int
	Float float64
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
		if ch == '#' {
			for c.pos < len(c.input) && c.input[c.pos] != '\n' {
				c.pos++
			}
			continue
		}
		if unicode.IsDigit(rune(ch)) {
			c.tokens = append(c.tokens, c.lexNumber(false))
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
					c.tokens = append(c.tokens, c.lexNumber(true))
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
		if ch == '{' {
			c.tokens = append(c.tokens, Token{Type: TOK_LBRACE})
			c.pos++
			continue
		}
		if ch == '}' {
			c.tokens = append(c.tokens, Token{Type: TOK_RBRACE})
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
		if ch == '"' {
			c.pos++
			var sb strings.Builder
			for c.pos < len(c.input) && c.input[c.pos] != '"' {
				if c.input[c.pos] == '\\' && c.pos+1 < len(c.input) {
					switch c.input[c.pos+1] {
					case 'n':
						sb.WriteByte('\n')
					case 't':
						sb.WriteByte('\t')
					case '\\':
						sb.WriteByte('\\')
					case '"':
						sb.WriteByte('"')
					default:
						panic(fmt.Sprintf("unknown escape: \\%c", c.input[c.pos+1]))
					}
					c.pos += 2
					continue
				}
				sb.WriteByte(c.input[c.pos])
				c.pos++
			}
			if c.pos >= len(c.input) {
				panic("unterminated string")
			}
			c.pos++
			c.tokens = append(c.tokens, Token{Type: TOK_STRING, Name: sb.String()})
			continue
		}
		panic(fmt.Sprintf("unknown char: %c", ch))
	}
	c.tokens = append(c.tokens, Token{Type: TOK_EOF})
}

// lexNumber reads digits and, if followed by `.<digits>`, a fractional part.
// Returns TOK_NUM or TOK_FNUM. neg flips the sign of the parsed value.
func (c *Compiler) lexNumber(neg bool) Token {
	value := 0
	for c.pos < len(c.input) && unicode.IsDigit(rune(c.input[c.pos])) {
		value = value*10 + int(c.input[c.pos]-'0')
		c.pos++
	}
	if c.pos+1 < len(c.input) && c.input[c.pos] == '.' && unicode.IsDigit(rune(c.input[c.pos+1])) {
		c.pos++
		f := float64(value)
		scale := 1.0
		for c.pos < len(c.input) && unicode.IsDigit(rune(c.input[c.pos])) {
			scale /= 10
			f += float64(c.input[c.pos]-'0') * scale
			c.pos++
		}
		if neg {
			f = -f
		}
		return Token{Type: TOK_FNUM, Float: f}
	}
	if neg {
		value = -value
	}
	return Token{Type: TOK_NUM, Value: value}
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
	panic(fmt.Sprintf("expected %s, got %s", typ, c.peek().Type))
}
