package internal

import (
	"bytes"
	"fmt"
	"github.com/unrotten/graphql/errors"
	"github.com/unrotten/graphql/internal/token"
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
				graphQLError.Locations = []errors.Location{l.location()}
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

func (l *lexer) location() errors.Location {
	return errors.Location{
		Line:   l.scan.Line,
		Column: l.scan.Column,
	}
}

// skip whitespace, also tab, commas, BOM and comments
func (l *lexer) skipWhitespace() {
	l.comment.Reset()
	for {
		l.next = l.scan.Scan()

		if l.next == ',' {
			continue
		}

		if l.next == '#' {
			l.skipComment()
			continue
		}
		break
	}
}

// If the next token is of the given kind, advance and skip whitespace.
// Otherwise, do not change the parser state and return error.
func (l *lexer) advance(expected rune) {
	if l.next != expected {
		l.SyntaxError(fmt.Sprintf("unexpected %q, expecting %s", l.scan.TokenText(), scanner.TokenString(expected)))
	}
	l.skipWhitespace()
}

// If the next token is of the given kind, advance and skip whitespace.
// Otherwise, do not change the parser state and return error.
func (l *lexer) advanceKeyWord(keyword string) {
	if l.next != token.NAME || l.scan.TokenText() != keyword {
		l.SyntaxError(fmt.Sprintf("unexpected %q, expecting %q", l.scan.TokenText(), keyword))
	}
	l.skipWhitespace()
}

func (l *lexer) SyntaxError(message string) {
	panic(syntaxError(message))
}
