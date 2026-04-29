package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/bodgit/sevenzip"
	"github.com/go-gl/glfw/v3.3/glfw"

	"containerway/internal/appui"
)

// main inicializa e executa o fluxo principal deste binario.
func main() {
	logPath := startupLogPath()
	appendStartupLog(logPath, "iniciando ContainerWay")
	appendStartupLog(logPath, fmt.Sprintf("goos=%s goarch=%s", runtime.GOOS, runtime.GOARCH))
	if runtime.GOOS == "windows" {
		relaunched, err := ensureOpenGLReady(logPath)
		if relaunched {
			appendStartupLog(logPath, "OpenGL: processo relançado com fallback de software")
			return
		}
		if err != nil {
			appendStartupLog(logPath, fmt.Sprintf("OpenGL: fallback não aplicado: %v", err))
		}
	}
	defer func() {
		if r := recover(); r != nil {
			appendStartupLog(logPath, fmt.Sprintf("panic: %v", r))
			appendStartupLog(logPath, string(debug.Stack()))
		}
	}()
	appui.Run()
	appendStartupLog(logPath, "encerrado sem panic")
}

// startupLogPath devolve o caminho do log local de inicialização.
func startupLogPath() string {
	base, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(base) == "" {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "ContainerWay")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "startup.log")
}

// appendStartupLog registra uma linha no log de inicialização do executável.
func appendStartupLog(path, line string) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(time.Now().Format("2006-01-02 15:04:05") + " " + line + "\n")
}

// ensureOpenGLReady valida OpenGL e tenta habilitar fallback por software no Windows.
func ensureOpenGLReady(logPath string) (bool, error) {
	if err := probeOpenGL(); err == nil {
		appendStartupLog(logPath, "OpenGL: driver nativo disponível")
		return false, nil
	} else {
		appendStartupLog(logPath, fmt.Sprintf("OpenGL: probe inicial falhou: %v", err))
	}
	dllPath, err := ensureSoftwareOpenGLDLL(logPath)
	if err != nil {
		return false, err
	}
	// Garante prioridade do diretório com o opengl32.dll de fallback.
	pathNow := os.Getenv("PATH")
	dllDir := filepath.Dir(dllPath)
	_ = os.Setenv("PATH", dllDir+";"+pathNow)
	appendStartupLog(logPath, "OpenGL: PATH atualizado com fallback de software")
	bootstrapped := strings.TrimSpace(os.Getenv("CONTAINERWAY_GL_BOOTSTRAPPED")) == "1"
	if !bootstrapped {
		if err := relaunchWithSoftwareGL(dllDir, pathNow, logPath); err != nil {
			appendStartupLog(logPath, fmt.Sprintf("OpenGL: relançamento falhou: %v", err))
		} else {
			return true, nil
		}
	}
	if err := probeOpenGL(); err != nil {
		return false, fmt.Errorf("probe após fallback falhou: %w", err)
	}
	appendStartupLog(logPath, "OpenGL: fallback de software ativo")
	return false, nil
}

// relaunchWithSoftwareGL relança o processo com PATH ajustado para o runtime OpenGL de fallback.
func relaunchWithSoftwareGL(dllDir, oldPath, logPath string) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exePath, os.Args[1:]...)
	cmd.Dir = filepath.Dir(exePath)
	env := os.Environ()
	env = append(env, "CONTAINERWAY_GL_BOOTSTRAPPED=1")
	env = append(env, "PATH="+dllDir+";"+oldPath)
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		return err
	}
	appendStartupLog(logPath, fmt.Sprintf("OpenGL: relançado com PID %d", cmd.Process.Pid))
	return nil
}

// probeOpenGL testa criação de contexto OpenGL mínimo com GLFW.
func probeOpenGL() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := glfw.Init(); err != nil {
		return err
	}
	defer glfw.Terminate()
	glfw.WindowHint(glfw.Visible, glfw.False)
	glfw.WindowHint(glfw.Resizable, glfw.False)
	w, err := glfw.CreateWindow(32, 32, "cw-probe", nil, nil)
	if err != nil {
		return err
	}
	w.MakeContextCurrent()
	w.Destroy()
	return nil
}

