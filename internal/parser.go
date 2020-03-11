package internal

import (
	"fmt"
	"github.com/unrotten/graphql/errors"
	"github.com/unrotten/graphql/internal/ast"
	"github.com/unrotten/graphql/internal/token"
	"text/scanner"
)

func Parse(source string) (*ast.Document, *errors.GraphQLError) {
	l := NewLexer(source, false)

	var doc *ast.Document
	err := l.catchSyntaxError(func() {
		doc = parseDocument(l)
	})
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func parseDocument(l *lexer) *ast.Document {
	doc := &ast.Document{}
	l.skipWhitespace()
	for l.peek() != token.EOF {
		if l.peek() == token.BRACE_L {
			op := &ast.OperationDefinition{Type: ast.Query, Loc: l.location()}
			op.SelectionSet = parseSelectionSet(l)
			doc.Definition = append(doc.Definition, op)
			continue
		}

		loc := l.location()
		switch name := parseName(l); name.Name {
		case "query":
			definition := parseOperationDefinition(l, ast.Query)
			definition.Loc = loc
			doc.Definition = append(doc.Definition, definition)
		case "mutation":
			doc.Definition = append(doc.Definition, parseOperationDefinition(l, ast.Mutation))
		case "subscription":
			doc.Definition = append(doc.Definition, parseOperationDefinition(l, ast.Mutation))
		case "fragment":
			fragment := parseFragmentDefinition(l)
			fragment.Loc = loc
			doc.Definition = append(doc.Definition, fragment)
		default:
			l.SyntaxError(fmt.Sprintf(`unexpected %q, expecting "fragment"`, name.Name))
		}
	}
	return doc
}

/**
 * FragmentDefinition :
 *   - fragment FragmentName on TypeCondition Directives? SelectionSet
 *
 * TypeCondition : NamedType
 */
func parseFragmentDefinition(l *lexer) *ast.FragmentDefinition {
	name := parseName(l)
	l.advanceKeyWord("on")
	typeCondition := parseNamed(l)
	directives := parseDirectives(l)
	selectionSet := parseSelectionSet(l)
	return &ast.FragmentDefinition{
		Name:          name,
		TypeCondition: typeCondition,
		Directives:    directives,
		SelectionSet:  selectionSet,
	}
}

func parseOperationDefinition(l *lexer, opType ast.OperationType) *ast.OperationDefinition {
	operationDefinition := &ast.OperationDefinition{Type: opType}
	if l.peek() == token.NAME {
		operationDefinition.Name = parseName(l)
	}
	operationDefinition.Vars = parseVariableDefinitions(l)
	operationDefinition.Directives = parseDirectives(l)
	operationDefinition.SelectionSet = parseSelectionSet(l)
	return operationDefinition
}

/**
 * VariableDefinitions : ( VariableDefinition+ )
 */
func parseVariableDefinitions(l *lexer) []*ast.VariableDefinition {
	var vars []*ast.VariableDefinition
	if l.peek() == token.PAREN_L {
		vars = append(vars, parseVariableDefinition(l))
	}
	return vars
}

/**
 * VariableDefinition : Variable : Type DefaultValue?
 */
func parseVariableDefinition(l *lexer) *ast.VariableDefinition {
	loc := l.location()
	variable := parseVariable(l)
	l.advance(token.COLON)
	t := parseType(l)
	var defaultValue ast.Value
	if l.peek() == token.EQUALS {
		l.advance(token.EQUALS)
		defaultValue = parseValueLiteral(l, true)
	}
	return &ast.VariableDefinition{
		Var:          variable,
		Type:         t,
		DefaultValue: defaultValue,
		Loc:          loc,
	}
}

/**
 * Type :
 *   - NamedType
 *   - ListType
 *   - NonNullType
 */
func parseType(l *lexer) ast.Type {
	loc := l.location()
	var t ast.Type
	switch l.peek() {
	case token.BRACKET_L:
		t = parseType(l)
		fallthrough
	case token.BRACKET_R:
		t = &ast.List{t, loc}
	case token.NAME:
		t = parseNamed(l)
	}
	if l.peek() == token.BANG {
		l.advance(token.BANG)
		return &ast.NonNull{
			Type: t,
			Loc:  loc,
		}
	}
	return t
}

// Converts a name lex token into a name parse node.
func parseName(l *lexer) *ast.Name {
	loc := l.location()
	name := l.scan.TokenText()
	l.advance(token.NAME)
	return &ast.Name{Name: name, Loc: loc}
}

/**
 * NamedType : Name
 */
func parseNamed(l *lexer) *ast.Named {
	loc := l.location()
	return &ast.Named{Name: parseName(l), Loc: loc}
}

/**
 * SelectionSet : { Selection+ }
 */
func parseSelectionSet(l *lexer) *ast.SelectionSet {
	var selections []ast.Selection
	loc := l.location()
	l.advance(token.BRACE_L)
	for l.peek() != token.BRACE_R {
		selections = append(selections, parseSelection(l))
	}
	l.advance(token.BRACE_R)
	return &ast.SelectionSet{
		Selections: selections,
		Loc:        loc,
	}
}

/**
 * Selection :
 *   - Field
 *   - FragmentSpread
 *   - InlineFragment
 */
func parseSelection(l *lexer) ast.Selection {
	if l.peek() == token.SPREAD {
		return parseFragment(l)
	}
	return parseField(l)
}

/**
 * Arguments : ( Argument+ )
 */
func parseArguments(l *lexer) []*ast.Argument {
	var args []*ast.Argument
	l.advance(token.PAREN_L)
	for l.peek() != token.PAREN_R {
		loc := l.location()
		name := parseName(l)
		l.advance(token.COLON)
		value := parseValueLiteral(l, false)
		args = append(args, &ast.Argument{Name: name, Value: value, Loc: loc})
	}
	l.advance(token.PAREN_R)
	return args
}

/**
 * Value[Const] :
 *   - [~Const] Variable
 *   - IntValue
 *   - FloatValue
 *   - StringValue
 *   - BooleanValue
 *   - EnumValue
 *   - ListValue[?Const]
 *   - ObjectValue[?Const]
 *
 * BooleanValue : one of `true` `false`
 *
 * EnumValue : Name but not `true`, `false` or `null`
 */
func parseValueLiteral(l *lexer, constOnly bool) ast.Value {
	loc := l.location()
	switch l.peek() {
	case token.BRACKET_L:
		return parseList(l, constOnly)
	case token.BRACE_L:
		return parseObject(l, constOnly)
	case token.DOLLAR:
		if !constOnly {
			return parseVariable(l)
		}
	case token.INT:
		return &ast.IntValue{Value: l.scan.TokenText(), Loc: loc}
	case token.FLOAT:
		return &ast.FloatValue{Value: l.scan.TokenText(), Loc: loc}
	case token.STRING:
		return &ast.StringValue{Value: l.scan.TokenText(), Loc: loc}
	case token.NAME:
		tokenText := l.scan.TokenText()
		if tokenText == "true" || tokenText == "false" {
			value := false
			if tokenText == "true" {
				value = true
			}
			return &ast.BooleanValue{Value: value, Loc: loc}
		} else if tokenText != "null" {
			return &ast.EnumValue{Value: tokenText, Loc: loc}
		}
	}
	panic(syntaxError(fmt.Sprintf("Unexpected %q", scanner.TokenString(l.peek()))))
}

/**
 * ListValue[Const] :
 *   - [ ]
 *   - [ Value[?Const]+ ]
 */
func parseList(l *lexer, constOnly bool) *ast.ListValue {
	loc := l.location()
	var list []ast.Value
	for l.peek() != token.BRACKET_R {
		list = append(list, parseValueLiteral(l, constOnly))
	}
	l.advance(token.BRACKET_R)
	return &ast.ListValue{Values: list, Loc: loc}
}

/**
 * ObjectValue[Const] :
 *   - { }
 *   - { ObjectField[?Const]+ }
 */
func parseObject(l *lexer, constOnly bool) *ast.ObjectValue {
	loc := l.location()
	l.advance(token.BRACE_L)
	var fields []*ast.ObjectField
	for l.peek() != token.BRACE_R {
		fields = append(fields, parseObjectField(l, constOnly))
	}
	l.advance(token.BRACE_R)
	return &ast.ObjectValue{Fields: fields, Loc: loc}
}

/**
 * ObjectField[Const] : Name : Value[?Const]
 */
func parseObjectField(l *lexer, constOnly bool) *ast.ObjectField {
	loc := l.location()
	name := parseNamed(l)
	l.advance(token.COLON)
	value := parseValueLiteral(l, constOnly)
	return &ast.ObjectField{Name: name, Value: value, Loc: loc}
}

/**
 * Variable : $ Name
 */
func parseVariable(l *lexer) *ast.Variable {
	loc := l.location()
	l.advance(token.DOLLAR)
	return &ast.Variable{Name: parseName(l), Loc: loc}
}

/**
 * Field : Alias? Name Arguments? Directives? SelectionSet?
 *
 * Alias : Name :
 */
func parseField(l *lexer) *ast.Field {
	field := &ast.Field{}
	field.Alias = parseName(l)
	field.Name = field.Alias
	if l.peek() == token.COLON {
		l.advance(token.COLON)
		field.Name = parseName(l)
	}
	if l.peek() == token.PAREN_L {
		field.Arguments = parseArguments(l)
	}
	field.Directives = parseDirectives(l)
	if l.peek() == token.BRACE_L {
		field.Loc = l.location()
		field.SelectionSet = parseSelectionSet(l)
	}
	return field
}

/**
 * Corresponds to both FragmentSpread and InlineFragment in the spec.
 *
 * FragmentSpread : ... FragmentName Directives?
 *
 * InlineFragment : ... TypeCondition? Directives? SelectionSet
 */
func parseFragment(l *lexer) ast.Selection {
	loc := l.location()
	l.advance(token.SPREAD)
	l.advance(token.SPREAD)
	l.advance(token.SPREAD)

	fragment := &ast.InlineFragment{Loc: loc}
	if l.peek() == token.NAME {
		name := parseName(l)
		if name.Name != "on" {
			spread := &ast.FragmentSpread{
				Name: name,
				Loc:  loc,
			}
			spread.Directives = parseDirectives(l)
			return spread
		}
		fragment.TypeCondition = parseNamed(l)
	}
	fragment.Directives = parseDirectives(l)
	fragment.SelectionSet = parseSelectionSet(l)
	return fragment
}

/**
 * Directives : Directive+
 */
func parseDirectives(l *lexer) []*ast.Directive {
	var directives []*ast.Directive
	for l.peek() == token.AT {
		directives = append(directives, parseDirective(l))
	}
	return directives
}

/**
 * Directive : @ Name Arguments?
 */
func parseDirective(l *lexer) *ast.Directive {
	loc := l.location()
	l.advance(token.AT)
	directive := &ast.Directive{}
	directive.Name = parseName(l)
	directive.Name.Loc.Column--
	directive.Loc = loc
	if l.peek() == token.PAREN_L {
		directive.Args = parseArguments(l)
	}
	return directive
}
