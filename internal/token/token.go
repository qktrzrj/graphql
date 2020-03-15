package token

import "text/scanner"

const (
	EOF       = scanner.EOF
	BANG      = '!'
	DOLLAR    = '$'
	PAREN_L   = '('
	PAREN_R   = ')'
	SPREAD    = '.'
	COLON     = ':'
	EQUALS    = '='
	AT        = '@'
	BRACKET_L = '['
	BRACKET_R = ']'
	BRACE_L   = '{'
	PIPE      = '|'
	BRACE_R   = '}'
	NAME      = scanner.Ident
	INT       = scanner.Int
	FLOAT     = scanner.Float
	STRING    = scanner.String
	RAWSTRING = scanner.RawString
	AMP       = '&'
)

// NAME -> keyword relationship
const (
	FRAGMENT     = "fragment"
	QUERY        = "query"
	MUTATION     = "mutation"
	SUBSCRIPTION = "subscription"
	SCHEMA       = "schema"
	SCALAR       = "scalar"
	TYPE         = "type"
	INTERFACE    = "interface"
	UNION        = "union"
	ENUM         = "enum"
	INPUT        = "input"
	EXTEND       = "extend"
	DIRECTIVE    = "directive"
)
