package protocompat

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// strPtr is a small helper to build scalar pointer fields in descriptors.
func strPtr(s string) *string { return &s }

func int32Ptr(v int32) *int32 { return &v }

func boolPtr(v bool) *bool { return &v }

func labelPtr(v descriptorpb.FieldDescriptorProto_Label) *descriptorpb.FieldDescriptorProto_Label {
	return &v
}

func typePtr(v descriptorpb.FieldDescriptorProto_Type) *descriptorpb.FieldDescriptorProto_Type {
	return &v
}

// TestBuildSingleFileVsMultiFile asserts Build aggregates definitions purely by
// their fully-qualified name and ignores which file a definition lives in.
func TestBuildSingleFileVsMultiFile(t *testing.T) {
	t.Parallel()

	// Same service and messages spread across one file vs two files.
	singleFile := &descriptorpb.FileDescriptorSet{
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
								OutputType:      strPtr(".sys.Empty"),
								ClientStreaming: boolPtr(false),
								ServerStreaming: boolPtr(false),
							},
						},
					},
				},
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name: strPtr("Empty"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     strPtr("id"),
								Number:   int32Ptr(1),
								Label:    labelPtr(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
								Type:     typePtr(descriptorpb.FieldDescriptorProto_TYPE_INT64),
								JsonName: strPtr("id"),
							},
						},
					},
				},
			},
		},
	}

	multiFile := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name:    strPtr("b.proto"),
				Package: strPtr("sys"),
				Options: &descriptorpb.FileOptions{GoPackage: strPtr("pb/sys")},
				Service: []*descriptorpb.ServiceDescriptorProto{
					{
						Name: strPtr("ops"),
						Method: []*descriptorpb.MethodDescriptorProto{
							{
								Name:            strPtr("ping"),
								InputType:       strPtr(".sys.Empty"),
								OutputType:      strPtr(".sys.Empty"),
								ClientStreaming: boolPtr(false),
								ServerStreaming: boolPtr(false),
							},
						},
					},
				},
			},
			{
				Name:    strPtr("c.proto"),
				Package: strPtr("sys"),
				Options: &descriptorpb.FileOptions{GoPackage: strPtr("pb/sys")},
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name: strPtr("Empty"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     strPtr("id"),
								Number:   int32Ptr(1),
								Label:    labelPtr(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
								Type:     typePtr(descriptorpb.FieldDescriptorProto_TYPE_INT64),
								JsonName: strPtr("id"),
							},
						},
					},
				},
			},
		},
	}

	single, err := Build(singleFile, "sys")
	if err != nil {
		t.Fatalf("Build singleFile: %v", err)
	}
	multi, err := Build(multiFile, "sys")
	if err != nil {
		t.Fatalf("Build multiFile: %v", err)
	}
	if diff := cmp.Diff(single, multi, cmp.Comparer(proto.Equal)); diff != "" {
		t.Fatalf("Build differs across file layout (-single +multi):\n%s", diff)
	}
}

// TestBuildOrderIndependent asserts that re-declaring the same set in a
// different order produces an equal Contract.
func TestBuildOrderIndependent(t *testing.T) {
	t.Parallel()

	mkSet := func(order ...string) *descriptorpb.FileDescriptorSet {
		mkMethod := func(name string) *descriptorpb.MethodDescriptorProto {
			return &descriptorpb.MethodDescriptorProto{
				Name:            strPtr(name),
				InputType:       strPtr(".sys.Empty"),
				OutputType:      strPtr(".sys.Empty"),
				ClientStreaming: boolPtr(false),
				ServerStreaming: boolPtr(false),
			}
		}
		mkMsg := func(name string) *descriptorpb.DescriptorProto {
			return &descriptorpb.DescriptorProto{
				Name: strPtr(name),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     strPtr("id"),
						Number:   int32Ptr(1),
						Label:    labelPtr(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
						Type:     typePtr(descriptorpb.FieldDescriptorProto_TYPE_INT64),
						JsonName: strPtr("id"),
					},
				},
			}
		}
		svc := &descriptorpb.ServiceDescriptorProto{Name: strPtr("ops")}
		for _, n := range order {
			svc.Method = append(svc.Method, mkMethod(n))
		}
		set := &descriptorpb.FileDescriptorSet{
			File: []*descriptorpb.FileDescriptorProto{
				{
					Name:        strPtr("a.proto"),
					Package:     strPtr("sys"),
					Options:     &descriptorpb.FileOptions{GoPackage: strPtr("pb/sys")},
					Service:     []*descriptorpb.ServiceDescriptorProto{svc},
					MessageType: []*descriptorpb.DescriptorProto{mkMsg("Empty")},
				},
			},
		}
		return set
	}

	a, err := Build(mkSet("ping", "pong"), "sys")
	if err != nil {
		t.Fatalf("Build a: %v", err)
	}
	b, err := Build(mkSet("pong", "ping"), "sys")
	if err != nil {
		t.Fatalf("Build b: %v", err)
	}
	if diff := cmp.Diff(a, b, cmp.Comparer(proto.Equal)); diff != "" {
		t.Fatalf("Build differs by declaration order (-a +b):\n%s", diff)
	}
}

