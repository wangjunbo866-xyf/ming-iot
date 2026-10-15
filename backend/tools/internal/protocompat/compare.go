package protocompat

import (
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/types/descriptorpb"
)

// Difference describes a single semantic change between two contracts at a
// fixed path. Path follows a stable dotted form such as
// "message:sys.UserInfo/field:1/type".
type Difference struct {
	Path string `json:"path"`
	Old  string `json:"old,omitempty"`
	New  string `json:"new,omitempty"`
}

// Compare returns the sorted, fully-resolved set of semantic differences
// between old and new. The result is deterministic: every map is iterated in
// sorted-key order and the final slice is sorted by Path, then Old, then New.
//
// Compare never returns nil differences: two semantically-equal contracts
// produce a zero-length slice.
func Compare(old, new Contract) []Difference {
	var ds []Difference

	// Package.
	if old.Package != new.Package {
		ds = append(ds, Difference{Path: "package", Old: old.Package, New: new.Package})
	}

	// Services: union of keys.
	ds = append(ds, compareServices(old.Services, new.Services)...)
	// Messages: union of keys.
	ds = append(ds, compareMessages(old.Messages, new.Messages)...)
	// Enums: union of keys.
	ds = append(ds, compareEnums(old.Enums, new.Enums)...)

	sortDifferences(ds)
	return ds
}

func compareServices(old, new map[string]Service) []Difference {
	var ds []Difference
	keys := unionKeys(serviceKeys(old), serviceKeys(new))
	for _, k := range keys {
		o, okOld := old[k]
		n, okNew := new[k]
		switch {
		case !okNew:
			ds = append(ds, Difference{Path: "service:" + k, Old: k})
		case !okOld:
			ds = append(ds, Difference{Path: "service:" + k, New: k})
		default:
			ds = append(ds, compareOneService(k, o, n)...)
		}
	}
	return ds
}

func compareOneService(key string, o, n Service) []Difference {
	var ds []Difference
	// Options.
	if !bytesEqual(o.Options, n.Options) {
		ds = append(ds, Difference{Path: "service:" + key + "/options", Old: optionsRepr(o.Options), New: optionsRepr(n.Options)})
	}
	// Methods: union by name.
	names := unionKeys(methodKeys(o.Methods), methodKeys(n.Methods))
	for _, name := range names {
		path := "service:" + key + "/method:" + name
		om, okOld := o.Methods[name]
		nm, okNew := n.Methods[name]
		switch {
		case !okNew:
			ds = append(ds, Difference{Path: path, Old: methodRepr(om)})
		case !okOld:
			ds = append(ds, Difference{Path: path, New: methodRepr(nm)})
		default:
			ds = append(ds, compareMethod(path, om, nm)...)
		}
	}
	return ds
}

func compareMethod(path string, o, n Method) []Difference {
	var ds []Difference
	if o.InputType != n.InputType {
		ds = append(ds, Difference{Path: path + "/input", Old: o.InputType, New: n.InputType})
	}
	if o.OutputType != n.OutputType {
		ds = append(ds, Difference{Path: path + "/output", Old: o.OutputType, New: n.OutputType})
	}
	if o.ClientStreaming != n.ClientStreaming {
		ds = append(ds, Difference{Path: path + "/client_streaming", Old: fmt.Sprint(o.ClientStreaming), New: fmt.Sprint(n.ClientStreaming)})
	}
	if o.ServerStreaming != n.ServerStreaming {
		ds = append(ds, Difference{Path: path + "/server_streaming", Old: fmt.Sprint(o.ServerStreaming), New: fmt.Sprint(n.ServerStreaming)})
	}
	if !bytesEqual(o.Options, n.Options) {
		ds = append(ds, Difference{Path: path + "/options", Old: optionsRepr(o.Options), New: optionsRepr(n.Options)})
	}
	return ds
}

func compareMessages(old, new map[string]Message) []Difference {
	var ds []Difference
	keys := unionKeys(messageKeys(old), messageKeys(new))
	for _, k := range keys {
		o, okOld := old[k]
		n, okNew := new[k]
		switch {
		case !okNew:
			ds = append(ds, Difference{Path: "message:" + k, Old: k})
		case !okOld:
			ds = append(ds, Difference{Path: "message:" + k, New: k})
		default:
			ds = append(ds, compareOneMessage(k, o, n)...)
		}
	}
	return ds
}

