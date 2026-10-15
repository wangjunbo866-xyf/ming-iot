// Package protocompat normalizes a protobuf FileDescriptorSet into a stable
// contract that is independent of file layout and declaration order, and
// compares two such contracts for semantic compatibility.
//
// The contract intentionally ignores: file names, file dependency graph,
// source code info (comments/positions) and declaration order. It aggregates
// definitions purely by their fully-qualified name so that physically splitting
// one .proto into many is detectable as a no-op.
package protocompat

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Contract is the package-aggregated, file-layout-independent view of one
// protobuf package.
type Contract struct {
	// Package is the protobuf package name the contract was built for.
	Package string
	// Services is keyed by fully-qualified service name, e.g. "sys.ops".
	Services map[string]Service
	// Messages collects every message (top-level, nested and map-entry) keyed
	// by fully-qualified name. Nested and map-entry types are included so that
	// compatibility comparison can recurse into them.
	Messages map[string]Message
	// TopLevelMessages records only file-top-level business messages. CLI
	// summaries such as "256 messages" count this set. Values are unused.
	TopLevelMessages map[string]struct{}
	// Enums is keyed by fully-qualified enum name.
	Enums map[string]Enum
}

// Service holds the per-method view of a service.
type Service struct {
	// Name is the fully-qualified service name.
	Name string
	// Methods is keyed by method (short) name.
	Methods map[string]Method
	// Options is the deterministically serialized ServiceOptions, empty if none.
	Options []byte
}

// Method captures the wire-relevant bits of an RPC method.
type Method struct {
	// Name is the short method name.
	Name string
	// InputType is the fully-qualified input message type, e.g. ".sys.Empty".
	InputType string
	// OutputType is the fully-qualified output message type.
	OutputType string
	// ClientStreaming mirrors MethodDescriptorProto.client_streaming.
	ClientStreaming bool
	// ServerStreaming mirrors MethodDescriptorProto.server_streaming.
	ServerStreaming bool
	// Options is the deterministically serialized MethodOptions, empty if none.
	Options []byte
}

// Message is the per-field view of a message type.
type Message struct {
	// Name is the fully-qualified message name.
	Name string
	// Fields is keyed by field number.
	Fields map[int32]Field
	// Oneofs is the sorted list of oneof declaration names.
	Oneofs []string
	// ReservedNames is the sorted set of reserved field names.
	ReservedNames []string
	// ReservedRanges is the sorted list of reserved field number ranges.
	ReservedRanges []ReservedRange
	// Options is the deterministically serialized MessageOptions, empty if none.
	Options []byte
}

// Field captures the wire-relevant bits of a single field.
type Field struct {
	Name           string
	JSONName       string
	Number         int32
	Type           descriptorpb.FieldDescriptorProto_Type
	TypeName       string
	Label          descriptorpb.FieldDescriptorProto_Label
	Oneof          string
	Proto3Optional bool
	Default        string
	// MapKey and MapValue are non-nil only when this field is a map and the
	// map-entry message was resolvable in the same contract build. They hold
	// the key/value field descriptors.
	MapKey   *Field
	MapValue *Field
	// Options is the deterministically serialized FieldOptions, empty if none.
	Options []byte
}

// Enum is the per-value view of an enum type.
type Enum struct {
	// Name is the fully-qualified enum name.
	Name string
	// Values is sorted by value number.
	Values []EnumValue
	// ReservedNames is the sorted set of reserved value names.
	ReservedNames []string
	// ReservedRanges is the sorted list of reserved value number ranges.
	ReservedRanges []ReservedRange
	// Options is the deterministically serialized EnumOptions, empty if none.
	Options []byte
}

// EnumValue pairs an enum value name with its number.
type EnumValue struct {
	Name    string
	Number  int32
	Options []byte
}

// ReservedRange is an inclusive [Start, End] number range. proto reserves are
// exclusive of End; the stored End matches the descriptor's exclusive boundary.
type ReservedRange struct {
	Start int32
	End   int32
}

// pendingMsg pairs a fully-qualified message name with its descriptor during
// the two-pass build.
type pendingMsg struct {
	fqName string
	desc   *descriptorpb.DescriptorProto
}

// pendingEnum pairs a fully-qualified enum name with its descriptor.
type pendingEnum struct {
	fqName string
	desc   *descriptorpb.EnumDescriptorProto
}

// Load reads a FileDescriptorSet from path and builds the contract for
// packageName.
func Load(path, packageName string) (Contract, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Contract{}, fmt.Errorf("read protoset %q: %w", path, err)
	}
	set := &descriptorpb.FileDescriptorSet{}
	if err := proto.Unmarshal(data, set); err != nil {
		return Contract{}, fmt.Errorf("unmarshal protoset %q: %w", path, err)
	}
	return Build(set, packageName)
}

