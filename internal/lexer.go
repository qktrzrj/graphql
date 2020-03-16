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

func (l *lexer) catchSyntaxError(fn func()) (graphQLError *errors.GraphQLError) {
	defer func() {
		if err := recover(); err != nil {
			if err, ok := err.(syntaxError); ok {
				graphQLError = errors.New("Syntax Error: %s", err)
				graphQLError.Locations = []errors.Location{l.location()}
				return
			}
			panic(err)
		}
	}()
	fn()
	return
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

func (l *lexer) skipComment() {
	if l.next != '#' {
		panic("consumeComment used in wrong context")
	}

	// TODO: count and trim whitespace so we can dedent any following lines.
	if l.scan.Peek() == ' ' {
		l.scan.Next()
	}

	if l.comment.Len() > 0 {
		l.comment.WriteRune('\n')
	}

	for {
		next := l.scan.Next()
		if next == '\r' || next == '\n' || next == scanner.EOF {
			break
		}
		l.comment.WriteRune(next)
	}
}

// If the next token is of the given kind, advance and skip whitespace.
// Otherwise, do not change the parser state and return error.
func (l *lexer) advance(expected rune) {
	if l.next != expected {
		found := strings.TrimPrefix(l.scan.TokenText(), `"`)
		found = strings.TrimSuffix(found, `"`)
		l.SyntaxError(fmt.Sprintf(`Expected %s, found %q.`, scanner.TokenString(expected), found))
	}
	l.skipWhitespace()
}

// If the next token is of the given kind, advance and skip whitespace.
// Otherwise, do not change the parser state and return error.
func (l *lexer) advanceKeyWord(keyword string) {
	if l.next != token.NAME || l.scan.TokenText() != keyword {
		found := strings.TrimPrefix(l.scan.TokenText(), `"`)
		found = strings.TrimSuffix(found, `"`)
		l.SyntaxError(fmt.Sprintf(`Expected "%s", found %q.`, keyword, found))
	}
	l.skipWhitespace()
}

func (l *lexer) SyntaxError(message string) {
	panic(syntaxError(message))
}