func compareOneMessage(key string, o, n Message) []Difference {
	var ds []Difference
	if !bytesEqual(o.Options, n.Options) {
		ds = append(ds, Difference{Path: "message:" + key + "/options", Old: optionsRepr(o.Options), New: optionsRepr(n.Options)})
	}
	// Oneofs: compare sorted slice equality.
	if !stringSliceEqual(o.Oneofs, n.Oneofs) {
		ds = append(ds, Difference{Path: "message:" + key + "/oneofs", Old: strings.Join(o.Oneofs, ","), New: strings.Join(n.Oneofs, ",")})
	}
	// Reserved names.
	if !stringSliceEqual(o.ReservedNames, n.ReservedNames) {
		ds = append(ds, Difference{Path: "message:" + key + "/reserved_names", Old: strings.Join(o.ReservedNames, ","), New: strings.Join(n.ReservedNames, ",")})
	}
	// Reserved ranges.
	if !rangeSliceEqual(o.ReservedRanges, n.ReservedRanges) {
		ds = append(ds, Difference{Path: "message:" + key + "/reserved_ranges", Old: rangesRepr(o.ReservedRanges), New: rangesRepr(n.ReservedRanges)})
	}
	// Fields: union by number.
	nums := unionKeysInt32(fieldKeys(o.Fields), fieldKeys(n.Fields))
	for _, num := range nums {
		path := "message:" + key + "/field:" + fmtInt32(num)
		of, okOld := o.Fields[num]
		nf, okNew := n.Fields[num]
		switch {
		case !okNew:
			ds = append(ds, Difference{Path: path, Old: fieldRepr(of)})
		case !okOld:
			ds = append(ds, Difference{Path: path, New: fieldRepr(nf)})
		default:
			ds = append(ds, compareField(path, of, nf)...)
		}
	}
	return ds
}

func compareField(path string, o, n Field) []Difference {
	var ds []Difference
	if o.Name != n.Name {
		ds = append(ds, Difference{Path: path + "/name", Old: o.Name, New: n.Name})
	}
	if o.Number != n.Number {
		ds = append(ds, Difference{Path: path + "/number", Old: fmtInt32(o.Number), New: fmtInt32(n.Number)})
	}
	if o.Type != n.Type {
		ds = append(ds, Difference{Path: path + "/type", Old: typeRepr(o.Type), New: typeRepr(n.Type)})
	}
	if o.TypeName != n.TypeName {
		ds = append(ds, Difference{Path: path + "/type_name", Old: o.TypeName, New: n.TypeName})
	}
	if o.Label != n.Label {
		ds = append(ds, Difference{Path: path + "/label", Old: labelRepr(o.Label), New: labelRepr(n.Label)})
	}
	if o.JSONName != n.JSONName {
		ds = append(ds, Difference{Path: path + "/json_name", Old: o.JSONName, New: n.JSONName})
	}
	if o.Oneof != n.Oneof {
		ds = append(ds, Difference{Path: path + "/oneof", Old: o.Oneof, New: n.Oneof})
	}
	if o.Proto3Optional != n.Proto3Optional {
		ds = append(ds, Difference{Path: path + "/proto3_optional", Old: fmt.Sprint(o.Proto3Optional), New: fmt.Sprint(n.Proto3Optional)})
	}
	if o.Default != n.Default {
		ds = append(ds, Difference{Path: path + "/default", Old: o.Default, New: n.Default})
	}
	// Map key/value: compare as field sub-paths.
	if (o.MapKey == nil) != (n.MapKey == nil) {
		ds = append(ds, Difference{Path: path + "/map_key", Old: fieldRepr(orField(o.MapKey)), New: fieldRepr(orField(n.MapKey))})
	} else if o.MapKey != nil {
		ds = append(ds, compareField(path+"/map_key", *o.MapKey, *n.MapKey)...)
	}
	if (o.MapValue == nil) != (n.MapValue == nil) {
		ds = append(ds, Difference{Path: path + "/map_value", Old: fieldRepr(orField(o.MapValue)), New: fieldRepr(orField(n.MapValue))})
	} else if o.MapValue != nil {
		ds = append(ds, compareField(path+"/map_value", *o.MapValue, *n.MapValue)...)
	}
	if !bytesEqual(o.Options, n.Options) {
		ds = append(ds, Difference{Path: path + "/options", Old: optionsRepr(o.Options), New: optionsRepr(n.Options)})
	}
	return ds
}

func compareEnums(old, new map[string]Enum) []Difference {
	var ds []Difference
	keys := unionKeys(enumKeys(old), enumKeys(new))
	for _, k := range keys {
		o, okOld := old[k]
		n, okNew := new[k]
		switch {
		case !okNew:
			ds = append(ds, Difference{Path: "enum:" + k, Old: k})
		case !okOld:
			ds = append(ds, Difference{Path: "enum:" + k, New: k})
		default:
			ds = append(ds, compareOneEnum(k, o, n)...)
		}
	}
	return ds
}