// TestBuildCounters asserts the service/method/message tallies the CLI uses.
func TestBuildCounters(t *testing.T) {
	t.Parallel()

	set := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name:    strPtr("a.proto"),
				Package: strPtr("sys"),
				Options: &descriptorpb.FileOptions{GoPackage: strPtr("pb/sys")},
				Service: []*descriptorpb.ServiceDescriptorProto{
					{Name: strPtr("ops"), Method: []*descriptorpb.MethodDescriptorProto{
						{Name: strPtr("ping"), InputType: strPtr(".sys.Empty"), OutputType: strPtr(".sys.Empty")},
					}},
				},
				MessageType: []*descriptorpb.DescriptorProto{
					{Name: strPtr("Empty")},
					{Name: strPtr("Wrap"), NestedType: []*descriptorpb.DescriptorProto{
						{Name: strPtr("Inner")},
					}},
				},
			},
		},
	}

	c, err := Build(set, "sys")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got, want := len(c.Services), 1; got != want {
		t.Fatalf("services = %d, want %d", got, want)
	}
	if got, want := len(c.Services["sys.ops"].Methods), 1; got != want {
		t.Fatalf("methods = %d, want %d", got, want)
	}
	// TopLevelMessages counts only file-top-level business messages (2 here).
	if got, want := len(c.TopLevelMessages), 2; got != want {
		t.Fatalf("top-level messages = %d, want %d", got, want)
	}
}

