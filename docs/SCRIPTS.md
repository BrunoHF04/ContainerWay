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

## Versão web (browser)

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

Usa `containerway-web.exe` / `containerway-web` na raiz se existir; senão `go run`.

### `scripts/web/build.bat` / `scripts/web/build.sh`

Compila o binário web na raiz do repo.

```bat
.\scripts\web\build.bat
```

```sh
./scripts/web/build.sh
```

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
