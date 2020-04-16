package schemabuilder

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/shyptr/graphql/internal"
	"reflect"
)

// Connection conforms to the GraphQL Connection type in the Relay Pagination spec.
type Connection struct {
	TotalCount int      `graphql:"totalCount"`
	Edges      []Edge   `graphql:"edges"`
	PageInfo   PageInfo `graphql:"pageInfo"`
}

// PageInfo contains information for pagination on a connection type. The list of Pages is used for
// page-number based pagination where the ith index corresponds to the start cursor of (i+1)st page.
type PageInfo struct {
	HasNextPage bool     `graphql:"hasNextPage"`
	EndCursor   *string  `graphql:"endCursor"`
	HasPrevPage bool     `graphql:"hasPrevPage"`
	StartCursor *string  `graphql:"startCursor"`
	Pages       []string `graphql:"pages"`
}

// Edge consists of a node paired with its b64 encoded cursor.
type Edge struct {
	Node   interface{} `graphql:"node"`
	Cursor string      `graphql:"cursor"`
}

// ConnectionArgs conform to the pagination arguments as specified by the Relay Spec for Connection
// types. https://facebook.github.io/relay/graphql/connections.htm#sec-Arguments
type ConnectionArgs struct {
	// first: n
	First *int64 `graphql:"first"`
	// last: n
	Last *int64 `graphql:"last"`
	// after: cursor
	After *string `graphql:"after"`
	// before: cursor
	Before *string `graphql:"before"`
}

func (p ConnectionArgs) limit() int {
	if p.First != nil {
		return int(*p.First)
	}
	if p.Last != nil {
		return int(*p.Last)
	}
	return 0
}

// PaginationInfo can be returned in a PaginateFieldFunc. The TotalCount function returns the
// totalCount field on the connection Fn. If the resolver makes a SQL Query, then HasNextPage and
// HasPrevPage can be resolved in an efficient manner by requesting first/last:n + 1 items in the
// query. Then the flags can be filled in by checking the result size.
type PaginationInfo struct {
	TotalCount  int
	HasNextPage bool
	HasPrevPage bool
	Pages       []string
}

const PREFIX = "arrayconnection:"

var (
	connectionArgsType = reflect.TypeOf(&ConnectionArgs{})
	paginationInfoType = reflect.TypeOf(&PaginationInfo{})
	pageInfoType       = reflect.TypeOf(&PageInfo{})
)

var relayKey map[reflect.Type]string

func RelayKey(typ interface{}, key string) {
	value := reflect.ValueOf(typ)
	if value.Kind() != reflect.Struct {
		panic("relay key must be struct")
	}
	if relayKey == nil {
		relayKey = make(map[reflect.Type]string)
	}
	relayKey[value.Type()] = key
}

var RelayConnection afterBuildFunc = func(param buildParam) error {
	sb, field, fctx, fnresolve := param.sb, param.f, param.functx, param.fnresolve
	var argAnonymous, retAnonymous bool
	if fctx.hasArg {
		if cf, ok := fctx.argTyp.FieldByName(connectionArgsType.Elem().Name()); ok && cf.Type == connectionArgsType && cf.Anonymous {
			argAnonymous = true
			fnresolve.handleChain = append(fnresolve.handleChain, relayParseArg(true))
			if !fctx.hasRet {
				return fmt.Errorf("if you use ConnectionArgs in your arg with anonymous,then you must return something")
			}
			ret := fctx.funcType.Out(0)
			if ret.Kind() != reflect.Ptr || ret.Elem().Kind() != reflect.Struct {
				return fmt.Errorf("if return Pagination in your object, then your object type must be a ptr of struct")
			}
			ret = ret.Elem()
			if pf, ok := ret.FieldByName(paginationInfoType.Elem().Name()); ok && pf.Type == paginationInfoType && pf.Anonymous {
				retAnonymous = true
				if ret.NumField() != 2 {
					return fmt.Errorf("for reutrn Pagination, your struct should have 2 field,one of pagination,and another is slice")
				}
				fnresolve.executeChain = append(fnresolve.executeChain, relayParseResult(true))
			} else {
				return fmt.Errorf("if you use ConnectionArgs in your arg with anonymous,then you must return PaginationInfo in your return object")
			}
		}
	}

	var connectionArgs *internal.InputObject
	if argAnonymous {
		connectionArgs = field.Args[connectionArgsType.Elem().Name()].Type.(*internal.InputObject)
		delete(field.Args, connectionArgsType.Elem().Name())
	} else {
		fnresolve.handleChain = append(fnresolve.handleChain, relayParseArg(false))
		fnresolve.executeChain = append(fnresolve.executeChain, relayParseResult(false))
		argType, err := sb.getType(connectionArgsType)
		if err != nil {
			return err
		}
		connectionArgs = argType.(*internal.InputObject)
	}
	for n, f := range connectionArgs.Fields {
		field.Args[n] = f
	}

	var sliceType *internal.List
	if retAnonymous {
		retobj := field.Type.(*internal.Object)
		delete(retobj.Fields, paginationInfoType.Elem().Name())
		for _, f := range retobj.Fields {
			if _, ok := f.Type.(*internal.List); !ok {
				return fmt.Errorf("for pagination reutrn,another field should be slice")
			}
			sliceType = f.Type.(*internal.List)
		}
	} else {
		if _, ok := field.Type.(*internal.List); !ok {
			return fmt.Errorf("for pagination reutrn should be slice")
		}
		sliceType = field.Type.(*internal.List)
	}

	if sliceType == nil {
		return fmt.Errorf("must return slice for relay")
	}

	object, err := validateSliceType(sliceType)
	if err != nil {
		return err
	}

	if _, ok := relayKey[reflect.TypeOf(object.IsTypeOf)]; !ok {
		return fmt.Errorf("%s don't have a relay key", object.Name)
	}

	return buildConnectionType(object.Name, sb, sliceType, fctx, field)
}

