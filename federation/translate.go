package federation

import (
	"encoding/json"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/shyptr/graphql"
	"github.com/shyptr/graphql/errors"
	"github.com/shyptr/graphql/internal"
)

func ConvertRequest(request *FederationRequest) *Plan {
	if request == nil {
		return nil
	}
	return &Plan{
		Kind:         request.GetKind(),
		SelectionSet: convertSelectionSet(request.GetSelectionSet()),
	}
}

func ConvertToSelectionSet(s *internal.SelectionSet) *SelectionSet {
	if s == nil {
		return nil
	}
	return &SelectionSet{
		Loc:        convertToLoc(s.Loc),
		Selections: convertToSelections(s.Selections),
		Fragments:  convertToFragments(s.Fragments),
	}
}

func ConvertResponse(response *FederationResponse) *graphql.Response {
	if response == nil {
		return nil
	}
	return &graphql.Response{
		Data:   convertAnyToInterface(response.GetData()),
		Errors: convertErrors(response.Errors),
	}
}

func ConvertToResponse(data interface{}, errs errors.MultiError) *FederationResponse {
	marshal, _ := json.Marshal(data)
	return &FederationResponse{
		Data:   &any.Any{Value: marshal},
		Errors: convertToErrors(errs),
	}
}

func convertToLocs(l []errors.Location) []*Location {
	var locs []*Location
	for _, li := range l {
		locs = append(locs, convertToLoc(li))
	}
	return locs
}

func convertToLoc(l errors.Location) *Location {
	return &Location{
		Line:   int32(l.Line),
		Column: int32(l.Column),
	}
}

func convertToErrors(errs errors.MultiError) []*GraphQLError {
	var es []*GraphQLError
	for _, e := range errs {
		var path []string
		for _, p := range e.Path {
			path = append(path, p.(string))
		}
		es = append(es, &GraphQLError{
			Message:   e.Message,
			Locations: convertToLocs(e.Locations),
			Path:      path,
		})
	}
	if len(es) > 0 {
		return es
	}
	return nil
}

func convertErrors(errs []*GraphQLError) errors.MultiError {
	var es errors.MultiError
	for _, e := range errs {
		var locs []errors.Location
		for _, l := range e.GetLocations() {
			locs = append(locs, convertLoc(l))
		}
		var path []interface{}
		for _, p := range e.GetPath() {
			path = append(path, p)
		}
		es = append(es, &errors.GraphQLError{
			Message:   e.GetMessage(),
			Locations: locs,
			Path:      path,
		})
	}
	if len(es) > 0 {
		return es
	}
	return nil
}

func convertSelectionSet(selectionSet *SelectionSet) *internal.SelectionSet {
	if selectionSet == nil {
		return nil
	}
	return &internal.SelectionSet{
		Loc:        convertLoc(selectionSet.Loc),
		Selections: convertSelections(selectionSet.GetSelections()),
		Fragments:  convertFragments(selectionSet.GetFragments()),
	}
}

func convertFragments(f []*FragmentSpread) []*internal.FragmentSpread {
	var fragments []*internal.FragmentSpread
	for _, fs := range f {
		fragments = append(fragments, &internal.FragmentSpread{
			Loc:        convertLoc(fs.GetLoc()),
			Fragment:   convertFragmentDefinitions(fs.GetFragment()),
			Directives: convertDirectives(fs.GetDirectives()),
		})
	}
	return fragments
}

func convertToFragments(f []*internal.FragmentSpread) []*FragmentSpread {
	var fragments []*FragmentSpread
	for _, fs := range f {
		fragments = append(fragments, &FragmentSpread{
			Loc:        convertToLoc(fs.Loc),
			Fragment:   convertToFragmentDefinitions(fs.Fragment),
			Directives: convertToDirectives(fs.Directives),
		})
	}
	return fragments
}

func convertToFragmentDefinitions(f *internal.FragmentDefinition) *FragmentDefinition {
	if f == nil {
		return nil
	}
	return &FragmentDefinition{
		Name:         f.Name,
		On:           f.On,
		SelectionSet: ConvertToSelectionSet(f.SelectionSet),
		Loc:          convertToLoc(f.Loc),
	}
}

func convertFragmentDefinitions(f *FragmentDefinition) *internal.FragmentDefinition {
	if f == nil {
		return nil
	}
	return &internal.FragmentDefinition{
		Name:         f.GetName(),
		On:           f.GetOn(),
		SelectionSet: convertSelectionSet(f.GetSelectionSet()),
		Loc:          convertLoc(f.GetLoc()),
	}
}

func convertToSelections(selections []*internal.Selection) []*Selection {
	var sels []*Selection
	for _, s := range selections {
		args, _ := json.Marshal(s.Args)
		sels = append(sels, &Selection{
			Name:         s.Name,
			Alias:        s.Alias,
			Args:         &any.Any{Value: args},
			SelectionSet: ConvertToSelectionSet(s.SelectionSet),
			Directives:   convertToDirectives(s.Directives),
			Loc:          convertToLoc(s.Loc),
		})
	}
	return sels
}

func convertSelections(selections []*Selection) []*internal.Selection {
	var sels []*internal.Selection
	for _, s := range selections {
		sels = append(sels, &internal.Selection{
			Name:         s.GetName(),
			Alias:        s.GetAlias(),
			Args:         convertAnyToInterface(s.GetArgs()),
			SelectionSet: convertSelectionSet(s.GetSelectionSet()),
			Directives:   convertDirectives(s.GetDirectives()),
			Loc:          convertLoc(s.GetLoc()),
		})
	}
	return sels
}

func convertToDirectives(directives []*internal.Directive) []*Directive {
	var dirs []*Directive
	for _, d := range directives {
		argVals, _ := json.Marshal(d.ArgVals)
		dirs = append(dirs, &Directive{
			Name:    d.Name,
			ArgVals: &any.Any{Value: argVals},
			Loc:     convertToLoc(d.Loc),
		})
	}
	return dirs
}

func convertDirectives(directives []*Directive) []*internal.Directive {
	var dirs []*internal.Directive
	for _, d := range directives {
		dirs = append(dirs, &internal.Directive{
			Name:    d.GetName(),
			ArgVals: convertAnyToInterface(d.GetArgVals()).(map[string]interface{}),
			Loc:     convertLoc(d.GetLoc()),
		})
	}
	return dirs
}

func convertAnyToInterface(any *any.Any) interface{} {
	dest := make(map[string]interface{})
	json.Unmarshal(any.GetValue(), &dest)
	return dest
}

func convertLoc(location *Location) errors.Location {
	return errors.Location{
		Line:   int(location.GetLine()),
		Column: int(location.GetColumn()),
	}
}
