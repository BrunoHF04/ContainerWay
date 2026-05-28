package webapp

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// openWithDefaultApp abre o ficheiro com a aplicação predefinida do SO (servidor local).
func openWithDefaultApp(filePath string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("cmd", "/c", "start", "", filePath).Start()
	case "darwin":
		return exec.Command("open", filePath).Start()
	default:
		return exec.Command("xdg-open", filePath).Start()
	}
}

// openWithEditor abre com programa predefinido ou Notepad++ (Windows).
func openWithEditor(filePath, editor string) error {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return fmt.Errorf("caminho vazio")
	}
	switch strings.ToLower(strings.TrimSpace(editor)) {
	case "", "default", "system":
		return openWithDefaultApp(filePath)
	case "notepad++", "npp":
		if p := findNotepadPlusPlus(); p != "" {
			return exec.Command(p, filePath).Start()
		}
		return fmt.Errorf("Notepad++ não encontrado — instale ou use o programa predefinido")
	default:
		return fmt.Errorf("editor desconhecido: %s", editor)
	}
}

func findNotepadPlusPlus() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	candidates := []string{
		`C:\Program Files\Notepad++\notepad++.exe`,
		`C:\Program Files (x86)\Notepad++\notepad++.exe`,
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
