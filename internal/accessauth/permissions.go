package accessauth

import "strings"

// IDs de telas (módulos) da interface web.
const (
	ScreenExplorer     = "explorer"
	ScreenDocker       = "docker"
	ScreenDisks        = "disks"
	ScreenTerminal     = "terminal"
	ScreenAutomations  = "automations"
	ScreenSettings     = "settings"
)

// IDs de ações (API + UI).
const (
	ActionFilesRead          = "files.read"
	ActionFilesWrite         = "files.write"
	ActionFilesDelete        = "files.delete"
	ActionFilesTransfer      = "files.transfer"
	ActionDockerView         = "docker.view"
	ActionDockerControl      = "docker.control"
	ActionDockerManage       = "docker.manage"
	ActionDisksView          = "disks.view"
	ActionDisksManage        = "disks.manage"
	ActionTerminalUse        = "terminal.use"
	ActionAutomationsManage  = "automations.manage"
	ActionSudoUse            = "sudo.use"
	ActionSettingsManage     = "settings.manage"
)

// Permissions telas e ações permitidas a um usuário local.
type Permissions struct {
	Screens []string `json:"screens,omitempty"`
	Actions []string `json:"actions,omitempty"`
}

// AllScreens devolve todos os módulos da web.
func AllScreens() []string {
	return []string{
		ScreenExplorer,
		ScreenDocker,
		ScreenDisks,
		ScreenTerminal,
		ScreenAutomations,
		ScreenSettings,
	}
}

// AllActions devolve todas as ações configuráveis.
func AllActions() []string {
	return []string{
		ActionFilesRead,
		ActionFilesWrite,
		ActionFilesDelete,
		ActionFilesTransfer,
		ActionDockerView,
		ActionDockerControl,
		ActionDockerManage,
		ActionDisksView,
		ActionDisksManage,
		ActionTerminalUse,
		ActionAutomationsManage,
		ActionSudoUse,
		ActionSettingsManage,
	}
}

// FullPermissions concede acesso total (admin).
func FullPermissions() Permissions {
	return Permissions{
		Screens: AllScreens(),
		Actions: AllActions(),
	}
}

// DefaultPermissions é o conjunto para usuários sem perfil explícito (compatível com versões anteriores).
func DefaultPermissions() Permissions {
	return Permissions{
		Screens: []string{
			ScreenExplorer,
			ScreenDocker,
			ScreenDisks,
			ScreenTerminal,
			ScreenAutomations,
			ScreenSettings,
		},
		Actions: without(AllActions(), ActionSettingsManage),
	}
}

func without(all []string, skip string) []string {
	out := make([]string, 0, len(all))
	for _, a := range all {
		if a != skip {
			out = append(out, a)
		}
	}
	return out
}

// ResolvePermissions devolve permissões efetivas (admin = tudo).
func ResolvePermissions(username string, stored *Permissions) Permissions {
	if IsAdminUsername(username) {
		return FullPermissions()
	}
	if stored == nil || (len(stored.Screens) == 0 && len(stored.Actions) == 0) {
		return DefaultPermissions()
	}
	p := Permissions{
		Screens: dedupe(stored.Screens),
		Actions: dedupe(stored.Actions),
	}
	if len(p.Screens) == 0 {
		p.Screens = DefaultPermissions().Screens
	}
	if len(p.Actions) == 0 {
		p.Actions = DefaultPermissions().Actions
	}
	return p
}

// IsAdminUsername indica conta administrador fixa.
func IsAdminUsername(username string) bool {
	return normalizeUsername(username) == normalizeUsername(defaultUser)
}

// HasScreen verifica acesso a um módulo.
func (p Permissions) HasScreen(screen string) bool {
	for _, s := range p.Screens {
		if s == screen {
			return true
		}
	}
	return false
}

// HasAction verifica permissão de ação.
func (p Permissions) HasAction(action string) bool {
	for _, a := range p.Actions {
		if a == action {
			return true
		}
	}
	return false
}

func dedupe(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = trimKey(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func trimKey(s string) string {
	return strings.TrimSpace(s)
}

// SanitizePermissions remove IDs desconhecidos.
func SanitizePermissions(p *Permissions) *Permissions {
	if p == nil {
		return nil
	}
	allowedScreens := map[string]struct{}{}
	for _, s := range AllScreens() {
		allowedScreens[s] = struct{}{}
	}
	allowedActions := map[string]struct{}{}
	for _, a := range AllActions() {
		allowedActions[a] = struct{}{}
	}
	out := Permissions{}
	for _, s := range p.Screens {
		if _, ok := allowedScreens[trimKey(s)]; ok {
			out.Screens = append(out.Screens, trimKey(s))
		}
	}
	for _, a := range p.Actions {
		if _, ok := allowedActions[trimKey(a)]; ok {
			out.Actions = append(out.Actions, trimKey(a))
		}
	}
	out.Screens = dedupe(out.Screens)
	out.Actions = dedupe(out.Actions)
	if len(out.Screens) == 0 && len(out.Actions) == 0 {
		return nil
	}
	return &out
}
