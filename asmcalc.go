package asmcalc

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
