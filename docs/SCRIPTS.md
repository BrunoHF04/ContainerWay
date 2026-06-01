# Scripts do projeto

Todos os scripts executáveis ficam em **`scripts/`**. Execute-os a partir da **raiz do repositório** (os scripts web e o `build.ps1` mudam o diretório de trabalho automaticamente).

## Desktop

### `scripts/build.ps1`

Compila o app **Fyne** (Windows) e, opcionalmente, pacotes **Linux** (`.deb`, `.flatpak`) via Docker.

```powershell
.\scripts\build.ps1
.\scripts\build.ps1 -Version 1.2.3
.\scripts\build.ps1 -SkipLinux
.\scripts\build.ps1 -SkipWindows
```

**Saídas:** `ContainerWay.exe` na raiz; `dist/deb/`, `dist/flatpak/` no Linux.

**Requisitos Windows:** Go, GCC/Clang (CGO para Fyne). **Linux:** Docker Desktop em execução.

---

## Versão web (browser) — utilizador final

### `scripts/build-web.ps1`

Gera o executável **`ContainerWay Web.exe`** na raiz (Windows). Ao executar, abre o browser automaticamente. **Sem CGO.**

Regenera ícones (`.exe`, favicon, UI) e aplica ícone Windows via [go-winres](https://github.com/tc-hib/go-winres) se estiver instalado:

```powershell
go install github.com/tc-hib/go-winres@latest
.\scripts\build-web.ps1
```

Ícones (mascote em `assets/mascot-source-*.png`): `go run ./cmd/iconforge/` → `assets/containerway-*.png/.ico`, `internal/webapp/static/favicon*` e `internal/appui/window-icon.png`.

Incluído também em `.\scripts\build.ps1` (use `-SkipWeb` para omitir).

**Linux:** `.\scripts\build.ps1 -SkipWindows` gera `dist/deb-web/containerway-web_*_amd64.deb` (menu de aplicações **ContainerWay Web**).

---

## Versão web — desenvolvimento

### `scripts/web/run.bat` / `scripts/web/run.sh`

Inicia o servidor em **http://127.0.0.1:8765** (só localhost).

```bat
.\scripts\web\run.bat
.\scripts\web\run.bat 127.0.0.1:9000
```

```sh
chmod +x scripts/web/*.sh
./scripts/web/run.sh
./scripts/web/run.sh 127.0.0.1:9000
```

Prefere `ContainerWay Web.exe`, depois `containerway-web.exe`; senão `go run`.

### `scripts/web/build.bat` / `scripts/web/build.sh`

Atalhos para `build-web.ps1` (Windows) ou `go build` (Unix).

Documentação da UI web: [WEB_UI.md](WEB_UI.md).

---

## Releases

### `scripts/release/publish-github-release.ps1`

Cria ou atualiza uma release no GitHub com artefatos (`.exe`, `.deb`, `.flatpak`).

```powershell
$env:GITHUB_TOKEN = "ghp_..."
.\scripts\release\publish-github-release.ps1 -Tag v1.1.0
```

Gere os ficheiros antes com `.\scripts\build.ps1 -Version 1.1.0`.
