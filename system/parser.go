package system

import (
	"fmt"
	"github.com/shyptr/graphql/errors"
	"github.com/shyptr/graphql/system/ast"
	"github.com/shyptr/graphql/system/kinds"
	"github.com/shyptr/graphql/system/token"
	"strconv"
	"strings"
	"text/scanner"
)

func Parse(source string) (*Document, *errors.GraphQLError) {
	doc, err := ParseDocument(source)
	if err != nil {
		return nil, err
	}
	var operations []*ast.OperationDefinition
	var fragments []*ast.FragmentDefinition
	for _, definition := range doc.Definition {
		switch o := definition.(type) {
		case *ast.OperationDefinition:
			operations = append(operations, o)
		case *ast.FragmentDefinition:
			fragments = append(fragments, o)
		}
	}
	return &Document{
		Operations: operations,
		Fragments:  fragments,
	}, nil
}

func ParseDocument(source string) (*ast.Document, *errors.GraphQLError) {
	if source == "" {
		return nil, errors.New("Must provide source. Received: undefined.")
	}
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
	doc := &ast.Document{Kind: kinds.Document, Loc: l.location()}
	l.SkipWhitespace()
	for l.peek() != token.EOF {
		if l.peek() == token.BRACE_L {
			op := &ast.OperationDefinition{Kind: kinds.OperationDefinition, Operation: ast.Query, Loc: l.location()}
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
			l.SyntaxError(fmt.Sprintf(`Unexpected %q.`, name.Name))
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
	name := parseFragmentName(l)
	l.advanceKeyWord("on")
	typeCondition := parseNamed(l)
	directives := parseDirectives(l)
	selectionSet := parseSelectionSet(l)
	return &ast.FragmentDefinition{
		Kind:          kinds.FragmentDefinition,
		Name:          name,
		TypeCondition: typeCondition,
		Directives:    directives,
		SelectionSet:  selectionSet,
	}
}

// Name : but not `on`
func parseFragmentName(l *lexer) *ast.Name {
	loc := l.location()
	name := l.scan.TokenText()
	if name == "on" {
		panic(syntaxError(`Unexpected Name "on".`))
	}
	l.advance(token.NAME)
	return &ast.Name{Kind: kinds.Name, Name: name, Loc: loc}
}

func parseOperationDefinition(l *lexer, opType ast.OperationType) *ast.OperationDefinition {
	operationDefinition := &ast.OperationDefinition{Kind: kinds.OperationDefinition, Operation: opType}
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
		l.advance(token.PAREN_L)
		for l.peek() != token.PAREN_R {
			vars = append(vars, parseVariableDefinition(l))
		}
		l.advance(token.PAREN_R)
	}
	return vars
}

/**
 * VariableDefinition : Variable : Operation DefaultValue?
 */
func parseVariableDefinition(l *lexer) *ast.VariableDefinition {
	loc := l.location()
	variable := parseVariable(l)
	l.advance(token.COLON)
	t := ParseType(l)
	var defaultValue ast.Value
	if l.peek() == token.EQUALS {
		l.advance(token.EQUALS)
		defaultValue = ParseValueLiteral(l, true)
	}
	var directives []*ast.Directive
	if l.peek() == token.AT {
		directives = parseDirectives(l)
	}
	return &ast.VariableDefinition{
		Kind:         kinds.VariableDefinition,
		Var:          variable,
		Type:         t,
		DefaultValue: defaultValue,
		Loc:          loc,
		Directives:   directives,
	}
}

/**
 * Operation :
 *   - NamedType
 *   - ListType
 *   - NonNullType
 */
func ParseType(l *lexer) ast.Type {
	loc := l.location()
	var t ast.Type
	switch l.peek() {
	case token.BRACKET_L:
		l.advance(token.BRACKET_L)
		t = ParseType(l)
		fallthrough
	case token.BRACKET_R:
		l.advance(token.BRACKET_R)
		t = &ast.List{Kind: kinds.List, Type: t, Loc: loc}
	case token.NAME:
		t = parseNamed(l)
	}
	if l.peek() == token.BANG {
		l.advance(token.BANG)
		return &ast.NonNull{
			Kind: kinds.NonNull,
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
	return &ast.Name{Kind: kinds.Name, Name: name, Loc: loc}
}

/**
 * NamedType : Name
 */
func parseNamed(l *lexer) *ast.Named {
	loc := l.location()
	return &ast.Named{Kind: kinds.Named, Name: parseName(l), Loc: loc}
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
		Kind:       kinds.SelectionSet,
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
		value := ParseValueLiteral(l, false)
		args = append(args, &ast.Argument{Kind: kinds.Argument, Name: name, Value: value, Loc: loc})
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
func ParseValueLiteral(l *lexer, constOnly bool) ast.Value {
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
		value := l.scan.TokenText()
		l.advance(token.INT)
		return &ast.IntValue{Kind: kinds.IntValue, Value: value, Loc: loc}
	case token.FLOAT:
		value := l.scan.TokenText()
		l.advance(token.FLOAT)
		return &ast.FloatValue{Kind: kinds.FloatValue, Value: value, Loc: loc}
	case token.STRING:
		value := l.scan.TokenText()
		value = strings.TrimPrefix(value, `"`)
		value = strings.TrimSuffix(value, `"`)
		l.advance(token.STRING)
		return &ast.StringValue{Kind: kinds.StringValue, Value: value, Loc: loc}
	case token.RAWSTRING:
		value := l.scan.TokenText()
		value = strings.TrimPrefix(value, "`")
		value = strings.TrimSuffix(value, "`")
		l.advance(token.RAWSTRING)
		return &ast.StringValue{Kind: kinds.StringValue, Value: value, Loc: loc}
	case token.NAME:
		tokenText := l.scan.TokenText()
		l.advance(token.NAME)
		if tokenText == "true" || tokenText == "false" {
			value := false
			if tokenText == "true" {
				value = true
			}
			return &ast.BooleanValue{Kind: kinds.BooleanValue, Value: value, Loc: loc}
		} else if tokenText == "null" {
			return &ast.NullValue{Kind: kinds.NullValue, Loc: loc}
		} else {
			return &ast.EnumValue{Kind: kinds.EnumValue, Value: tokenText, Loc: loc}
		}
	}
	panic(syntaxError(fmt.Sprintf("Unexpected %q.", scanner.TokenString(l.peek()))))
}

/**
 * ListValue[Const] :
 *   - [ ]
 *   - [ Value[?Const]+ ]
 */
func parseList(l *lexer, constOnly bool) *ast.ListValue {
	loc := l.location()
	var list []ast.Value
	l.advance(token.BRACKET_L)
	for l.peek() != token.BRACKET_R {
		list = append(list, ParseValueLiteral(l, constOnly))
	}
	l.advance(token.BRACKET_R)
	return &ast.ListValue{Kind: kinds.ListValue, Values: list, Loc: loc}
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
	return &ast.ObjectValue{Kind: kinds.ObjectValue, Fields: fields, Loc: loc}
}

/**
 * ObjectField[Const] : Name : Value[?Const]
 */
func parseObjectField(l *lexer, constOnly bool) *ast.ObjectField {
	loc := l.location()
	name := parseNamed(l)
	l.advance(token.COLON)
	value := ParseValueLiteral(l, constOnly)
	return &ast.ObjectField{Kind: kinds.ObjectField, Name: name, Value: value, Loc: loc}
}

/**
 * Variable : $ Name
 */
func parseVariable(l *lexer) *ast.Variable {
	loc := l.location()
	l.advance(token.DOLLAR)
	return &ast.Variable{Kind: kinds.Variable, Name: parseName(l), Loc: loc}
}

/**
 * Field : Alias? Name Arguments? Directives? SelectionSet?
 *
 * Alias : Name :
 */
func parseField(l *lexer) *ast.Field {
	field := &ast.Field{Kind: kinds.Field, Loc: l.location()}
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

	fragment := &ast.InlineFragment{Kind: kinds.InlineFragment, Loc: loc}
	if l.peek() == token.NAME {
		name := parseName(l)
		if name.Name != "on" {
			spread := &ast.FragmentSpread{
				Kind: kinds.FragmentSpread,
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
	directive := &ast.Directive{Kind: kinds.Directive}
	directive.Name = parseName(l)
	directive.Name.Loc.Column--
	directive.Loc = loc
	if l.peek() == token.PAREN_L {
		directive.Args = parseArguments(l)
	}
	return directive
}

func ValueToJson(value ast.Value, vars map[string]interface{}) (interface{}, *errors.GraphQLError) {
	switch value := value.(type) {
	case *ast.IntValue:
		v, err := strconv.ParseInt(value.Value, 10, 64)
		if err != nil {
			return nil, errors.New("bad int arg: %s", err)
		}
		return float64(v), nil
	case *ast.FloatValue:
		v, err := strconv.ParseFloat(value.Value, 64)
		if err != nil {
			return nil, errors.New("bad float arg: %s", err)
		}
		return v, nil
	case *ast.StringValue:
		return value.Value, nil
	case *ast.NullValue:
		return nil, nil
	case *ast.BooleanValue:
		return value.Value, nil
	case *ast.EnumValue:
		return value.Value, nil
	case *ast.Variable:
		actual, ok := vars[value.Name.Name]
		if !ok {
			return nil, nil
		}
		return actual, nil
	case *ast.ObjectValue:
		obj := make(map[string]interface{})
		for _, field := range value.Fields {
			name := field.Name.Name.Name
			if _, found := obj[name]; found {
				return nil, errors.New("duplicate field")
			}
			value, err := ValueToJson(field.Value, vars)
			if err != nil {
				return nil, err
			}
			obj[name] = value
		}
		return obj, nil
	case *ast.ListValue:
		list := make([]interface{}, 0, len(value.Values))
		for _, item := range value.Values {
			value, err := ValueToJson(item, vars)
			if err != nil {
				return nil, err
			}
			list = append(list, value)
		}
		return list, nil
	default:
		return nil, errors.New("unsupported value type: %s", value.GetKind())
	}
}