// ensureSoftwareOpenGLDLL garante presença da DLL de fallback OpenGL em caminho carregável.
func ensureSoftwareOpenGLDLL(logPath string) (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	exeDir := filepath.Dir(exePath)
	exeDLL := filepath.Join(exeDir, "opengl32.dll")
	if _, err := os.Stat(exeDLL); err == nil {
		_ = hideFileWindows(exeDLL)
		appendStartupLog(logPath, "OpenGL: opengl32.dll já existe ao lado do executável")
		return exeDLL, nil
	}

	cacheDir := filepath.Join(filepath.Dir(logPath), "runtime-mesa")
	_ = os.MkdirAll(cacheDir, 0o755)
	archivePath := filepath.Join(cacheDir, "opengl32sw-64.7z")
	if err := downloadMesaArchive(archivePath); err != nil {
		return "", err
	}
	appendStartupLog(logPath, "OpenGL: pacote de fallback baixado")
	if err := extractMesaArchive(archivePath, cacheDir, logPath); err != nil {
		return "", err
	}
	appendStartupLog(logPath, "OpenGL: pacote de fallback extraído")
	srcDLL := filepath.Join(cacheDir, "opengl32sw.dll")
	if _, err := os.Stat(srcDLL); err != nil {
		return "", fmt.Errorf("dll extraída não encontrada: %w", err)
	}

	// 1) Tenta copiar para o diretório do executável (caminho preferencial do loader).
	if err := copyFile(srcDLL, exeDLL); err == nil {
		_ = hideFileWindows(exeDLL)
		appendStartupLog(logPath, "OpenGL: fallback copiado para o diretório do executável")
		return exeDLL, nil
	}
	// 2) Se não puder escrever ao lado do executável, usa cache local.
	cacheDLL := filepath.Join(cacheDir, "opengl32.dll")
	if err := copyFile(srcDLL, cacheDLL); err != nil {
		return "", err
	}
	_ = hideFileWindows(cacheDLL)
	appendStartupLog(logPath, "OpenGL: fallback salvo no cache local do usuário")
	return cacheDLL, nil
}

// downloadMesaArchive baixa o pacote de OpenGL por software apenas quando necessário.
func downloadMesaArchive(dst string) error {
	if fi, err := os.Stat(dst); err == nil && fi.Size() > 0 {
		return nil
	}
	const mesaURL = "https://download.qt.io/development_releases/prebuilt/llvmpipe/windows/opengl32sw-64.7z"
	resp, err := http.Get(mesaURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download falhou: status %d", resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// extractMesaArchive extrai o pacote .7z usando tar e fallback em Go puro.
func extractMesaArchive(archivePath, targetDir, logPath string) error {
	// 1) Caminho mais rápido quando o tar do sistema suporta .7z.
	cmd := exec.Command("tar", "-xf", archivePath, "-C", targetDir)
	if out, err := cmd.CombinedOutput(); err == nil {
		return nil
	} else if strings.TrimSpace(string(out)) != "" {
		appendStartupLog(logPath, "OpenGL: tar falhou: "+strings.TrimSpace(string(out)))
	} else {
		appendStartupLog(logPath, "OpenGL: tar falhou sem saída")
		appendStartupLog(logPath, "OpenGL: erro do tar: "+err.Error())
	}

	// 2) Fallback em Go puro, sem depender de utilitários do servidor.
	r, err := sevenzip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		name := filepath.Base(f.Name)
		dst := filepath.Join(targetDir, name)
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(dst, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(dst)
		if err != nil {
			rc.Close()
			return err
		}
		_, cpErr := io.Copy(out, rc)
		closeErr := out.Close()
		_ = rc.Close()
		if cpErr != nil {
			return cpErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

// copyFile copia um arquivo binário da origem para o destino.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// hideFileWindows marca o arquivo como oculto no Windows.
func hideFileWindows(path string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	cmd := exec.Command("attrib", "+h", path)
	return cmd.Run()
}
