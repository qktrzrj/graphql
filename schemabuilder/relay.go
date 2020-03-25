package schemabuilder

// Connection conforms to the GraphQL Connection type in the Relay Pagination spec.
type Connection struct {
	TotalCount int
	Edges      []Edge
	PageInfo   PageInfo
}

// PageInfo contains information for pagination on a connection type. The list of Pages is used for
// page-number based pagination where the ith index corresponds to the start cursor of (i+1)st page.
type PageInfo struct {
	HasNextPage bool
	EndCursor   string
	HasPrevPage bool
	StartCursor string
	Pages       []string
}

// Edge consists of a node paired with its b64 encoded cursor.
type Edge struct {
	Node   interface{}
	Cursor string
}

// ConnectionArgs conform to the pagination arguments as specified by the Relay Spec for Connection
// types. https://facebook.github.io/relay/graphql/connections.htm#sec-Arguments
type ConnectionArgs struct {
	// first: n
	First *int64
	// last: n
	Last *int64
	// after: cursor
	After *string
	// before: cursor
	Before *string
	// User-facing args.
	Args interface{}
	// filterText: "text search"
	FilterText *string
	// FilterTextFields: ["filter name"]
	FilterTextFields *[]string
	// sortBy: "fieldName"
	SortBy *string
	// sortOrder: "asc" | "desc"
	SortOrder *SortOrder
}

// PaginationArgs are used in externally set connections by embedding them in an args struct. They
// are mapped onto ConnectionArgs, which follows the Relay spec for connection types.
type PaginationArgs struct {
	First  *int64
	Last   *int64
	After  *string
	Before *string

	FilterText       *string
	FilterTextFields *[]string
	SortBy           *string
	SortOrder        *SortOrder
}

type SortOrder int64

const (
	SortOrder_Ascending SortOrder = iota
	SortOrder_Descending
)
