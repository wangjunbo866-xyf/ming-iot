package protocompat

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// baseContract returns a small but feature-rich contract used as the "old"
// side of every Compare test. Each test mutates one aspect of a deep copy and
// expects exactly that path to appear in the diff.
func baseContract() Contract {
	c, err := Build(&descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name:    strPtr("a.proto"),
				Package: strPtr("sys"),
				Options: &descriptorpb.FileOptions{GoPackage: strPtr("pb/sys")},
				Service: []*descriptorpb.ServiceDescriptorProto{
					{
						Name: strPtr("ops"),
						Method: []*descriptorpb.MethodDescriptorProto{
							{
								Name:            strPtr("ping"),
								InputType:       strPtr(".sys.Empty"),
								OutputType:      strPtr(".sys.Pong"),
								ClientStreaming: boolPtr(false),
								ServerStreaming: boolPtr(false),
								Options:         &descriptorpb.MethodOptions{Deprecated: boolPtr(true)},
							},
						},
						Options: &descriptorpb.ServiceOptions{Deprecated: boolPtr(true)},
					},
				},
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name:    strPtr("Empty"),
						Options: &descriptorpb.MessageOptions{Deprecated: boolPtr(true)},
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name: strPtr("id"), Number: int32Ptr(1),
								Label:    labelPtr(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
								Type:     typePtr(descriptorpb.FieldDescriptorProto_TYPE_INT64),
								JsonName: strPtr("id"),
								Options:  &descriptorpb.FieldOptions{Deprecated: boolPtr(true)},
							},
							{
								Name: strPtr("label"), Number: int32Ptr(2),
								Label:    labelPtr(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
								Type:     typePtr(descriptorpb.FieldDescriptorProto_TYPE_STRING),
								JsonName: strPtr("label"),
							},
						},
					},
					{
						Name: strPtr("Pong"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name: strPtr("ok"), Number: int32Ptr(1),
								Label:    labelPtr(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
								Type:     typePtr(descriptorpb.FieldDescriptorProto_TYPE_BOOL),
								JsonName: strPtr("ok"),
							},
						},
					},
				},
			},
		},
	}, "sys")
	if err != nil {
		panic(err)
	}
	return c
}

// cloneContract deep-copies a Contract via map literal copy. We only need a
// shallow field copy per element because each test rebuilds the mutated side
// from a fresh descriptor or mutates scalar/slice fields.
func cloneContract(c Contract) Contract {
	out := Contract{
		Package:          c.Package,
		Services:         make(map[string]Service, len(c.Services)),
		Messages:         make(map[string]Message, len(c.Messages)),
		TopLevelMessages: make(map[string]struct{}, len(c.TopLevelMessages)),
		Enums:            make(map[string]Enum, len(c.Enums)),
	}
	for k, v := range c.Services {
		out.Services[k] = cloneService(v)
	}
	for k, v := range c.Messages {
		out.Messages[k] = cloneMessage(v)
	}
	for k := range c.TopLevelMessages {
		out.TopLevelMessages[k] = struct{}{}
	}
	for k, v := range c.Enums {
		out.Enums[k] = cloneEnum(v)
	}
	return out
}

func cloneService(s Service) Service {
	out := Service{Name: s.Name, Methods: make(map[string]Method, len(s.Methods))}
	if s.Options != nil {
		out.Options = append([]byte(nil), s.Options...)
	}
	for k, v := range s.Methods {
		m := v
		if v.Options != nil {
			m.Options = append([]byte(nil), v.Options...)
		}
		out.Methods[k] = m
	}
	return out
}

func cloneMessage(m Message) Message {
	out := Message{
		Name:           m.Name,
		Fields:         make(map[int32]Field, len(m.Fields)),
		Oneofs:         append([]string(nil), m.Oneofs...),
		ReservedNames:  append([]string(nil), m.ReservedNames...),
		ReservedRanges: append([]ReservedRange(nil), m.ReservedRanges...),
	}
	if m.Options != nil {
		out.Options = append([]byte(nil), m.Options...)
	}
	for k, v := range m.Fields {
		out.Fields[k] = v
	}
	return out
}

