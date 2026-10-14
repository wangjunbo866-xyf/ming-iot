package sysdirect

import (
	"sort"
	"testing"

	"google.golang.org/grpc"
)

// TestRegisterServers 验证 syssvr 的 15 个领域 gRPC 服务都被正确注册。
// 它创建一个全新的 grpc.Server，调用 registerServers(server, nil)（传 nil
// ServiceContext，因为各 NewXxxServer 构造函数只保存 svcCtx，不在注册期解引用），
// 再断言 server.GetServiceInfo() 的 key 集合正好是预期的 15 个服务全名，
// 其中 ops 服务保持小写（sys.ops），不存在 sys.Ops。
func TestRegisterServers(t *testing.T) {
	server := grpc.NewServer()
	t.Cleanup(server.GracefulStop)

	registerServers(server, nil)

	info := server.GetServiceInfo()
	got := make([]string, 0, len(info))
	for name := range info {
		got = append(got, name)
	}
	sort.Strings(got)

	want := []string{
		"sys.AccessManage",
		"sys.AppManage",
		"sys.AreaManage",
		"sys.Common",
		"sys.DataManage",
		"sys.DepartmentManage",
		"sys.DictManage",
		"sys.Log",
		"sys.ModuleManage",
		"sys.NotifyManage",
		"sys.ProjectManage",
		"sys.RoleManage",
		"sys.TenantManage",
		"sys.UserManage",
		"sys.ops",
	}
	sort.Strings(want)

	if len(got) != len(want) {
		t.Fatalf("服务数量不匹配: got %d (%v), want %d (%v)", len(got), got, len(want), want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("服务集合不匹配:\n got = %v\nwant = %v", got, want)
		}
	}

	// 单独强调 ops 服务保持小写。
	if _, ok := info["sys.ops"]; !ok {
		t.Errorf("期望存在 sys.ops，实际服务集合: %v", got)
	}
	if _, ok := info["sys.Ops"]; ok {
		t.Errorf("不应存在 sys.Ops（ops 服务必须保持小写），实际服务集合: %v", got)
	}
}