func validateSliceType(typ internal.Type) (*internal.Object, error) {
	switch typ := typ.(type) {
	case *internal.List:
		return validateSliceType(typ.Type)
	case *internal.NonNull:
		return validateSliceType(typ.Type)
	case *internal.Object:
		return typ, nil
	default:
		return nil, fmt.Errorf("relay slice elem must be object")
	}
}

func buildConnectionType(name string, sb *schemaBuilder, sliceType *internal.List, fctx *funcContext, field *internal.Field) error {
	fieldMap := make(map[string]*internal.Field)

	countType, _ := reflect.TypeOf(Connection{}).FieldByName("TotalCount")
	countField, err := sb.buildField(countType)
	if err != nil {
		return err
	}
	fieldMap["totalCount"] = countField
	nodeField := &internal.Field{
		Name: "node",
		Type: sliceType.Type,
		Resolve: func(ctx context.Context, source, args interface{}) (interface{}, error) {
			if value, ok := source.(Edge); ok {
				return value.Node, nil
			}

			return nil, fmt.Errorf("error resolving node in edge")
		},
	}
	cursorType, _ := reflect.TypeOf(Edge{}).FieldByName("Cursor")
	cursorField, err := sb.buildField(cursorType)
	if err != nil {
		return err
	}
	fieldMap["edges"] = &internal.Field{
		Name: "edges",
		Type: &internal.List{Type: &internal.NonNull{Type: &internal.Object{
			Name: fmt.Sprintf("%sEdge", name),
			Fields: map[string]*internal.Field{
				"node":   nodeField,
				"cursor": cursorField,
			},
			IsTypeOf: Edge{},
		}}},
		Resolve: func(ctx context.Context, source, args interface{}) (interface{}, error) {
			if value, ok := source.(Connection); ok {
				return value.Edges, nil
			}
			return nil, fmt.Errorf("error resolving edges in connection")
		},
	}
	pageInfoType, _ := reflect.TypeOf(Connection{}).FieldByName("PageInfo")
	pageInfoField, err := sb.buildField(pageInfoType)

	if err != nil {
		return err
	}
	fieldMap["pageInfo"] = pageInfoField
	field.Type = &internal.NonNull{Type: &internal.Object{
		Name:     fmt.Sprintf("%sConnection", name),
		Fields:   fieldMap,
		IsTypeOf: reflect.New(fctx.typ).Interface(),
	}}
	return nil
}

func relayParseArg(anonymous bool) ExecuteFunc {

	return func(ctx context.Context, args, source interface{}) error {
		if args == nil {
			args = make(map[string]interface{})
		}
		if anonymous {
			argMap := args.(map[string]interface{})
			connectionMap := make(map[string]interface{})
			for n, i := range argMap {
				connectionMap[n] = i
			}
			argMap[connectionArgsType.Elem().Name()] = connectionMap
		}
		return nil
	}
}

func relayParseResult(anonymous bool) afterExecuteFunc {

	return func(param executeFuncParam) (interface{}, error) {
		_, _, args, source := param.sb, param.ctx, param.args.(map[string]interface{}), param.source
		var (
			paginationArgs *ConnectionArgs
			result         interface{}
			paginationInfo *PaginationInfo
		)
		convert, err := Convert(args, connectionArgsType)
		if err != nil {
			return nil, err
		}
		paginationArgs = convert.(*ConnectionArgs)
		if anonymous {
			value := reflect.ValueOf(source).Elem()
			for i := 0; i < value.NumField(); i++ {
				field := value.Field(i)
				if field.Type() == paginationInfoType {
					paginationInfo = field.Interface().(*PaginationInfo)
					continue
				}
				result = field.Interface()
			}
		} else {
			result = source
		}
		return buildConnection(anonymous, paginationArgs, paginationInfo, result)
	}
}