// Build aggregates set into a Contract for packageName. Only files whose
// declared package matches packageName contribute. Definitions are keyed by
// fully-qualified name and aggregated across files; file layout and declaration
// order are ignored.
func Build(set *descriptorpb.FileDescriptorSet, packageName string) (Contract, error) {
	c := Contract{
		Package:          packageName,
		Services:         map[string]Service{},
		Messages:         map[string]Message{},
		TopLevelMessages: map[string]struct{}{},
		Enums:            map[string]Enum{},
	}
	if set == nil {
		return c, errors.New("nil FileDescriptorSet")
	}

	var (
		msgs  []pendingMsg
		enums []pendingEnum
	)
	matchedAny := false

	for _, f := range set.File {
		if f == nil || f.GetPackage() != packageName {
			continue
		}
		matchedAny = true
		// Fully-qualified names use the "pkg.Name" form (no leading dot) for
		// human-readable keys; the descriptor's leading-dot form is only used
		// for type_name resolution and is normalized at lookup time.
		prefix := packageName + "."
		for _, m := range f.MessageType {
			collectMessages(prefix, m, &msgs, &enums)
			fq := prefix + m.GetName()
			c.TopLevelMessages[fq] = struct{}{}
		}
		for _, e := range f.EnumType {
			collectEnums(prefix, e, &enums)
		}
		for _, s := range f.Service {
			if s == nil {
				continue
			}
			fq := prefix + s.GetName()
			svc := Service{Name: fq, Methods: map[string]Method{}}
			if s.Options != nil {
				if b, err := proto.Marshal(s.Options); err == nil {
					svc.Options = b
				}
			}
			for _, md := range s.Method {
				if md == nil {
					continue
				}
				m := Method{
					Name:            md.GetName(),
					InputType:       md.GetInputType(),
					OutputType:      md.GetOutputType(),
					ClientStreaming: md.GetClientStreaming(),
					ServerStreaming: md.GetServerStreaming(),
				}
				if md.Options != nil {
					b, err := proto.Marshal(md.Options)
					if err != nil {
						return c, fmt.Errorf("marshal method options %s.%s: %w", fq, m.Name, err)
					}
					m.Options = b
				}
				svc.Methods[m.Name] = m
			}
			c.Services[fq] = svc
		}
	}
	if !matchedAny {
		return c, fmt.Errorf("package %q not present in descriptor set", packageName)
	}

	// Build message map first so map-entry fields can resolve nested entries.
	for _, pm := range msgs {
		c.Messages[pm.fqName] = buildMessage(pm.fqName, pm.desc, msgs)
	}
	for _, pe := range enums {
		c.Enums[pe.fqName] = buildEnum(pe.fqName, pe.desc)
	}

	return c, nil
}

func collectMessages(prefix string, d *descriptorpb.DescriptorProto, out *[]pendingMsg, enums *[]pendingEnum) {
	if d == nil {
		return
	}
	fq := prefix + d.GetName()
	*out = append(*out, pendingMsg{fqName: fq, desc: d})
	nestedPrefix := fq + "."
	for _, n := range d.NestedType {
		collectMessages(nestedPrefix, n, out, enums)
	}
	for _, e := range d.EnumType {
		collectEnums(nestedPrefix, e, enums)
	}
}

func collectEnums(prefix string, e *descriptorpb.EnumDescriptorProto, out *[]pendingEnum) {
	if e == nil {
		return
	}
	fq := prefix + e.GetName()
	*out = append(*out, pendingEnum{fqName: fq, desc: e})
}