// TestBuildFeatures is a table-driven test covering nested message, nested
// enum, map entry, oneof, proto3 optional, reserved names/ranges and method
// streaming flags.
func TestBuildFeatures(t *testing.T) {
	t.Parallel()

	set := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name:    strPtr("a.proto"),
				Package: strPtr("sys"),
				Options: &descriptorpb.FileOptions{GoPackage: strPtr("pb/sys")},
				Service: []*descriptorpb.ServiceDescriptorProto{
					{
						Name: strPtr("Stream"),
						Method: []*descriptorpb.MethodDescriptorProto{
							{
								Name:            strPtr("up"),
								InputType:       strPtr(".sys.Empty"),
								OutputType:      strPtr(".sys.Empty"),
								ClientStreaming: boolPtr(true),
								ServerStreaming: boolPtr(false),
							},
							{
								Name:            strPtr("down"),
								InputType:       strPtr(".sys.Empty"),
								OutputType:      strPtr(".sys.Empty"),
								ClientStreaming: boolPtr(false),
								ServerStreaming: boolPtr(true),
							},
						},
					},
				},
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name: strPtr("Features"),
						Options: &descriptorpb.MessageOptions{
							Deprecated: boolPtr(true),
						},
						ReservedName: []string{"dropped"},
						ReservedRange: []*descriptorpb.DescriptorProto_ReservedRange{
							{Start: int32Ptr(100), End: int32Ptr(200)},
						},
						OneofDecl: []*descriptorpb.OneofDescriptorProto{
							{Name: strPtr("which")},
						},
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:           strPtr("a"),
								Number:         int32Ptr(1),
								Label:          labelPtr(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
								Type:           typePtr(descriptorpb.FieldDescriptorProto_TYPE_STRING),
								JsonName:       strPtr("a"),
								OneofIndex:     int32Ptr(0),
								Proto3Optional: boolPtr(true),
							},
							{
								Name:     strPtr("pairs"),
								Number:   int32Ptr(2),
								Label:    labelPtr(descriptorpb.FieldDescriptorProto_LABEL_REPEATED),
								Type:     typePtr(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
								TypeName: strPtr(".sys.Features.PairsEntry"),
								JsonName: strPtr("pairs"),
							},
						},
						NestedType: []*descriptorpb.DescriptorProto{
							{
								Name: strPtr("PairsEntry"),
								Options: &descriptorpb.MessageOptions{
									MapEntry: boolPtr(true),
								},
								Field: []*descriptorpb.FieldDescriptorProto{
									{
										Name: strPtr("key"), Number: int32Ptr(1),
										Label: labelPtr(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
										Type:  typePtr(descriptorpb.FieldDescriptorProto_TYPE_STRING),
									},
									{
										Name: strPtr("value"), Number: int32Ptr(2),
										Label: labelPtr(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
										Type:  typePtr(descriptorpb.FieldDescriptorProto_TYPE_INT64),
									},
								},
							},
							{
								Name: strPtr("Inner"),
							},
						},
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

	c, err := Build(set, "sys")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	m, ok := c.Messages["sys.Features"]
	if !ok {
		t.Fatalf("missing message sys.Features")
	}
	if len(m.Oneofs) == 0 || m.Oneofs[0] != "which" {
		t.Fatalf("oneofs = %v, want [which]", m.Oneofs)
	}
	if len(m.ReservedNames) == 0 || m.ReservedNames[0] != "dropped" {
		t.Fatalf("reserved names = %v", m.ReservedNames)
	}
	if len(m.ReservedRanges) == 0 || m.ReservedRanges[0].Start != 100 || m.ReservedRanges[0].End != 200 {
		t.Fatalf("reserved ranges = %v", m.ReservedRanges)
	}
	f1, ok := m.Fields[1]
	if !ok {
		t.Fatalf("missing field number 1")
	}
	if !f1.Proto3Optional {
		t.Errorf("field 1 proto3_optional = false, want true")
	}
	if f1.Oneof != "which" {
		t.Errorf("field 1 oneof = %q, want which", f1.Oneof)
	}
	f2, ok := m.Fields[2]
	if !ok {
		t.Fatalf("missing field number 2")
	}
	if f2.MapKey == nil || f2.MapValue == nil {
		t.Errorf("field 2 map key/value not captured")
	}
	if _, ok := c.Messages["sys.Features.PairsEntry"]; !ok {
		t.Errorf("map entry message not aggregated under Messages")
	}
	if _, ok := c.Messages["sys.Features.Inner"]; !ok {
		t.Errorf("nested message not aggregated under Messages")
	}
	e, ok := c.Enums["sys.Features.State"]
	if !ok {
		t.Fatalf("missing enum sys.Features.State")
	}
	if len(e.Values) == 0 || e.Values[0].Number != 0 {
		t.Fatalf("enum values = %v", e.Values)
	}
	if len(e.ReservedRanges) == 0 || e.ReservedRanges[0].Start != 5 {
		t.Fatalf("enum reserved ranges = %v", e.ReservedRanges)
	}
	svc, ok := c.Services["sys.Stream"]
	if !ok {
		t.Fatalf("missing service sys.Stream")
	}
	if up, ok := svc.Methods["up"]; !ok || !up.ClientStreaming {
		t.Errorf("method up streaming flags wrong")
	}
	if down, ok := svc.Methods["down"]; !ok || !down.ServerStreaming {
		t.Errorf("method down streaming flags wrong")
	}
}

// TestBuildRejectsForeignPackage makes sure Build only aggregates the requested package.
func TestBuildRejectsForeignPackage(t *testing.T) {
	t.Parallel()

	set := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name:        strPtr("a.proto"),
				Package:     strPtr("other"),
				Options:     &descriptorpb.FileOptions{GoPackage: strPtr("pb/other")},
				MessageType: []*descriptorpb.DescriptorProto{{Name: strPtr("X")}},
			},
		},
	}
	if _, err := Build(set, "sys"); err == nil {
		t.Fatalf("Build accepted a foreign-only set without error")
	}
}
