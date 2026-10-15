package middleware

import (
	"gitee.com/ming-iot/saas/service/apisvr/internal/config"
	role "gitee.com/ming-iot/saas/service/syssvr/client/rolemanage"
	user "gitee.com/ming-iot/saas/service/syssvr/client/usermanage"
	"net/http"
)

type CheckTokenWareMiddleware struct {
	cfg     config.Config
	UserRpc user.UserManage
	AuthRpc role.RoleManage
}

func NewCheckTokenWareMiddleware(cfg config.Config, UserRpc user.UserManage, AuthRpc role.RoleManage) *CheckTokenWareMiddleware {
	return &CheckTokenWareMiddleware{cfg: cfg, UserRpc: UserRpc, AuthRpc: AuthRpc}
}

func (m *CheckTokenWareMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
	}
}
