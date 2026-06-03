package accessauth

import "testing"

func TestResolvePermissionsAdmin(t *testing.T) {
	p := ResolvePermissions("admin", nil)
	if !p.HasScreen(ScreenDocker) || !p.HasAction(ActionSettingsManage) {
		t.Fatal("admin deve ter acesso total")
	}
}

func TestResolvePermissionsDefault(t *testing.T) {
	p := ResolvePermissions("joao", nil)
	if !p.HasScreen(ScreenExplorer) {
		t.Fatal("default deve incluir explorer")
	}
	if !p.HasScreen(ScreenDesktop) {
		t.Fatal("default deve incluir desktop")
	}
	if p.HasAction(ActionSettingsManage) {
		t.Fatal("default não deve gerir utilizadores")
	}
}

func TestResolvePermissionsCustom(t *testing.T) {
	stored := &Permissions{
		Screens: []string{ScreenTerminal},
		Actions: []string{ActionTerminalUse},
	}
	p := ResolvePermissions("ops", stored)
	if !p.HasScreen(ScreenTerminal) || p.HasScreen(ScreenDocker) {
		t.Fatal("custom screens")
	}
	if !p.HasAction(ActionTerminalUse) || p.HasAction(ActionFilesWrite) {
		t.Fatal("custom actions")
	}
}