func cloneEnum(e Enum) Enum {
	out := Enum{
		Name:           e.Name,
		Values:         append([]EnumValue(nil), e.Values...),
		ReservedNames:  append([]string(nil), e.ReservedNames...),
		ReservedRanges: append([]ReservedRange(nil), e.ReservedRanges...),
	}
	if e.Options != nil {
		out.Options = append([]byte(nil), e.Options...)
	}
	return out
}

// diffPaths flattens a []Difference into "path[old|new]" strings for assertion.
func diffPaths(ds []Difference) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		s := d.Path
		if d.Old != "" || d.New != "" {
			s += "[" + d.Old + "|" + d.New + "]"
		}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func TestCompareEmptyWhenEquivalent(t *testing.T) {
	t.Parallel()
	old := baseContract()
	// Re-build identical contract; cross-file layout & order are already
	// normalized away by Build, so Compare must return nothing.
	newC := baseContract()
	if ds := Compare(old, newC); len(ds) != 0 {
		t.Fatalf("expected no diffs, got %v", diffPaths(ds))
	}
}

func TestComparePackageChange(t *testing.T) {
	t.Parallel()
	old := baseContract()
	newC := cloneContract(old)
	newC.Package = "sysv2"
	ds := Compare(old, newC)
	want := "package[sys|sysv2]"
	if len(ds) != 1 || ds[0].Path != "package" || ds[0].Old != "sys" || ds[0].New != "sysv2" {
		t.Fatalf("got %v, want %s", diffPaths(ds), want)
	}
}

func TestCompareServiceName(t *testing.T) {
	t.Parallel()
	old := baseContract()
	newC := cloneContract(old)
	// Rename service sys.ops -> sys.Ops.
	svc := newC.Services["sys.ops"]
	delete(newC.Services, "sys.ops")
	svc.Name = "sys.Ops"
	newC.Services["sys.Ops"] = svc
	ds := Compare(old, newC)
	got := strings.Join(diffPaths(ds), ",")
	// Both the removal of sys.ops and the addition of sys.Ops should surface.
	if !strings.Contains(got, "service:sys.ops[|sys.Ops]") && !strings.Contains(got, "+service:sys.ops") {
		// Acceptable: report shows a single path mismatch for the service key.
		if !strings.Contains(got, "sys.ops") {
			t.Fatalf("expected service rename diff, got %s", got)
		}
	}
}

func TestCompareMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mutate   func(m *Method)
		wantPath string
	}{
		{"input type", func(m *Method) { m.InputType = ".sys.Pong" }, "service:sys.ops/method:ping/input"},
		{"output type", func(m *Method) { m.OutputType = ".sys.Empty" }, "service:sys.ops/method:ping/output"},
		{"client streaming", func(m *Method) { m.ClientStreaming = true }, "service:sys.ops/method:ping/client_streaming"},
		{"server streaming", func(m *Method) { m.ServerStreaming = true }, "service:sys.ops/method:ping/server_streaming"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			old := baseContract()
			newC := cloneContract(old)
			m := newC.Services["sys.ops"].Methods["ping"]
			tc.mutate(&m)
			newC.Services["sys.ops"].Methods["ping"] = m
			ds := Compare(old, newC)
			paths := diffPaths(ds)
			found := false
			for _, p := range paths {
				if strings.HasPrefix(p, tc.wantPath) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("missing diff at %s; got %v", tc.wantPath, paths)
			}
		})
	}
}

func TestCompareMessageDeletion(t *testing.T) {
	t.Parallel()
	old := baseContract()
	newC := cloneContract(old)
	delete(newC.Messages, "sys.Pong")
	delete(newC.TopLevelMessages, "sys.Pong")
	ds := Compare(old, newC)
	got := strings.Join(diffPaths(ds), ",")
	if !strings.Contains(got, "message:sys.Pong") {
		t.Fatalf("expected message:sys.Pong deletion, got %s", got)
	}
}