func buildMessage(fqName string, d *descriptorpb.DescriptorProto, allMsgs []pendingMsg) Message {
	m := Message{Name: fqName, Fields: map[int32]Field{}}
	for _, od := range d.OneofDecl {
		if od != nil {
			m.Oneofs = append(m.Oneofs, od.GetName())
		}
	}
	sort.Strings(m.Oneofs)

	m.ReservedNames = append(m.ReservedNames, d.ReservedName...)
	sort.Strings(m.ReservedNames)

	for _, rr := range d.ReservedRange {
		if rr == nil {
			continue
		}
		m.ReservedRanges = append(m.ReservedRanges, ReservedRange{Start: rr.GetStart(), End: rr.GetEnd()})
	}
	sort.Slice(m.ReservedRanges, func(i, j int) bool {
		if m.ReservedRanges[i].Start != m.ReservedRanges[j].Start {
			return m.ReservedRanges[i].Start < m.ReservedRanges[j].Start
		}
		return m.ReservedRanges[i].End < m.ReservedRanges[j].End
	})

	for _, fd := range d.Field {
		if fd == nil {
			continue
		}
		f := Field{
			Name:           fd.GetName(),
			JSONName:       fd.GetJsonName(),
			Number:         fd.GetNumber(),
			Type:           fd.GetType(),
			TypeName:       fd.GetTypeName(),
			Label:          fd.GetLabel(),
			Proto3Optional: fd.GetProto3Optional(),
			Default:        fd.GetDefaultValue(),
		}
		if oi := fd.OneofIndex; oi != nil && int(*oi) < len(d.OneofDecl) {
			f.Oneof = d.OneofDecl[*oi].GetName()
		}
		// Map entry resolution. type_name uses the descriptor's leading-dot
		// form; normalize by stripping it to match our keys.
		if tn := fd.GetTypeName(); tn != "" {
			key := normalizeTypeName(tn)
			if isMapEntry(key, allMsgs) {
				if entry := findMsg(key, allMsgs); entry != nil {
					f.MapKey = mapEntryField(entry, 1)
					f.MapValue = mapEntryField(entry, 2)
				}
			}
		}
		if fd.Options != nil {
			if b, err := proto.Marshal(fd.Options); err == nil {
				f.Options = b
			}
		}
		m.Fields[f.Number] = f
	}

	if d.Options != nil {
		if b, err := proto.Marshal(d.Options); err == nil {
			m.Options = b
		}
	}
	return m
}

// normalizeTypeName strips the leading-dot prefix that descriptors use for
// fully-qualified type_name references, so it matches our dotted key form.
func normalizeTypeName(tn string) string {
	if len(tn) > 0 && tn[0] == '.' {
		return tn[1:]
	}
	return tn
}

// isMapEntry reports whether the message named key (our dotted form) exists and
// is declared with the map_entry = true option.
func isMapEntry(key string, allMsgs []pendingMsg) bool {
	d := findMsg(key, allMsgs)
	if d == nil || d.Options == nil {
		return false
	}
	return d.Options.GetMapEntry()
}

func findMsg(key string, allMsgs []pendingMsg) *descriptorpb.DescriptorProto {
	for _, pm := range allMsgs {
		if pm.fqName == key {
			return pm.desc
		}
	}
	return nil
}

// mapEntryField returns a shallow Field for the map-entry message field with
// the given number (1 = key, 2 = value), or nil if absent.
func mapEntryField(entry *descriptorpb.DescriptorProto, number int32) *Field {
	for _, fd := range entry.Field {
		if fd != nil && fd.GetNumber() == number {
			return &Field{
				Name:     fd.GetName(),
				JSONName: fd.GetJsonName(),
				Number:   fd.GetNumber(),
				Type:     fd.GetType(),
				TypeName: fd.GetTypeName(),
				Label:    fd.GetLabel(),
			}
		}
	}
	return nil
}

func buildEnum(fqName string, e *descriptorpb.EnumDescriptorProto) Enum {
	en := Enum{Name: fqName}
	for _, v := range e.Value {
		if v == nil {
			continue
		}
		ev := EnumValue{Name: v.GetName(), Number: v.GetNumber()}
		if v.Options != nil {
			if b, err := proto.Marshal(v.Options); err == nil {
				ev.Options = b
			}
		}
		en.Values = append(en.Values, ev)
	}
	sort.Slice(en.Values, func(i, j int) bool {
		if en.Values[i].Number != en.Values[j].Number {
			return en.Values[i].Number < en.Values[j].Number
		}
		return en.Values[i].Name < en.Values[j].Name
	})
	en.ReservedNames = append(en.ReservedNames, e.ReservedName...)
	sort.Strings(en.ReservedNames)
	for _, rr := range e.ReservedRange {
		if rr == nil {
			continue
		}
		en.ReservedRanges = append(en.ReservedRanges, ReservedRange{Start: rr.GetStart(), End: rr.GetEnd()})
	}
	sort.Slice(en.ReservedRanges, func(i, j int) bool {
		if en.ReservedRanges[i].Start != en.ReservedRanges[j].Start {
			return en.ReservedRanges[i].Start < en.ReservedRanges[j].Start
		}
		return en.ReservedRanges[i].End < en.ReservedRanges[j].End
	})
	if e.Options != nil {
		if b, err := proto.Marshal(e.Options); err == nil {
			en.Options = b
		}
	}
	return en
}
