package rpcgen

import (
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/types/descriptorpb"
)

// CheckProtoGraph validates the manifest's explicitly referenced protobuf
// files and their descriptor dependency graph.
func CheckProtoGraph(m *Manifest, set *descriptorpb.FileDescriptorSet) error {
	graph, err := buildProtoGraph(m, set)
	if err != nil {
		return err
	}
	_, err = topologicalOrder(graph)
	return err
}

// TopologicalProtoOrder returns supporting and service protos with every
// relevant dependency before its dependent, in deterministic order.
func TopologicalProtoOrder(m *Manifest, set *descriptorpb.FileDescriptorSet) ([]string, error) {
	graph, err := buildProtoGraph(m, set)
	if err != nil {
		return nil, err
	}
	return topologicalOrder(graph)
}

func topologicalOrder(graph protoGraph) ([]string, error) {
	color := make(map[string]uint8, len(graph.nodes))
	order := make([]string, 0, len(graph.nodes))
	var visit func(string) error
	visit = func(name string) error {
		switch color[name] {
		case 1:
			return fmt.Errorf("proto_graph: dependency cycle at %q", name)
		case 2:
			return nil
		}
		color[name] = 1
		dependencies := append([]string(nil), graph.dependencies[name]...)
		sort.Strings(dependencies)
		for _, dependency := range dependencies {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		color[name] = 2
		order = append(order, name)
		return nil
	}

	nodes := append([]string(nil), graph.nodes...)
	sort.Strings(nodes)
	for _, name := range nodes {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	return order, nil
}

type protoGraph struct {
	nodes        []string
	dependencies map[string][]string
}

func buildProtoGraph(m *Manifest, set *descriptorpb.FileDescriptorSet) (protoGraph, error) {
	if m == nil {
		return protoGraph{}, fmt.Errorf("proto_graph: manifest is nil")
	}
	if set == nil {
		return protoGraph{}, fmt.Errorf("proto_graph: descriptor set is nil")
	}

	referenced := map[string]string{m.Aggregate: "aggregate"}
	for _, name := range m.SupportingProtos {
		referenced[name] = "supporting proto"
	}
	for _, service := range m.Services {
		referenced[service.Proto] = "service proto"
	}

	counts := make(map[string]int, len(set.File))
	files := make(map[string]*descriptorpb.FileDescriptorProto, len(set.File))
	for _, file := range set.File {
		if file == nil || file.GetName() == "" {
			continue
		}
		name := file.GetName()
		counts[name]++
		if counts[name] == 1 {
			files[name] = file
		}
	}
	related := make([]string, 0, len(referenced))
	for name := range referenced {
		related = append(related, name)
	}
	sort.Strings(related)
	for _, name := range related {
		role := referenced[name]
		if name == "" {
			return protoGraph{}, fmt.Errorf("proto_graph: %s name is empty", role)
		}
		if counts[name] == 0 {
			return protoGraph{}, fmt.Errorf("proto_graph: referenced file %q is missing", name)
		}
		if counts[name] != 1 {
			return protoGraph{}, fmt.Errorf("proto_graph: referenced file %q is not unique", name)
		}
	}

	for _, name := range related {
		file := files[name]
		if file.GetPackage() != m.Package {
			return protoGraph{}, fmt.Errorf("proto_graph: file %q package %q, want %q", name, file.GetPackage(), m.Package)
		}
		option := file.GetOptions()
		if option == nil || option.GetGoPackage() == "" {
			return protoGraph{}, fmt.Errorf("proto_graph: file %q has no go_package, want %q", name, m.GoPackage)
		}
		goPackage := strings.SplitN(option.GetGoPackage(), ";", 2)[0]
		if goPackage != m.GoPackage {
			return protoGraph{}, fmt.Errorf("proto_graph: file %q go_package %q, want %q", name, goPackage, m.GoPackage)
		}
	}

	aggregate := files[m.Aggregate]
	if len(aggregate.GetMessageType()) != 0 || len(aggregate.GetEnumType()) != 0 || len(aggregate.GetService()) != 0 {
		return protoGraph{}, fmt.Errorf("aggregate: %q must not define message, enum, or service", m.Aggregate)
	}
	public := make(map[string]bool, len(aggregate.GetPublicDependency()))
	for _, index := range aggregate.GetPublicDependency() {
		if index < 0 || int(index) >= len(aggregate.GetDependency()) {
			return protoGraph{}, fmt.Errorf("aggregate: %q has invalid public dependency index %d", m.Aggregate, index)
		}
		public[aggregate.GetDependency()[index]] = true
	}
	for _, name := range related {
		if name == m.Aggregate {
			continue
		}
		found := false
		for _, dependency := range aggregate.GetDependency() {
			if dependency == name {
				found = true
				break
			}
		}
		if !found || !public[name] {
			return protoGraph{}, fmt.Errorf("aggregate: %q must public import %q", m.Aggregate, name)
		}
	}

	declared := make(map[string]string, len(m.Services))
	for _, service := range m.Services {
		if previous, ok := declared[service.Service]; ok {
			return protoGraph{}, fmt.Errorf("proto_graph: service %q declared by %q and %q", service.Service, previous, service.Proto)
		}
		declared[service.Service] = service.Proto
	}
	serviceCounts := make(map[string]int, len(declared))
	for _, name := range related {
		for _, service := range files[name].GetService() {
			serviceName := service.GetName()
			declaredFile, ok := declared[serviceName]
			if !ok {
				return protoGraph{}, fmt.Errorf("proto_graph: extra service %q in %q", serviceName, name)
			}
			serviceCounts[serviceName]++
			if declaredFile != name {
				return protoGraph{}, fmt.Errorf("proto_graph: service %q in %q, want %q", serviceName, name, declaredFile)
			}
		}
	}
	serviceNames := make([]string, 0, len(declared))
	for serviceName := range declared {
		serviceNames = append(serviceNames, serviceName)
	}
	sort.Strings(serviceNames)
	for _, serviceName := range serviceNames {
		declaredFile := declared[serviceName]
		if serviceCounts[serviceName] == 0 {
			return protoGraph{}, fmt.Errorf("proto_graph: service %q missing from %q", serviceName, declaredFile)
		}
		if serviceCounts[serviceName] != 1 {
			return protoGraph{}, fmt.Errorf("proto_graph: service %q defined %d times", serviceName, serviceCounts[serviceName])
		}
	}

	nodes := make([]string, 0, len(related)-1)
	dependencies := make(map[string][]string, len(related)-1)
	for _, name := range related {
		if name == m.Aggregate {
			continue
		}
		nodes = append(nodes, name)
		for _, dependency := range files[name].GetDependency() {
			if dependency != m.Aggregate && referenced[dependency] != "" {
				dependencies[name] = append(dependencies[name], dependency)
			}
		}
	}
	return protoGraph{nodes: nodes, dependencies: dependencies}, nil
}