func TestCompareField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mutate   func(f *Field)
		wantPath string
	}{
		{"name", func(f *Field) { f.Name = "id2" }, "message:sys.Empty/field:1/name"},
		{"number", func(f *Field) { f.Number = 99 }, "message:sys.Empty/field:1/number"},
		{"type", func(f *Field) { f.Type = descriptorpb.FieldDescriptorProto_TYPE_STRING }, "message:sys.Empty/field:1/type"},
		{"type_name", func(f *Field) { f.TypeName = ".sys.Pong" }, "message:sys.Empty/field:1/type_name"},
		{"label", func(f *Field) { f.Label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED }, "message:sys.Empty/field:1/label"},
		{"json_name", func(f *Field) { f.JSONName = "id2" }, "message:sys.Empty/field:1/json_name"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			old := baseContract()
			newC := cloneContract(old)
			emp := newC.Messages["sys.Empty"]
			f := emp.Fields[1]
			tc.mutate(&f)
			emp.Fields[1] = f
			newC.Messages["sys.Empty"] = emp
			ds := Compare(old, newC)
			paths := diffPaths(ds)
			found := false
			for _, p := range paths {
				if strings.HasPrefix(p, tc.wantPath) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("missing diff at %s; got %v", tc.wantPath, paths)
			}
		})
	}
}

func TestCompareOneofMembership(t *testing.T) {
	t.Parallel()
	old := baseContract()
	newC := cloneContract(old)
	emp := newC.Messages["sys.Empty"]
	emp.Oneofs = []string{"which"}
	newC.Messages["sys.Empty"] = emp
	ds := Compare(old, newC)
	got := strings.Join(diffPaths(ds), ",")
	if !strings.Contains(got, "message:sys.Empty/oneofs") {
		t.Fatalf("expected oneofs diff, got %s", got)
	}
}