func buildConnection(anonymous bool, paginationArgs *ConnectionArgs, paginationInfo *PaginationInfo, result interface{}) (Connection, error) {
	if result == nil {
		return Connection{}, nil
	}
	resultVal := reflect.ValueOf(result)
	if resultVal.Len() == 0 {
		return Connection{}, nil
	}
	var (
		edges []Edge
		pages []string
	)
	limit := paginationArgs.limit()
	for i := 0; i < resultVal.Len(); i++ {
		value := resultVal.Index(i)
		for value.Kind() == reflect.Ptr {
			value = value.Elem()
		}
		var cursor string
		if key, ok := relayKey[value.Type()]; ok {
			cursor = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%v%v", PREFIX, GetField(value, key).Interface())))
		} else {
			return Connection{}, fmt.Errorf("must provide key for relay")
		}
		edges = append(edges, Edge{Node: value.Interface(), Cursor: cursor})
		// The blank cursor indicates the initial page.
		if i == 0 {
			pages = append(pages, "")
		}
		// Limit at zero means infinite / no pages.
		if limit == 0 {
			continue
		}
		// The last cursor can't be followed by another page because there are no more entries.
		if i == resultVal.Len()-1 {
			continue
		}
		// If the next cursor is the start cursor of a page then push the current cursor
		// to the list.
		if (i+1)%limit == 0 {
			pages = append(pages, edges[i].Cursor)
		}
	}

	connection := Connection{
		TotalCount: len(edges),
		Edges:      edges,
		PageInfo:   PageInfo{Pages: pages},
	}
	return setConnection(anonymous, paginationArgs, paginationInfo, connection)
}

func setConnection(anonymous bool, paginationArgs *ConnectionArgs, paginationInfo *PaginationInfo, connection Connection) (Connection, error) {
	if anonymous {
		connection.PageInfo.HasNextPage = paginationInfo.HasNextPage
		connection.PageInfo.HasPrevPage = paginationInfo.HasPrevPage
		connection.TotalCount = paginationInfo.TotalCount
		connection.PageInfo.Pages = paginationInfo.Pages
	} else {
		if err := paginateManually(&connection, paginationArgs); err != nil {
			return connection, err
		}
	}
	if len(connection.Edges) > 0 {
		connection.PageInfo.EndCursor = &connection.Edges[len(connection.Edges)-1].Cursor
		connection.PageInfo.StartCursor = &connection.Edges[0].Cursor
	}
	return connection, nil
}

func safeInt64Ptr(i *int64) int64 {
	if i == nil {
		return 0
	}
	return *i
}

// paginateManually applies the pagination arguments to the edges in memory and sets hasNextPage +
// hasPrevPage. The behavior is expected to conform to the Relay Cursor spec:
// https://facebook.github.io/relay/graphql/connections.htm#EdgesToReturn()
func paginateManually(c *Connection, args *ConnectionArgs) error {
	var elemsAfter, elemsBefore bool
	c.Edges, elemsAfter, elemsBefore = applyCursorsToAllEdges(c.Edges, args.Before, args.After)

	c.PageInfo.HasNextPage = args.Before != nil && elemsAfter
	c.PageInfo.HasPrevPage = args.After != nil && elemsBefore

	if (safeInt64Ptr(args.First) < 0) || safeInt64Ptr(args.Last) < 0 {
		return fmt.Errorf("first/last cannot be a negative integer")
	}

	if args.First != nil && args.Last != nil {
		return fmt.Errorf("cannot use both first and last together")
	}

	if args.First != nil && len(c.Edges) > int(*args.First) {
		c.Edges = c.Edges[:int(*args.First)]
		c.PageInfo.HasNextPage = true
	}

	if args.Last != nil && len(c.Edges) > int(*args.Last) {
		c.Edges = c.Edges[len(c.Edges)-int(*args.Last):]
		c.PageInfo.HasPrevPage = true
	}
	return nil
}

// getCursorIndex returns the index corresponding to the cursor in the slice.
func getCursorIndex(edges []Edge, cursor string) int {
	for i, val := range edges {
		if val.Cursor == cursor {
			return i
		}
	}
	return -1
}

// applyCursorsToAllEdges returns the slice of edges after applying the after and before arguments.
// It also implements part of the hasNextPage and hasPrevPage algorithm by returning if there are
// elements after or before the arguments.
func applyCursorsToAllEdges(edges []Edge, before *string, after *string) ([]Edge, bool, bool) {
	edgeCount := len(edges)
	elemsAfter := false
	elemsBefore := false

	if after != nil {
		i := getCursorIndex(edges, *after)
		if i != -1 {
			edges = edges[i+1:]
			if i != 0 {
				elemsBefore = true
			}
		}

	}
	if before != nil {
		i := getCursorIndex(edges, *before)
		if i != -1 {
			edges = edges[:i]
			if i != edgeCount-1 {
				elemsAfter = true
			}
		}

	}

	return edges, elemsAfter, elemsBefore

}