func compareOneEnum(key string, o, n Enum) []Difference {
	var ds []Difference
	if !bytesEqual(o.Options, n.Options) {
		ds = append(ds, Difference{Path: "enum:" + key + "/options", Old: optionsRepr(o.Options), New: optionsRepr(n.Options)})
	}
	if !stringSliceEqual(o.ReservedNames, n.ReservedNames) {
		ds = append(ds, Difference{Path: "enum:" + key + "/reserved_names", Old: strings.Join(o.ReservedNames, ","), New: strings.Join(n.ReservedNames, ",")})
	}
	if !rangeSliceEqual(o.ReservedRanges, n.ReservedRanges) {
		ds = append(ds, Difference{Path: "enum:" + key + "/reserved_ranges", Old: rangesRepr(o.ReservedRanges), New: rangesRepr(n.ReservedRanges)})
	}
	// Values: union by value name. Enum values are keyed by name so that a
	// number change surfaces as "value:<name>/number" rather than a
	// delete/add pair.
	names := unionKeys(enumValueNames(o.Values), enumValueNames(n.Values))
	for _, name := range names {
		path := "enum:" + key + "/value:" + name
		ov, okOld := enumValueByName(o.Values, name)
		nv, okNew := enumValueByName(n.Values, name)
		switch {
		case !okNew:
			ds = append(ds, Difference{Path: path, Old: fmtInt32(ov.Number)})
		case !okOld:
			ds = append(ds, Difference{Path: path, New: fmtInt32(nv.Number)})
		default:
			if ov.Number != nv.Number {
				ds = append(ds, Difference{Path: path + "/number", Old: fmtInt32(ov.Number), New: fmtInt32(nv.Number)})
			}
			if !bytesEqual(ov.Options, nv.Options) {
				ds = append(ds, Difference{Path: path + "/options", Old: optionsRepr(ov.Options), New: optionsRepr(nv.Options)})
			}
		}
	}
	return ds
}

// ---- key/union helpers ----

func serviceKeys(m map[string]Service) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func methodKeys(m map[string]Method) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func messageKeys(m map[string]Message) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func enumKeys(m map[string]Enum) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func fieldKeys(m map[int32]Field) []int32 {
	out := make([]int32, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func enumValueNames(vs []EnumValue) []string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.Name)
	}
	sort.Strings(out)
	return out
}

func enumValueByName(vs []EnumValue, name string) (EnumValue, bool) {
	for _, v := range vs {
		if v.Name == name {
			return v, true
		}
	}
	return EnumValue{}, false
}

// unionKeys merges two already-sorted int32 slices into a sorted unique slice.
func unionKeysInt32(a, b []int32) []int32 {
	out := make([]int32, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		var cand int32
		switch {
		case j >= len(b) || (i < len(a) && a[i] <= b[j]):
			cand = a[i]
			i++
		default:
			cand = b[j]
			j++
		}
		if n := len(out); n == 0 || out[n-1] != cand {
			out = append(out, cand)
		}
	}
	return out
}

// unionKeys merges two sorted string slices into a sorted unique slice.
func unionKeys(a, b []string) []string {
	out := make([]string, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		var cand string
		switch {
		case j >= len(b) || (i < len(a) && a[i] <= b[j]):
			cand = a[i]
			i++
		default:
			cand = b[j]
			j++
		}
		if n := len(out); n == 0 || out[n-1] != cand {
			out = append(out, cand)
		}
	}
	return out
}

// fieldKeys returns sorted keys, but unionKeysInt32 is the actual merge; alias
// for clarity in compareOneMessage.
var _ = unionKeysInt32

// ---- formatting helpers ----

func sortDifferences(ds []Difference) {
	sort.SliceStable(ds, func(i, j int) bool {
		if ds[i].Path != ds[j].Path {
			return ds[i].Path < ds[j].Path
		}
		if ds[i].Old != ds[j].Old {
			return ds[i].Old < ds[j].Old
		}
		return ds[i].New < ds[j].New
	})
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func rangeSliceEqual(a, b []ReservedRange) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func optionsRepr(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return fmt.Sprintf("0x%x", b)
}

func rangesRepr(rs []ReservedRange) string {
	parts := make([]string, 0, len(rs))
	for _, r := range rs {
		parts = append(parts, fmt.Sprintf("%d-%d", r.Start, r.End))
	}
	return strings.Join(parts, ",")
}

func typeRepr(t descriptorpb.FieldDescriptorProto_Type) string {
	return t.String()
}

func labelRepr(l descriptorpb.FieldDescriptorProto_Label) string {
	return l.String()
}

func fieldRepr(f Field) string {
	return fmt.Sprintf("%s#%d:%s", f.Name, f.Number, typeRepr(f.Type))
}

func methodRepr(m Method) string {
	return fmt.Sprintf("%s(%s)->(%s)", m.Name, m.InputType, m.OutputType)
}

func orField(f *Field) Field {
	if f == nil {
		return Field{}
	}
	return *f
}

func fmtInt32(v int32) string {
	return fmt.Sprintf("%d", v)
}