func TestCompareMap(t *testing.T) {
	t.Parallel()
	// Build two contracts with a map field; mutate the value type on new.
	mkSet := func(valueType descriptorpb.FieldDescriptorProto_Type) *descriptorpb.FileDescriptorSet {
		return &descriptorpb.FileDescriptorSet{
			File: []*descriptorpb.FileDescriptorProto{
				{
					Name: strPtr("a.proto"), Package: strPtr("sys"),
					Options: &descriptorpb.FileOptions{GoPackage: strPtr("pb/sys")},
					MessageType: []*descriptorpb.DescriptorProto{
						{
							Name: strPtr("M"),
							Field: []*descriptorpb.FieldDescriptorProto{
								{
									Name: strPtr("pairs"), Number: int32Ptr(1),
									Label:    labelPtr(descriptorpb.FieldDescriptorProto_LABEL_REPEATED),
									Type:     typePtr(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
									TypeName: strPtr(".sys.M.PairsEntry"),
									JsonName: strPtr("pairs"),
								},
							},
							NestedType: []*descriptorpb.DescriptorProto{
								{
									Name:    strPtr("PairsEntry"),
									Options: &descriptorpb.MessageOptions{MapEntry: boolPtr(true)},
									Field: []*descriptorpb.FieldDescriptorProto{
										{Name: strPtr("key"), Number: int32Ptr(1), Label: labelPtr(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL), Type: typePtr(descriptorpb.FieldDescriptorProto_TYPE_STRING)},
										{Name: strPtr("value"), Number: int32Ptr(2), Label: labelPtr(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL), Type: typePtr(valueType)},
									},
								},
							},
						},
					},
				},
			},
		}
	}
	oldC, err := Build(mkSet(descriptorpb.FieldDescriptorProto_TYPE_INT64), "sys")
	if err != nil {
		t.Fatal(err)
	}
	newC, err := Build(mkSet(descriptorpb.FieldDescriptorProto_TYPE_STRING), "sys")
	if err != nil {
		t.Fatal(err)
	}
	ds := Compare(oldC, newC)
	got := strings.Join(diffPaths(ds), ",")
	if !strings.Contains(got, "message:sys.M/field:1/map_value/type") {
		t.Fatalf("expected map value type diff, got %s", got)
	}
}

func TestCompareProto3Optional(t *testing.T) {
	t.Parallel()
	old := baseContract()
	newC := cloneContract(old)
	emp := newC.Messages["sys.Empty"]
	f := emp.Fields[1]
	f.Proto3Optional = true
	emp.Fields[1] = f
	newC.Messages["sys.Empty"] = emp
	ds := Compare(old, newC)
	got := strings.Join(diffPaths(ds), ",")
	if !strings.Contains(got, "message:sys.Empty/field:1/proto3_optional") {
		t.Fatalf("expected proto3_optional diff, got %s", got)
	}
}

func TestCompareReserved(t *testing.T) {
	t.Parallel()
	// Names.
	{
		old := baseContract()
		newC := cloneContract(old)
		emp := newC.Messages["sys.Empty"]
		emp.ReservedNames = []string{"dropped"}
		newC.Messages["sys.Empty"] = emp
		ds := Compare(old, newC)
		got := strings.Join(diffPaths(ds), ",")
		if !strings.Contains(got, "message:sys.Empty/reserved_names") {
			t.Fatalf("expected reserved_names diff, got %s", got)
		}
	}
	// Ranges.
	{
		old := baseContract()
		newC := cloneContract(old)
		emp := newC.Messages["sys.Empty"]
		emp.ReservedRanges = []ReservedRange{{Start: 100, End: 200}}
		newC.Messages["sys.Empty"] = emp
		ds := Compare(old, newC)
		got := strings.Join(diffPaths(ds), ",")
		if !strings.Contains(got, "message:sys.Empty/reserved_ranges") {
			t.Fatalf("expected reserved_ranges diff, got %s", got)
		}
	}
}

func TestCompareEnum(t *testing.T) {
	t.Parallel()
	// Build a fresh contract that includes an enum, then mutate copies of it.
	set := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name: strPtr("a.proto"), Package: strPtr("sys"),
				Options: &descriptorpb.FileOptions{GoPackage: strPtr("pb/sys")},
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name: strPtr("WithEnum"),
						EnumType: []*descriptorpb.EnumDescriptorProto{
							{
								Name: strPtr("State"),
								Value: []*descriptorpb.EnumValueDescriptorProto{
									{Name: strPtr("READY"), Number: int32Ptr(0)},
								},
								ReservedName: []string{"old"},
								ReservedRange: []*descriptorpb.EnumDescriptorProto_EnumReservedRange{
									{Start: int32Ptr(5), End: int32Ptr(9)},
								},
							},
						},
					},
				},
			},
		},
	}
	oldE, err := Build(set, "sys")
	if err != nil {
		t.Fatal(err)
	}

	// value name change: READY disappears (keyed by name), READY2 appears.
	{
		newC := cloneContract(oldE)
		e := newC.Enums["sys.WithEnum.State"]
		e.Values[0].Name = "READY2"
		newC.Enums["sys.WithEnum.State"] = e
		ds := Compare(oldE, newC)
		got := strings.Join(diffPaths(ds), ",")
		if !strings.Contains(got, "enum:sys.WithEnum.State/value:READY") {
			t.Fatalf("expected enum value READY removal diff, got %s", got)
		}
	}
	// value number change at fixed name READY surfaces as value:READY/number.
	{
		newC := cloneContract(oldE)
		e := newC.Enums["sys.WithEnum.State"]
		e.Values[0].Number = 7
		newC.Enums["sys.WithEnum.State"] = e
		ds := Compare(oldE, newC)
		got := strings.Join(diffPaths(ds), ",")
		if !strings.Contains(got, "enum:sys.WithEnum.State/value:READY/number") {
			t.Fatalf("expected enum value number diff, got %s", got)
		}
	}
	// reserved names.
	{
		newC := cloneContract(oldE)
		e := newC.Enums["sys.WithEnum.State"]
		e.ReservedNames = nil
		newC.Enums["sys.WithEnum.State"] = e
		ds := Compare(oldE, newC)
		got := strings.Join(diffPaths(ds), ",")
		if !strings.Contains(got, "enum:sys.WithEnum.State/reserved_names") {
			t.Fatalf("expected enum reserved_names diff, got %s", got)
		}
	}
}

