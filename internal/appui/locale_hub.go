package appui

import (
	"strings"

	"fyne.io/fyne/v2"
)

const (
	langPreferenceKey = "ui.language"
	langPTBR           = "pt-BR"
	langEN             = "en"
	langES             = "es"
)

// Textos do menu principal (hub da sessão) por idioma.
var hubStrings = map[string]map[string]string{
	langPTBR: {
		"sessionStart":       "Início da sessão",
		"connectedToFmt":     "Conectado a: %s",
		"hostUnknown":        "(host não informado)",
		"searchPlaceholder":  "Pesquisar módulos (ex.: arquivos, docker, discos, e-mail, usuários)…",
		"hintNoModules":      "Nenhum módulo corresponde à pesquisa. Limpe o campo ou tente outras palavras.",
		"open":               "Abrir",
		"manualBtn":          "Manual do sistema",
		"themeLabel":         "Tema",
		"langLabel":          "Idioma",
		"cardFilesTitle":     "Gerenciador de arquivos",
		"cardFilesDesc":      "Pastas no computador local, no servidor e nos contêineres. Envio e recebimento de arquivos.",
		"cardDockerTitle":    "Contêineres Docker",
		"cardDockerDesc":     "Lista do que está em execução no host, com opção de reinício unitário ou em lote.",
		"cardDisksTitle":     "Discos e armazenamento",
		"cardDisksDesc":      "Tabela a partir de lsblk, abas (assistente LVM, resumo, df/LVM) e filtro opcional de dispositivos loop (Snap).",
		"cardTerminalTitle":  "Terminal SSH",
		"cardTerminalDesc":   "Console remoto básico para executar comandos no host conectado.",
		"cardAutoTitle":      "Central de automações",
		"cardAutoDesc":       "Regras operacionais com gatilho e ação para executar tarefas automáticas no host conectado.",
		"cardSettingsTitle":  "Configurações",
		"cardSettingsDesc":   "Contas de acesso ao aplicativo e alertas por SMTP.",
		"btnUsers":           "Usuários",
		"btnMail":            "Alertas por e-mail",
		"srchFiles":          "gerenciador arquivos arquivo pasta servidor sftp transferência local remoto explorador receber enviar files manager folder",
		"srchDocker":         "docker contêiner container rodando reiniciar host imagem compose container running image",
		"srchDisks":          "disco discos armazenamento lsblk lvm volume partição df montagem snap loop disk storage partition",
		"srchTerminal":       "terminal ssh shell console comando host remoto bash sh command remote",
		"srchAuto":           "automação automacoes gatilho ação acao tarefa rotina runbook operação incidente alerta automation rules trigger",
		"srchSettings":       "configurações configuração usuário usuários admin e-mail email smtp alerta notificação destinatário settings accounts",
	},
	langEN: {
		"sessionStart":       "Session home",
		"connectedToFmt":     "Connected to: %s",
		"hostUnknown":        "(host not set)",
		"searchPlaceholder":  "Search modules (e.g. files, docker, disks, e-mail, users)…",
		"hintNoModules":      "No module matches the search. Clear the field or try other words.",
		"open":               "Open",
		"manualBtn":          "System manual",
		"themeLabel":         "Theme",
		"langLabel":          "Language",
		"cardFilesTitle":     "File manager",
		"cardFilesDesc":      "Folders on the local PC, server and containers. Send and receive files.",
		"cardDockerTitle":    "Docker containers",
		"cardDockerDesc":     "List of what is running on the host, with single or batch restart.",
		"cardDisksTitle":     "Disks and storage",
		"cardDisksDesc":      "Table from lsblk, tabs (LVM assistant, summary, df/LVM) and optional loop-device filter (Snap).",
		"cardTerminalTitle":  "SSH terminal",
		"cardTerminalDesc":   "Basic remote console to run commands on the connected host.",
		"cardAutoTitle":      "Automation center",
		"cardAutoDesc":       "Operational rules with trigger and action to run tasks on the connected host.",
		"cardSettingsTitle":  "Settings",
		"cardSettingsDesc":   "App access accounts and SMTP alerts.",
		"btnUsers":           "Users",
		"btnMail":            "E-mail alerts",
		"srchFiles":          "file manager folder server sftp transfer local remote explorer upload download gerenciador arquivos archivo carpeta",
		"srchDocker":         "docker container running restart host image compose contêiner contenedor imagen",
		"srchDisks":          "disk storage lsblk lvm partition mount snap loop disco volumen partición almacenamiento",
		"srchTerminal":       "terminal ssh shell console command remote bash sh comando consola",
		"srchAuto":          "automation rules trigger action task runbook alert operación automatización reglas",
		"srchSettings":      "settings users admin email smtp notification recipient configuración usuarios correo",
	},
	langES: {
		"sessionStart":       "Inicio de sesión",
		"connectedToFmt":     "Conectado a: %s",
		"hostUnknown":        "(host no indicado)",
		"searchPlaceholder":  "Buscar módulos (ej.: archivos, docker, discos, e-mail, usuarios)…",
		"hintNoModules":      "Ningún módulo coincide con la búsqueda. Vacíe el campo o pruebe otras palabras.",
		"open":               "Abrir",
		"manualBtn":          "Manual del sistema",
		"themeLabel":         "Tema",
		"langLabel":          "Idioma",
		"cardFilesTitle":     "Administrador de archivos",
		"cardFilesDesc":      "Carpetas en el equipo local, servidor y contenedores. Envío y recepción de archivos.",
		"cardDockerTitle":    "Contenedores Docker",
		"cardDockerDesc":     "Lista de lo en ejecución en el host, con reinicio unitario o en lote.",
		"cardDisksTitle":     "Discos y almacenamiento",
		"cardDisksDesc":      "Tabla desde lsblk, pestañas (asistente LVM, resumen, df/LVM) y filtro opcional de loop (Snap).",
		"cardTerminalTitle":  "Terminal SSH",
		"cardTerminalDesc":   "Consola remota básica para ejecutar comandos en el host conectado.",
		"cardAutoTitle":      "Central de automatizaciones",
		"cardAutoDesc":       "Reglas operativas con disparador y acción para tareas automáticas en el host.",
		"cardSettingsTitle":  "Configuración",
		"cardSettingsDesc":   "Cuentas de acceso a la aplicación y alertas por SMTP.",
		"btnUsers":           "Usuarios",
		"btnMail":            "Alertas por e-mail",
		"srchFiles":          "archivos carpeta servidor sftp transferencia local remoto explorador enviar recibir file folder manager",
		"srchDocker":         "docker contenedor ejecutando reiniciar host imagen compose container image",
		"srchDisks":          "disco discos almacenamiento lsblk lvm partición montaje snap loop volume partition",
		"srchTerminal":       "terminal ssh consola comando host remoto bash sh shell command",
		"srchAuto":          "automatización reglas disparador acción tarea runbook alerta automation",
		"srchSettings":      "configuración usuarios admin correo smtp notificación destinatario settings e-mail",
	},
}

func loadUILanguage(a fyne.App) string {
	if a == nil {
		return langPTBR
	}
	s := strings.TrimSpace(a.Preferences().StringWithFallback(langPreferenceKey, langPTBR))
	switch s {
	case langPTBR, langEN, langES:
		return s
	default:
		return langPTBR
	}
}

func saveUILanguage(a fyne.App, code string) {
	switch code {
	case langEN, langES:
		a.Preferences().SetString(langPreferenceKey, code)
	default:
		a.Preferences().SetString(langPreferenceKey, langPTBR)
	}
}

func langLabelForCode(code string) string {
	switch code {
	case langEN:
		return "English"
	case langES:
		return "Español"
	default:
		return "Português (BR)"
	}
}

func langCodeFromLabel(label string) string {
	switch strings.TrimSpace(label) {
	case "English":
		return langEN
	case "Español":
		return langES
	default:
		return langPTBR
	}
}
