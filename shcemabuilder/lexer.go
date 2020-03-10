package shcemabuilder

import (
	"bytes"
	"github.com/unrotten/graphql/errors"
	"strings"
	"text/scanner"
)

type syntaxError string

type lexer struct {
	scan                  *scanner.Scanner
	next                  rune
	comment               bytes.Buffer
	useStringDescriptions bool
}

type Ident struct {
	Name string
	Loc  errors.Location
}

func NewLexer(source string, useStringDescriptions ...bool) *lexer {
	scan := &scanner.Scanner{
		Mode: scanner.ScanIdents | scanner.ScanInts | scanner.ScanFloats | scanner.ScanStrings,
	}
	scan.Init(strings.NewReader(source))

	if len(useStringDescriptions) > 0 {
		return &lexer{scan: scan, useStringDescriptions: useStringDescriptions[0]}
	}
	return &lexer{scan: scan}
}

func (l *lexer) catchSyntaxError(fn func()) *errors.GraphQLError {
	var graphQLError *errors.GraphQLError
	defer func() {
		if err := recover(); err != nil {
			if err, ok := err.(syntaxError); ok {
				graphQLError = errors.New("syntax error: %s", err)
				graphQLError.Locations = []errors.Location{l.Location()}
				return
			}
			panic(err)
		}
	}()
	fn()
	return graphQLError
}

func (l *lexer) peek() rune {
	return l.next
}

// skip whitespace, also tab, commas, BOM and comments
func (l *lexer) ConsumeWhitespace() {
	l.comment.Reset()
	for {
		l.next = l.scan.Scan()

		if l.next == ',' {
			continue
		}

		if l.next == '#' {
			l.consumeComment()
			continue
		}
		break
	}
}