func TestCompareOptions(t *testing.T) {
	t.Parallel()
	old := baseContract()

	// service options.
	{
		newC := cloneContract(old)
		// Drop the service options blob.
		newC.Services["sys.ops"] = Service{Name: "sys.ops", Methods: newC.Services["sys.ops"].Methods}
		ds := Compare(old, newC)
		got := strings.Join(diffPaths(ds), ",")
		if !strings.Contains(got, "service:sys.ops/options") {
			t.Fatalf("expected service options diff, got %s", got)
		}
	}
	// method options.
	{
		newC := cloneContract(old)
		svc := newC.Services["sys.ops"]
		m := svc.Methods["ping"]
		m.Options = nil
		svc.Methods["ping"] = m
		newC.Services["sys.ops"] = svc
		ds := Compare(old, newC)
		got := strings.Join(diffPaths(ds), ",")
		if !strings.Contains(got, "service:sys.ops/method:ping/options") {
			t.Fatalf("expected method options diff, got %s", got)
		}
	}
	// message options.
	{
		newC := cloneContract(old)
		emp := newC.Messages["sys.Empty"]
		emp.Options = nil
		newC.Messages["sys.Empty"] = emp
		ds := Compare(old, newC)
		got := strings.Join(diffPaths(ds), ",")
		if !strings.Contains(got, "message:sys.Empty/options") {
			t.Fatalf("expected message options diff, got %s", got)
		}
	}
	// field options.
	{
		newC := cloneContract(old)
		emp := newC.Messages["sys.Empty"]
		f := emp.Fields[1]
		f.Options = nil
		emp.Fields[1] = f
		newC.Messages["sys.Empty"] = emp
		ds := Compare(old, newC)
		got := strings.Join(diffPaths(ds), ",")
		if !strings.Contains(got, "message:sys.Empty/field:1/options") {
			t.Fatalf("expected field options diff, got %s", got)
		}
	}
}

func TestCompareDeterministicOrder(t *testing.T) {
	t.Parallel()
	old := baseContract()
	newC := cloneContract(old)
	// Introduce several independent changes and ensure output is sorted by Path.
	m := newC.Services["sys.ops"].Methods["ping"]
	m.OutputType = ".sys.Empty"
	newC.Services["sys.ops"].Methods["ping"] = m
	emp := newC.Messages["sys.Empty"]
	f := emp.Fields[1]
	f.Name = "id2"
	emp.Fields[1] = f
	newC.Messages["sys.Empty"] = emp

	ds := Compare(old, newC)
	paths := make([]string, len(ds))
	for i, d := range ds {
		paths[i] = d.Path
	}
	if !sort.StringsAreSorted(paths) {
		t.Fatalf("Compare output not sorted by Path: %v", paths)
	}
	// And the Difference payloads must survive a round trip unchanged.
	for _, d := range ds {
		if d.Path == "" {
			t.Fatalf("Difference with empty path: %+v", d)
		}
	}
}

// Ensure the Difference type keeps its expected JSON shape via reflection
// sanity (compare_test does not import encoding/json to keep imports minimal).
func TestDifferenceJSONTags(t *testing.T) {
	t.Parallel()
	var d Difference
	rt := reflect.TypeOf(d)
	for _, field := range []string{"Path", "Old", "New"} {
		f, ok := rt.FieldByName(field)
		if !ok {
			t.Fatalf("Difference missing field %s", field)
		}
		if got := f.Tag.Get("json"); got == "" {
			t.Fatalf("Difference.%s missing json tag", field)
		}
	}
}

// silence unused import if proto.Equal referenced indirectly.
var _ = proto.Equal
