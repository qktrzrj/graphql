package internal

type Schema struct {
	objects   map[string]*Object
	enumTypes map[string]*Enum
}
