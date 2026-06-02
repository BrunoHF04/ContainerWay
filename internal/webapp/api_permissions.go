package webapp

import (
	"net/http"
	"strings"

	"containerway/internal/accessauth"
)

func permissionsPayload(username string) map[string]any {
	p := accessauth.PermissionsForUser(username)
	return map[string]any{
		"screens": p.Screens,
		"actions": p.Actions,
	}
}

// permissionForRequest devolve a ação exigida para o pedido API, ou "" se não aplicável.
func permissionForRequest(method, path string) string {
	path = strings.TrimSuffix(path, "/")
	switch {
	case path == "/api/health",
		path == "/api/auth/login",
		path == "/api/auth/logout",
		path == "/api/auth/me":
		return ""
	case path == "/api/settings/diagnostics":
		return ""
	case strings.HasPrefix(path, "/api/admin/"):
		return accessauth.ActionSettingsManage
	case path == "/api/ssh/terminal/ws":
		return accessauth.ActionTerminalUse
	case path == "/api/ssh/sudo":
		return accessauth.ActionSudoUse
	case strings.HasPrefix(path, "/api/automations/"):
		return accessauth.ActionAutomationsManage
	case strings.HasPrefix(path, "/api/disks/"):
		if method == http.MethodGet {
			return accessauth.ActionDisksView
		}
		return accessauth.ActionDisksManage
	case strings.HasPrefix(path, "/api/docker/"):
		return dockerPermission(method, path)
	case strings.HasPrefix(path, "/api/transfer/"):
		return accessauth.ActionFilesTransfer
	case path == "/api/local/delete" || path == "/api/remote/delete":
		return accessauth.ActionFilesDelete
	case path == "/api/local/mkdir" || path == "/api/local/rename" ||
		path == "/api/remote/mkdir" || path == "/api/remote/rename" ||
		path == "/api/explorer/paste" || path == "/api/explorer/sync-mirror":
		return accessauth.ActionFilesWrite
	case path == "/api/local/file" || path == "/api/remote/file":
		if method == http.MethodGet || method == http.MethodHead {
			return accessauth.ActionFilesRead
		}
		return accessauth.ActionFilesWrite
	case strings.HasPrefix(path, "/api/local/") || strings.HasPrefix(path, "/api/remote/") ||
		strings.HasPrefix(path, "/api/explorer/"):
		if method == http.MethodGet || method == http.MethodHead {
			return accessauth.ActionFilesRead
		}
		if method == http.MethodDelete {
			return accessauth.ActionFilesDelete
		}
		return accessauth.ActionFilesWrite
	case path == "/api/connections" || path == "/api/ssh/connect" ||
		path == "/api/ssh/test" || path == "/api/ssh/disconnect" || path == "/api/ssh/status":
		return ""
	default:
		return ""
	}
}

func dockerPermission(method, path string) string {
	if method == http.MethodGet {
		return accessauth.ActionDockerView
	}
	switch path {
	case "/api/docker/containers/create", "/api/docker/networks/create",
		"/api/docker/volumes/create", "/api/docker/images/remove",
		"/api/docker/volumes/remove", "/api/docker/networks/remove",
		"/api/docker/images/prune", "/api/docker/volumes/prune",
		"/api/docker/buildcache/prune":
		return accessauth.ActionDockerManage
	case "/api/docker/remove", "/api/docker/stop", "/api/docker/start",
		"/api/docker/pause", "/api/docker/unpause",
		"/api/docker/restart", "/api/docker/restart-batch",
		"/api/docker/compose/restart-project":
		return accessauth.ActionDockerControl
	default:
		if strings.Contains(path, "/remove") || strings.Contains(path, "/prune") || strings.Contains(path, "/create") {
			return accessauth.ActionDockerManage
		}
		return accessauth.ActionDockerControl
	}
}

func userHasRequestPermission(username string, method, path string) bool {
	need := permissionForRequest(method, path)
	if need == "" {
		return true
	}
	return accessauth.PermissionsForUser(username).HasAction(need)
}
