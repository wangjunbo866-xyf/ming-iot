package rpcgen

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/types/descriptorpb"
)

func TestCheckDescriptorSetValidatesGraphAndServiceSet(t *testing.T) {
	m, set := protoGraphFixture()
	if err := CheckDescriptorSet(m, set); err != nil {
		t.Fatal(err)
	}

	t.Run("nil descriptor", func(t *testing.T) {
		if err := CheckDescriptorSet(m, nil); err == nil || !strings.HasPrefix(err.Error(), "proto_graph:") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("extra descriptor service", func(t *testing.T) {
		_, changed := protoGraphFixture()
		changed.File[3].Service = append(changed.File[3].Service, &descriptorpb.ServiceDescriptorProto{Name: strPtr("Extra")})
		if err := CheckDescriptorSet(m, changed); err == nil || !strings.HasPrefix(err.Error(), "proto_graph:") {
			t.Fatalf("err = %v", err)
		}
	})
}
