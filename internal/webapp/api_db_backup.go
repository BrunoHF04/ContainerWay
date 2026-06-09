package webapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"containerway/internal/dockerutil"
)

type DetectedDB struct {
	SourceType  string   `json:"sourceType"`  // "docker" | "systemd"
	Engine      string   `json:"engine"`      // "postgres" | "mysql" | "mariadb" | "sqlserver"
	Name        string   `json:"name"`        // Nome do container ou serviço systemd
	TargetID    string   `json:"targetId"`    // ID do container ou nome do serviço
	Status      string   `json:"status"`      // Estado ("running", "active", "exited", etc.)
	Image       string   `json:"image"`       // Imagem docker ou "local-service"
	DefaultUser string   `json:"defaultUser"` // Usuário padrão para login
	Port        int      `json:"port"`        // Porta padrão
	Databases   []string `json:"databases"`   // Lista de bancos lógicos detectados nesta instância
}

type DBBackupRoutine struct {
	ID         string `json:"id"`
	Engine     string `json:"engine"`     // "postgres", "mysql", "mariadb", "sqlserver"
	Type       string `json:"type"`       // "docker", "systemd"
	Target     string `json:"target"`     // Container ID/name ou nome do serviço systemd
	DB         string `json:"db"`         // Nome do banco de dados (ou vazio para todos)
	User       string `json:"user"`       // Usuário do banco
	Dest       string `json:"dest"`       // Diretório de backups no servidor
	Retention  int    `json:"retention"`  // Dias de retenção
	Cron       string `json:"cron"`       // Expressão cron
	ScriptPath string `json:"scriptPath"` // Caminho do script remoto
	BackupMode string `json:"backupMode"` // "full" ou "incremental"
	LastRun    string `json:"lastRun"`    // Última execução (timestamp)
	LastStatus string `json:"lastStatus"` // Último estado (success / error)
}

// resolveHomePath converte caminhos que começam com ~ para a pasta home remota real.
func (b *sshBundle) resolveHomePath(ctx context.Context, p string) (string, error) {
	if !strings.HasPrefix(p, "~") {
		return p, nil
	}
	stdout, _, err := b.runSSHCommand(ctx, "echo $HOME", "")
	if err != nil {
		return p, err
	}
	home := strings.TrimSpace(stdout)
	if home == "" {
		return p, fmt.Errorf("não foi possível obter o diretório home remoto")
	}
	return strings.Replace(p, "~", home, 1), nil
}

// writeRemoteFile cria e grava conteúdo de arquivo via SFTP no servidor.
func (b *sshBundle) writeRemoteFile(ctx context.Context, remotePath string, data []byte, perm os.FileMode) error {
	if b == nil || b.Sess == nil || b.Sess.SFTP == nil {
		return fmt.Errorf("SFTP indisponível nesta conexão")
	}
	// Garante diretório pai
	err := b.Sess.SFTP.MkdirAll(path.Dir(remotePath))
	if err != nil {
		return fmt.Errorf("falha ao criar pasta remota: %w", err)
	}
	f, err := b.Sess.SFTP.Create(remotePath)
	if err != nil {
		return fmt.Errorf("falha ao criar arquivo remoto: %w", err)
	}
	defer f.Close()
	_, err = f.Write(data)
	if err != nil {
		return fmt.Errorf("falha ao gravar dados remoto: %w", err)
	}
	if perm != 0 {
		err = b.Sess.SFTP.Chmod(remotePath, perm)
		if err != nil {
			return fmt.Errorf("falha ao ajustar permissões: %w", err)
		}
	}
	return nil
}

// streamSSHCommand executa um comando remoto canalizando sua saída direta para um io.Writer.
func (b *sshBundle) streamSSHCommand(ctx context.Context, cmd, input string, outStream io.Writer) error {
	if b == nil || b.Sess == nil || b.Sess.SSH == nil {
		return fmt.Errorf("sessão SSH indisponível")
	}
	sess, err := b.Sess.SSH.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()

	sess.Stdout = outStream
	var errBuf strings.Builder
	sess.Stderr = &errBuf

	stdin, err := sess.StdinPipe()
	if err != nil {
		return err
	}

	if err := sess.Start(cmd); err != nil {
		return err
	}

	if input != "" {
		_, _ = io.WriteString(stdin, input+"\n")
	}
	_ = stdin.Close()

	done := make(chan error, 1)
	go func() {
		done <- sess.Wait()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		if err != nil {
			msg := strings.TrimSpace(errBuf.String())
			if msg != "" {
				return fmt.Errorf("%s", msg)
			}
			return err
		}
		return nil
	}
}

// cleanDisplayName remove prefixos e simplifica nomes de containers (ex: Swarm/Compose).
func cleanDisplayName(name string) string {
	name = strings.TrimPrefix(name, "/")
	parts := strings.Split(name, ".")
	if len(parts) > 1 {
		return parts[0]
	}
	return name
}

// discoverLogicalDBs descobre e lista bancos de dados lógicos criados pelo usuário dentro de uma instância.
func (b *sshBundle) discoverLogicalDBs(ctx context.Context, engine, sourceType, target, user, pass string) []string {
	var cmd string
	switch engine {
	case "postgres":
		if sourceType == "docker" {
			cmd = fmt.Sprintf("docker exec -i -e PGPASSWORD=%s %s psql -U %s -t -A -c %s 2>/dev/null",
				shellQuote(pass), shellQuote(target), shellQuote(user), shellQuote("SELECT datname FROM pg_database WHERE datistemplate = false AND datallowconn = true;"))
		} else {
			cmd = fmt.Sprintf("sudo -u %s psql -t -A -c %s 2>/dev/null || psql -U %s -t -A -c %s 2>/dev/null",
				shellQuote(user), shellQuote("SELECT datname FROM pg_database WHERE datistemplate = false AND datallowconn = true;"),
				shellQuote(user), shellQuote("SELECT datname FROM pg_database WHERE datistemplate = false AND datallowconn = true;"))
		}
	case "mysql", "mariadb":
		var pFlag string
		if pass != "" {
			pFlag = "-p" + pass
		}
		if sourceType == "docker" {
			cmd = fmt.Sprintf("docker exec -i %s mysql -u%s %s -e %s -s -N 2>/dev/null",
				shellQuote(target), shellQuote(user), pFlag, shellQuote("SHOW DATABASES;"))
		} else {
			cmd = fmt.Sprintf("mysql -u%s %s -e %s -s -N 2>/dev/null",
				shellQuote(user), pFlag, shellQuote("SHOW DATABASES;"))
		}
	case "sqlserver":
		sql := "SET NOCOUNT ON; SELECT name FROM sys.databases WHERE name NOT IN ('master', 'tempdb', 'model', 'msdb');"
		if sourceType == "docker" {
			cmd = fmt.Sprintf("docker exec -i %s /opt/mssql-tools/bin/sqlcmd -S localhost -U sa -P %s -Q %s -h -1 2>/dev/null || docker exec -i %s /opt/mssql-tools18/bin/sqlcmd -S localhost -U sa -P %s -Q %s -h -1 -C 2>/dev/null",
				shellQuote(target), shellQuote(pass), shellQuote(sql),
				shellQuote(target), shellQuote(pass), shellQuote(sql))
		} else {
			cmd = fmt.Sprintf("sqlcmd -S localhost -U sa -P %s -Q %s -h -1 2>/dev/null || sqlcmd -S localhost -U sa -P %s -Q %s -h -1 -C 2>/dev/null",
				shellQuote(pass), shellQuote(sql),
				shellQuote(pass), shellQuote(sql))
		}
	default:
		return nil
	}

	stdout, _, err := b.runSSHCommand(ctx, cmd, "")
	if err != nil {
		return nil
	}

	var dbs []string
	lines := strings.Split(stdout, "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if engine == "mysql" || engine == "mariadb" {
			lower := strings.ToLower(l)
			if lower == "information_schema" || lower == "mysql" || lower == "performance_schema" || lower == "sys" {
				continue
			}
		}
		dbs = append(dbs, l)
	}
	return dbs
}

// handleDBDiscover detecta bancos de dados locais e em Docker no servidor.
func (s *Server) handleDBDiscover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	detected := make([]DetectedDB, 0)

	// 1. Escanear contêineres Docker
	if b.Sess.Docker != nil {
		containers, err := listDockerContainers(ctx, b.Sess.Docker, true, false)
		if err == nil {
			for _, c := range containers {
				imgLower := strings.ToLower(c.Image)
				var engine string
				var defaultUser string
				var port int

				switch {
				case strings.Contains(imgLower, "postgres"):
					engine = "postgres"
					defaultUser = "postgres"
					port = 5432
				case strings.Contains(imgLower, "mysql"):
					engine = "mysql"
					defaultUser = "root"
					port = 3306
				case strings.Contains(imgLower, "mariadb"):
					engine = "mariadb"
					defaultUser = "root"
					port = 3306
				case strings.Contains(imgLower, "mssql") || strings.Contains(imgLower, "sqlserver"):
					engine = "sqlserver"
					defaultUser = "sa"
					port = 1433
				}

				if engine != "" {
					item := DetectedDB{
						SourceType:  "docker",
						Engine:      engine,
						Name:        cleanDisplayName(c.Name),
						TargetID:    c.ID,
						Status:      c.State,
						Image:       c.Image,
						DefaultUser: defaultUser,
						Port:        port,
					}

					// Tenta extrair variáveis de ambiente para preenchimento de usuários e senhas customizados
					var defaultPass string
					inspect, err := b.Sess.Docker.ContainerInspect(ctx, c.IDFull)
					if err == nil && inspect.Config != nil {
						for _, env := range inspect.Config.Env {
							parts := strings.SplitN(env, "=", 2)
							if len(parts) == 2 {
								key := parts[0]
								val := parts[1]
								switch key {
								case "POSTGRES_USER":
									item.DefaultUser = val
								case "POSTGRES_PASSWORD":
									defaultPass = val
								case "MYSQL_USER":
									item.DefaultUser = val
								case "MYSQL_ROOT_PASSWORD", "MYSQL_PASSWORD":
									defaultPass = val
								case "SA_PASSWORD":
									defaultPass = val
								}
							}
						}
					}

					// Executa a autodescoberta dos bancos de dados lógicos internos
					item.Databases = b.discoverLogicalDBs(ctx, item.Engine, item.SourceType, item.TargetID, item.DefaultUser, defaultPass)

					detected = append(detected, item)
				}
			}
		}
	}

	// 2. Escanear serviços locais via Systemd e executáveis
	checkCmd := `
	echo "--- SERVICES ---"
	systemctl list-units --type=service --all 2>/dev/null | grep -E 'postgresql|mysql|mariadb|mssql-server' || true
	echo "--- BINARIES ---"
	which pg_dump 2>/dev/null || true
	which mysqldump 2>/dev/null || true
	which sqlcmd 2>/dev/null || true
	`
	stdout, _, _ := b.runSSHCommand(ctx, "sh -lc "+shellQuote(strings.TrimSpace(checkCmd)), "")

	lines := strings.Split(stdout, "\n")
	hasPgDump := false
	hasMysqlDump := false
	hasSqlCmd := false
	isServices := true

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "--- BINARIES ---" {
			isServices = false
			continue
		}
		if line == "" || strings.HasPrefix(line, "---") {
			continue
		}

		if isServices {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				svcName := parts[0]
				status := parts[2]

				var engine string
				var defaultUser string
				var port int

				switch {
				case strings.Contains(svcName, "postgresql"):
					engine = "postgres"
					defaultUser = "postgres"
					port = 5432
				case strings.Contains(svcName, "mysql"):
					engine = "mysql"
					defaultUser = "root"
					port = 3306
				case strings.Contains(svcName, "mariadb"):
					engine = "mariadb"
					defaultUser = "root"
					port = 3306
				case strings.Contains(svcName, "mssql") || strings.Contains(svcName, "sqlserver"):
					engine = "sqlserver"
					defaultUser = "sa"
					port = 1433
				}

				if engine != "" {
					item := DetectedDB{
						SourceType:  "systemd",
						Engine:      engine,
						Name:        cleanDisplayName(svcName),
						TargetID:    svcName,
						Status:      status,
						Image:       "local-service",
						DefaultUser: defaultUser,
						Port:        port,
					}
					item.Databases = b.discoverLogicalDBs(ctx, item.Engine, item.SourceType, item.TargetID, item.DefaultUser, "")
					detected = append(detected, item)
				}
			}
		} else {
			if strings.HasSuffix(line, "pg_dump") {
				hasPgDump = true
			}
			if strings.HasSuffix(line, "mysqldump") {
				hasMysqlDump = true
			}
			if strings.HasSuffix(line, "sqlcmd") {
				hasSqlCmd = true
			}
		}
	}

	// Insere fallback baseado nos utilitários caso o serviço systemd não tenha sido detectado diretamente
	hasPgSvc := false
	hasMysqlSvc := false
	hasMssqlSvc := false
	for _, d := range detected {
		if d.SourceType == "systemd" {
			switch d.Engine {
			case "postgres":
				hasPgSvc = true
			case "mysql", "mariadb":
				hasMysqlSvc = true
			case "sqlserver":
				hasMssqlSvc = true
			}
		}
	}

	if hasPgDump && !hasPgSvc {
		item := DetectedDB{
			SourceType:  "systemd",
			Engine:      "postgres",
			Name:        "postgresql (binário detectado)",
			TargetID:    "postgresql",
			Status:      "unknown",
			Image:       "local-service",
			DefaultUser: "postgres",
			Port:        5432,
		}
		item.Databases = b.discoverLogicalDBs(ctx, item.Engine, item.SourceType, item.TargetID, item.DefaultUser, "")
		detected = append(detected, item)
	}
	if hasMysqlDump && !hasMysqlSvc {
		item := DetectedDB{
			SourceType:  "systemd",
			Engine:      "mysql",
			Name:        "mysql (binário detectado)",
			TargetID:    "mysql",
			Status:      "unknown",
			Image:       "local-service",
			DefaultUser: "root",
			Port:        3306,
		}
		item.Databases = b.discoverLogicalDBs(ctx, item.Engine, item.SourceType, item.TargetID, item.DefaultUser, "")
		detected = append(detected, item)
	}
	if hasSqlCmd && !hasMssqlSvc {
		item := DetectedDB{
			SourceType:  "systemd",
			Engine:      "sqlserver",
			Name:        "mssql-server (binário detectado)",
			TargetID:    "mssql-server",
			Status:      "unknown",
			Image:       "local-service",
			DefaultUser: "sa",
			Port:        1433,
		}
		item.Databases = b.discoverLogicalDBs(ctx, item.Engine, item.SourceType, item.TargetID, item.DefaultUser, "")
		detected = append(detected, item)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"databases": detected,
		"count":     len(detected),
	})
}

// handleDBBackupRun executa backup integral e realiza download ou salvamento remoto.
func (s *Server) handleDBBackupRun(w http.ResponseWriter, r *http.Request) {
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}

	var body struct {
		Engine   string `json:"engine"`   // postgres, mysql, mariadb, sqlserver
		Type     string `json:"type"`     // docker, systemd
		Target   string `json:"target"`   // container ID/name ou serviço systemd
		DB       string `json:"db"`       // nome do banco de dados (opcional)
		User     string `json:"user"`     // usuário de acesso
		Pass     string `json:"pass"`     // senha de acesso
		DestType string `json:"destType"` // download, ssh
		DestPath string `json:"destPath"` // diretório no servidor (se ssh)
	}

	if r.Method == http.MethodGet {
		body.Engine = r.URL.Query().Get("engine")
		body.Type = r.URL.Query().Get("type")
		body.Target = r.URL.Query().Get("target")
		body.DB = r.URL.Query().Get("db")
		body.User = r.URL.Query().Get("user")
		body.Pass = r.URL.Query().Get("pass")
		body.DestType = r.URL.Query().Get("destType")
		body.DestPath = r.URL.Query().Get("destPath")
	} else if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "dados da requisição inválidos"})
			return
		}
	} else {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}

	if strings.TrimSpace(body.Engine) == "" || strings.TrimSpace(body.Target) == "" || strings.TrimSpace(body.DestType) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "campos obrigatórios ausentes"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()

	var cmd string
	var filename string

	switch body.Engine {
	case "postgres":
		if body.DB != "" {
			filename = fmt.Sprintf("backup_%s.sql.gz", body.DB)
			if body.Type == "docker" {
				cmd = fmt.Sprintf("docker exec -i -e PGPASSWORD=%s %s pg_dump -U %s -d %s | gzip",
					shellQuote(body.Pass), shellQuote(body.Target), shellQuote(body.User), shellQuote(body.DB))
			} else {
				cmd = fmt.Sprintf("PGPASSWORD=%s pg_dump -U %s -h localhost -d %s | gzip",
					shellQuote(body.Pass), shellQuote(body.User), shellQuote(body.DB))
			}
		} else {
			filename = "backup_all.sql.gz"
			if body.Type == "docker" {
				cmd = fmt.Sprintf("docker exec -i -e PGPASSWORD=%s %s pg_dumpall -U %s | gzip",
					shellQuote(body.Pass), shellQuote(body.Target), shellQuote(body.User))
			} else {
				cmd = fmt.Sprintf("PGPASSWORD=%s pg_dumpall -U %s -h localhost | gzip",
					shellQuote(body.Pass), shellQuote(body.User))
			}
		}

	case "mysql", "mariadb":
		if body.DB != "" {
			filename = fmt.Sprintf("backup_%s.sql.gz", body.DB)
			if body.Type == "docker" {
				cmd = fmt.Sprintf("docker exec -i %s mysqldump -u%s -p%s %s | gzip",
					shellQuote(body.Target), shellQuote(body.User), shellQuote(body.Pass), shellQuote(body.DB))
			} else {
				cmd = fmt.Sprintf("mysqldump -u%s -p%s -h localhost %s | gzip",
					shellQuote(body.User), shellQuote(body.Pass), shellQuote(body.DB))
			}
		} else {
			filename = "backup_all.sql.gz"
			if body.Type == "docker" {
				cmd = fmt.Sprintf("docker exec -i %s mysqldump -u%s -p%s --all-databases | gzip",
					shellQuote(body.Target), shellQuote(body.User), shellQuote(body.Pass))
			} else {
				cmd = fmt.Sprintf("mysqldump -u%s -p%s -h localhost --all-databases | gzip",
					shellQuote(body.User), shellQuote(body.Pass))
			}
		}

	case "sqlserver":
		if body.DB == "" {
			body.DB = "master"
		}
		filename = fmt.Sprintf("backup_%s.bak.gz", body.DB)
		tempBak := fmt.Sprintf("/var/opt/mssql/data/backup_temp_%d.bak", time.Now().Unix())

		if body.Type == "docker" {
			backupSQL := fmt.Sprintf("BACKUP DATABASE [%s] TO DISK='%s' WITH FORMAT, COPY_ONLY", body.DB, tempBak)
			sqlcmd := fmt.Sprintf("docker exec -i %s /opt/mssql-tools/bin/sqlcmd -S localhost -U %s -P %s -Q %s",
				shellQuote(body.Target), shellQuote(body.User), shellQuote(body.Pass), shellQuote(backupSQL))

			_, stderr, err := b.runSSHCommand(ctx, sqlcmd, "")
			if err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("falha no backup SQL Server: %v (stderr: %s)", err, stderr)})
				return
			}
			cmd = fmt.Sprintf("docker exec -i %s cat %s | gzip && docker exec -i %s rm -f %s >/dev/null 2>&1",
				shellQuote(body.Target), tempBak, shellQuote(body.Target), tempBak)
		} else {
			backupSQL := fmt.Sprintf("BACKUP DATABASE [%s] TO DISK='%s' WITH FORMAT, COPY_ONLY", body.DB, tempBak)
			sqlcmd := fmt.Sprintf("sqlcmd -S localhost -U %s -P %s -Q %s",
				shellQuote(body.User), shellQuote(body.Pass), shellQuote(backupSQL))

			_, stderr, err := b.runSSHCommand(ctx, sqlcmd, "")
			if err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("falha no backup SQL Server: %v (stderr: %s)", err, stderr)})
				return
			}
			cmd = fmt.Sprintf("cat %s | gzip && rm -f %s >/dev/null 2>&1", tempBak, tempBak)
		}
	}

	if body.DestType == "download" {
		w.Header().Set("Content-Type", "application/x-gzip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		w.WriteHeader(http.StatusOK)

		_ = b.streamSSHCommand(ctx, cmd, "", w)
	} else if body.DestType == "ssh" {
		if strings.TrimSpace(body.DestPath) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "caminho de destino remoto obrigatório"})
			return
		}

		destFolder := path.Dir(body.DestPath)
		mkdirCmd := fmt.Sprintf("mkdir -p %s", shellQuote(destFolder))
		_, _, _ = b.runSSHCommand(ctx, mkdirCmd, "")

		sshCmd := fmt.Sprintf("%s > %s", cmd, shellQuote(body.DestPath))
		_, stderr, err := b.runSSHCommand(ctx, sshCmd, "")
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("falha ao gravar arquivo remoto: %v (stderr: %s)", err, stderr)})
			return
		}

		sizeCmd := fmt.Sprintf("stat -c %%s %s 2>/dev/null || wc -c < %s 2>/dev/null", shellQuote(body.DestPath), shellQuote(body.DestPath))
		stdout, _, _ := b.runSSHCommand(ctx, sizeCmd, "")
		sizeBytes, _ := strconv.ParseUint(strings.TrimSpace(stdout), 10, 64)

		writeJSON(w, http.StatusOK, map[string]any{
			"status":       "success",
			"bytesWritten": sizeBytes,
			"sizeHuman":    dockerutil.FormatBytes(sizeBytes),
		})
	} else {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tipo de destino inválido"})
	}
}

// handleDBBackupRoutines gerencia a listagem, criação e exclusão de rotinas.
func (s *Server) handleDBBackupRoutines(w http.ResponseWriter, r *http.Request) {
	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listRoutines(w, r, b)
	case http.MethodPost:
		s.createRoutine(w, r, b)
	case http.MethodDelete:
		s.deleteRoutine(w, r, b)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
	}
}

// listRoutines devolve a lista de rotinas a partir do crontab.
func (s *Server) listRoutines(w http.ResponseWriter, r *http.Request, b *sshBundle) {
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	stdout, stderr, err := b.runSSHCommand(ctx, "crontab -l", "")
	if err != nil {
		if strings.Contains(strings.ToLower(stderr), "no crontab") || strings.Contains(strings.ToLower(err.Error()), "no crontab") {
			writeJSON(w, http.StatusOK, map[string]any{"routines": []DBBackupRoutine{}})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("falha ao ler crontab: %v (stderr: %s)", err, stderr)})
		return
	}

	lines := strings.Split(stdout, "\n")
	routines := make([]DBBackupRoutine, 0)

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "# CW-DB-BACKUP:") {
			id := strings.TrimPrefix(line, "# CW-DB-BACKUP:")
			if i+1 < len(lines) {
				cronLine := strings.TrimSpace(lines[i+1])
				cronExpr, scriptPath := parseCronLine(cronLine)
				if scriptPath != "" {
					routine, err := s.readRoutineMetadata(ctx, b, id, cronExpr, scriptPath)
					if err == nil {
						routines = append(routines, routine)
					}
				}
				i++
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"routines": routines})
}

// parseCronLine faz parse da linha cron extraindo a expressão cron (5 campos) e o caminho do script shell.
func parseCronLine(line string) (string, string) {
	parts := strings.Fields(line)
	if len(parts) < 6 {
		return "", ""
	}
	cronExpr := strings.Join(parts[0:5], " ")
	var scriptPath string
	for _, part := range parts[5:] {
		if strings.HasSuffix(part, ".sh") {
			scriptPath = part
			break
		}
	}
	return cronExpr, scriptPath
}

// readRoutineMetadata lê os metadados JSON do script shell e o estado do log no servidor remoto.
func (s *Server) readRoutineMetadata(ctx context.Context, b *sshBundle, id, cronExpr, scriptPath string) (DBBackupRoutine, error) {
	routine := DBBackupRoutine{
		ID:         id,
		Cron:       cronExpr,
		ScriptPath: scriptPath,
	}

	cmd := fmt.Sprintf("grep -m 1 'CW-ROUTINE-METADATA:' %s", shellQuote(scriptPath))
	stdout, _, err := b.runSSHCommand(ctx, cmd, "")
	if err == nil {
		stdout = strings.TrimSpace(stdout)
		idx := strings.Index(stdout, "CW-ROUTINE-METADATA:")
		if idx >= 0 {
			jsonStr := stdout[idx+len("CW-ROUTINE-METADATA:"):]
			var meta struct {
				Engine     string `json:"engine"`
				Type       string `json:"type"`
				Target     string `json:"target"`
				DB         string `json:"db"`
				User       string `json:"user"`
				Dest       string `json:"dest"`
				Retention  int    `json:"retention"`
				BackupMode string `json:"backupMode"`
			}
			if json.Unmarshal([]byte(jsonStr), &meta) == nil {
				routine.Engine = meta.Engine
				routine.Type = meta.Type
				routine.Target = meta.Target
				routine.DB = meta.DB
				routine.User = meta.User
				routine.Dest = meta.Dest
				routine.Retention = meta.Retention
				routine.BackupMode = meta.BackupMode
			}
		}
	}

	logPath := strings.TrimSuffix(scriptPath, ".sh") + ".log"
	statCmd := fmt.Sprintf("stat -c '%%y' %s 2>/dev/null || date -r %s 2>/dev/null", shellQuote(logPath), shellQuote(logPath))
	logStat, _, err := b.runSSHCommand(ctx, statCmd, "")
	if err == nil && strings.TrimSpace(logStat) != "" {
		routine.LastRun = strings.TrimSpace(logStat)
		tailCmd := fmt.Sprintf("tail -n 15 %s 2>/dev/null", shellQuote(logPath))
		logTail, _, _ := b.runSSHCommand(ctx, tailCmd, "")
		
		switch {
		case strings.Contains(logTail, "completed successfully") || strings.Contains(logTail, "sucesso"):
			routine.LastStatus = "success"
		case strings.Contains(logTail, "ERROR") || strings.Contains(logTail, "ERRO") || strings.Contains(logTail, "fail") || strings.Contains(logTail, "falha"):
			routine.LastStatus = "error"
		case logTail != "":
			routine.LastStatus = "running"
		default:
			routine.LastStatus = "unknown"
		}
	} else {
		routine.LastRun = "nunca executado"
		routine.LastStatus = "pending"
	}

	return routine, nil
}

// createRoutine cria um script shell executável e agenda no crontab remoto.
func (s *Server) createRoutine(w http.ResponseWriter, r *http.Request, b *sshBundle) {
	var body struct {
		Cron       string `json:"cron"`
		Engine     string `json:"engine"`     // postgres, mysql, mariadb, sqlserver
		Type       string `json:"type"`       // docker, systemd
		Target     string `json:"target"`     // container ID/name ou serviço systemd
		DB         string `json:"db"`         // banco de dados (opcional)
		User       string `json:"user"`       // usuário
		Pass       string `json:"pass"`       // senha
		Dest       string `json:"dest"`       // diretório destino remoto
		Retention  int    `json:"retention"`  // dias retenção
		BackupMode string `json:"backupMode"` // "full" ou "incremental"
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "dados da requisição inválidos"})
		return
	}
	if strings.TrimSpace(body.Cron) == "" || strings.TrimSpace(body.Engine) == "" || strings.TrimSpace(body.Target) == "" || strings.TrimSpace(body.Dest) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "campos obrigatórios ausentes"})
		return
	}

	if body.BackupMode == "" {
		body.BackupMode = "full"
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	id := fmt.Sprintf("db-backup-%d", time.Now().Unix())
	scriptPath := fmt.Sprintf("~/.containerway/routines/backup_%s.sh", id)
	logPath := fmt.Sprintf("~/.containerway/routines/backup_%s.log", id)

	_, _, _ = b.runSSHCommand(ctx, "mkdir -p ~/.containerway/routines", "")

	realScriptPath, _ := b.resolveHomePath(ctx, scriptPath)
	realLogPath, _ := b.resolveHomePath(ctx, logPath)

	scriptContent := s.generateBackupScript(body.Engine, body.Type, body.Target, body.DB, body.User, body.Pass, body.Dest, body.Retention, id, body.BackupMode)

	err := b.writeRemoteFile(ctx, realScriptPath, []byte(scriptContent), 0700)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("falha ao gravar script remoto: %v", err)})
		return
	}

	currentCron, _, _ := b.runSSHCommand(ctx, "crontab -l", "")
	lines := strings.Split(currentCron, "\n")
	var newLines []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			newLines = append(newLines, l)
		}
	}

	newLines = append(newLines, fmt.Sprintf("# CW-DB-BACKUP:%s", id))
	newLines = append(newLines, fmt.Sprintf("%s /bin/bash %s >> %s 2>&1", body.Cron, realScriptPath, realLogPath))

	newCronContent := strings.Join(newLines, "\n") + "\n"
	tempCronPath := "~/.containerway/temp_cron"
	realTempCronPath, _ := b.resolveHomePath(ctx, tempCronPath)

	err = b.writeRemoteFile(ctx, realTempCronPath, []byte(newCronContent), 0600)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("falha ao criar arquivo temporário: %v", err)})
		return
	}

	_, stderr, err := b.runSSHCommand(ctx, fmt.Sprintf("crontab %s && rm -f %s", shellQuote(realTempCronPath), shellQuote(realTempCronPath)), "")
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("falha ao registrar crontab: %v (stderr: %s)", err, stderr)})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "success",
		"id":     id,
	})
}

// deleteRoutine apaga a tarefa cron e os arquivos do script e logs no servidor remoto.
func (s *Server) deleteRoutine(w http.ResponseWriter, r *http.Request, b *sshBundle) {
	id := r.URL.Query().Get("id")
	if strings.TrimSpace(id) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id obrigatório"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	currentCron, _, _ := b.runSSHCommand(ctx, "crontab -l", "")
	lines := strings.Split(currentCron, "\n")
	var newLines []string

	skipNext := false
	scriptPath := ""

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == fmt.Sprintf("# CW-DB-BACKUP:%s", id) {
			if i+1 < len(lines) {
				_, scriptPath = parseCronLine(lines[i+1])
			}
			skipNext = true
			continue
		}
		if skipNext {
			skipNext = false
			continue
		}
		if line != "" {
			newLines = append(newLines, lines[i])
		}
	}

	newCronContent := strings.Join(newLines, "\n") + "\n"
	tempCronPath := "~/.containerway/temp_cron"
	realTempCronPath, _ := b.resolveHomePath(ctx, tempCronPath)

	err := b.writeRemoteFile(ctx, realTempCronPath, []byte(newCronContent), 0600)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("falha ao criar arquivo temporário: %v", err)})
		return
	}

	var cronCmd string
	if len(newLines) == 0 {
		cronCmd = fmt.Sprintf("crontab -r 2>/dev/null || true; rm -f %s", shellQuote(realTempCronPath))
	} else {
		cronCmd = fmt.Sprintf("crontab %s && rm -f %s", shellQuote(realTempCronPath), shellQuote(realTempCronPath))
	}

	_, stderr, err := b.runSSHCommand(ctx, cronCmd, "")
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("falha ao redefinir crontab: %v (stderr: %s)", err, stderr)})
		return
	}

	if scriptPath != "" {
		logPath := strings.TrimSuffix(scriptPath, ".sh") + ".log"
		rmCmd := fmt.Sprintf("rm -f %s %s", shellQuote(scriptPath), shellQuote(logPath))
		_, _, _ = b.runSSHCommand(ctx, rmCmd, "")
	} else {
		fallbackScriptPath := fmt.Sprintf("~/.containerway/routines/backup_%s.sh", id)
		fallbackLogPath := fmt.Sprintf("~/.containerway/routines/backup_%s.log", id)
		realFScript, _ := b.resolveHomePath(ctx, fallbackScriptPath)
		realFLog, _ := b.resolveHomePath(ctx, fallbackLogPath)
		rmCmd := fmt.Sprintf("rm -f %s %s", shellQuote(realFScript), shellQuote(realFLog))
		_, _, _ = b.runSSHCommand(ctx, rmCmd, "")
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

// generateBackupScript constrói o script bash de backup.
func (s *Server) generateBackupScript(engine, sourceType, target, db, user, pass, dest string, retention int, routineID string, backupMode string) string {
	var sb strings.Builder
	sb.WriteString("#!/bin/bash\n")
	sb.WriteString(fmt.Sprintf("# CW-ROUTINE-METADATA: {\"id\":\"%s\",\"engine\":\"%s\",\"type\":\"%s\",\"target\":\"%s\",\"db\":\"%s\",\"user\":\"%s\",\"dest\":\"%s\",\"retention\":%d,\"backupMode\":\"%s\"}\n\n",
		routineID, engine, sourceType, target, db, user, dest, retention, backupMode))

	sb.WriteString("# Configurações básicas\n")
	sb.WriteString(fmt.Sprintf("BACKUP_DIR=%s\n", shellQuote(dest)))
	sb.WriteString("mkdir -p \"$BACKUP_DIR\"\n")
	sb.WriteString("TIMESTAMP=$(date +\"%Y%m%d_%H%M%S\")\n\n")

	sb.WriteString(fmt.Sprintf("echo \"[$(date)] Iniciando rotina de backup (%s)...\"\n\n", backupMode))

	isIncremental := backupMode == "incremental"
	suffix := ""
	if isIncremental {
		suffix = "_incremental"
	}

	var filename string
	var backupCmd string
	isSQLServer := engine == "sqlserver"

	switch engine {
	case "postgres":
		if db != "" {
			filename = fmt.Sprintf("backup_%s_${TIMESTAMP}%s.sql.gz", db, suffix)
			if sourceType == "docker" {
				backupCmd = fmt.Sprintf("docker exec -i -e PGPASSWORD=%s %s pg_dump -U %s -d %s | gzip",
					shellQuote(pass), shellQuote(target), shellQuote(user), shellQuote(db))
			} else {
				backupCmd = fmt.Sprintf("PGPASSWORD=%s pg_dump -U %s -h localhost -d %s | gzip",
					shellQuote(pass), shellQuote(user), shellQuote(db))
			}
		} else {
			filename = fmt.Sprintf("backup_all_${TIMESTAMP}%s.sql.gz", suffix)
			if sourceType == "docker" {
				backupCmd = fmt.Sprintf("docker exec -i -e PGPASSWORD=%s %s pg_dumpall -U %s | gzip",
					shellQuote(pass), shellQuote(target), shellQuote(user))
			} else {
				backupCmd = fmt.Sprintf("PGPASSWORD=%s pg_dumpall -U %s -h localhost | gzip",
					shellQuote(pass), shellQuote(user))
			}
		}

	case "mysql", "mariadb":
		if db != "" {
			filename = fmt.Sprintf("backup_%s_${TIMESTAMP}%s.sql.gz", db, suffix)
			if sourceType == "docker" {
				backupCmd = fmt.Sprintf("docker exec -i %s mysqldump -u%s -p%s %s | gzip",
					shellQuote(target), shellQuote(user), shellQuote(pass), shellQuote(db))
			} else {
				backupCmd = fmt.Sprintf("mysqldump -u%s -p%s -h localhost %s | gzip",
					shellQuote(user), shellQuote(pass), shellQuote(db))
			}
		} else {
			filename = fmt.Sprintf("backup_all_${TIMESTAMP}%s.sql.gz", suffix)
			if sourceType == "docker" {
				backupCmd = fmt.Sprintf("docker exec -i %s mysqldump -u%s -p%s --all-databases | gzip",
					shellQuote(target), shellQuote(user), shellQuote(pass))
			} else {
				backupCmd = fmt.Sprintf("mysqldump -u%s -p%s -h localhost --all-databases | gzip",
					shellQuote(user), shellQuote(pass))
			}
		}

	case "sqlserver":
		if db == "" {
			db = "master"
		}
		filename = fmt.Sprintf("backup_%s_${TIMESTAMP}%s.bak.gz", db, suffix)
		tempBakInside := fmt.Sprintf("/var/opt/mssql/data/backup_%s_%s.bak", db, routineID)
		tempBakHost := fmt.Sprintf("/var/opt/mssql/data/backup_%s_%s.bak", db, routineID)

		var withOptions string
		if isIncremental {
			withOptions = "WITH DIFFERENTIAL, FORMAT, INIT"
		} else {
			withOptions = "WITH FORMAT, INIT"
		}

		if sourceType == "docker" {
			sb.WriteString(fmt.Sprintf("echo \"[$(date)] Executando BACKUP DATABASE (%s) no SQL Server Docker...\"\n", backupMode))
			sb.WriteString(fmt.Sprintf("docker exec -i %s /opt/mssql-tools/bin/sqlcmd -S localhost -U %s -P %s -Q \"BACKUP DATABASE [%s] TO DISK='%s' %s\" >> \"$BACKUP_DIR/backup_%s_%s.log\" 2>&1\n",
				shellQuote(target), shellQuote(user), shellQuote(pass), shellQuote(db), tempBakInside, withOptions, db, routineID))
			sb.WriteString("EC=$?\n")
			sb.WriteString("if [ $EC -eq 0 ]; then\n")
			sb.WriteString(fmt.Sprintf("  docker exec -i %s cat %s | gzip > \"$BACKUP_DIR/%s\"\n", shellQuote(target), tempBakInside, filename))
			sb.WriteString(fmt.Sprintf("  docker exec -i %s rm -f %s\n", shellQuote(target), tempBakInside))
			sb.WriteString("else\n")
			sb.WriteString("  echo \"[$(date)] ERRO: Comando BACKUP DATABASE falhou.\"\n")
			sb.WriteString("  exit 1\n")
			sb.WriteString("fi\n\n")
		} else {
			sb.WriteString(fmt.Sprintf("echo \"[$(date)] Executando BACKUP DATABASE (%s) no SQL Server local...\"\n", backupMode))
			sb.WriteString(fmt.Sprintf("sqlcmd -S localhost -U %s -P %s -Q \"BACKUP DATABASE [%s] TO DISK='%s' %s\" >> \"$BACKUP_DIR/backup_%s_%s.log\" 2>&1\n",
				shellQuote(user), shellQuote(pass), shellQuote(db), tempBakHost, withOptions, db, routineID))
			sb.WriteString("EC=$?\n")
			sb.WriteString("if [ $EC -eq 0 ]; then\n")
			sb.WriteString(fmt.Sprintf("  cat %s | gzip > \"$BACKUP_DIR/%s\"\n", tempBakHost, filename))
			sb.WriteString(fmt.Sprintf("  rm -f %s\n", tempBakHost))
			sb.WriteString("else\n")
			sb.WriteString("  echo \"[$(date)] ERRO: Comando BACKUP DATABASE falhou.\"\n")
			sb.WriteString("  exit 1\n")
			sb.WriteString("fi\n\n")
		}
	}

	if !isSQLServer {
		sb.WriteString(fmt.Sprintf("echo \"[$(date)] Executando dump do banco (%s): %s\"\n", backupMode, engine))
		sb.WriteString(fmt.Sprintf("%s > \"$BACKUP_DIR/%s\"\n", backupCmd, filename))
		sb.WriteString("EC=${PIPESTATUS[0]}\n")
		sb.WriteString("if [ $EC -ne 0 ]; then\n")
		sb.WriteString("  echo \"[$(date)] ERRO: Dump falhou com código $EC\"\n")
		sb.WriteString("  rm -f \"$BACKUP_DIR/$filename\"\n")
		sb.WriteString("  exit 1\n")
		sb.WriteString("fi\n\n")
	}

	sb.WriteString("echo \"[$(date)] Backup gravado com sucesso em: $BACKUP_DIR/\"\n")

	if retention > 0 {
		sb.WriteString(fmt.Sprintf("\necho \"[$(date)] Executando limpeza de backups com mais de %d dias...\"\n", retention))
		sb.WriteString(fmt.Sprintf("find \"$BACKUP_DIR\" -name \"backup_*\" -mtime +%d -delete 2>/dev/null\n", retention))
	}

	sb.WriteString("\necho \"[$(date)] Rotina de backup concluída com sucesso!\"\n")

	return sb.String()
}

// handleDBBackupRestore executa a restauração de um backup de banco de dados (completo ou lógico específico).
func (s *Server) handleDBBackupRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "método não permitido"})
		return
	}

	_, b, ok := s.requireSSH(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Minute)
	defer cancel()

	var engine, sourceType, target, db, user, pass, srcType, sourcePath string
	var uploadFile io.Reader
	var uploadFilename string

	// Verifica se a requisição é multipart (upload de arquivo) ou JSON simples
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		err := r.ParseMultipartForm(100 << 20) // Max 100MB na memória
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "falha ao ler multipart form"})
			return
		}
		engine = r.FormValue("engine")
		sourceType = r.FormValue("type")
		target = r.FormValue("target")
		db = r.FormValue("db")
		user = r.FormValue("user")
		pass = r.FormValue("pass")
		srcType = r.FormValue("sourceType")
		sourcePath = r.FormValue("sourcePath")

		file, header, err := r.FormFile("file")
		if err == nil {
			defer file.Close()
			uploadFile = file
			uploadFilename = header.Filename
		}
	} else {
		var req struct {
			Engine     string `json:"engine"`
			Type       string `json:"type"`
			Target     string `json:"target"`
			DB         string `json:"db"`
			User       string `json:"user"`
			Pass       string `json:"pass"`
			SourceType string `json:"sourceType"`
			SourcePath string `json:"sourcePath"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "dados da requisição inválidos"})
			return
		}
		engine = req.Engine
		sourceType = req.Type
		target = req.Target
		db = req.DB
		user = req.User
		pass = req.Pass
		srcType = req.SourceType
		sourcePath = req.SourcePath
	}

	if engine == "" || target == "" || srcType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "campos obrigatórios ausentes"})
		return
	}

	var remoteFilePath string
	var isTempFile bool

	if srcType == "upload" {
		if uploadFile == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "arquivo de backup ausente no upload"})
			return
		}
		// Grava o arquivo temporário no servidor remoto via SFTP
		tempID := time.Now().UnixNano()
		ext := ".sql.gz"
		if strings.Contains(strings.ToLower(uploadFilename), ".bak") {
			ext = ".bak.gz"
		}
		remoteFilePath = fmt.Sprintf("/tmp/cw_restore_temp_%d%s", tempID, ext)
		isTempFile = true

		// Transmitir arquivo por SFTP
		realRemotePath, _ := b.resolveHomePath(ctx, remoteFilePath)
		sftpFile, err := b.Sess.SFTP.Create(realRemotePath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("falha ao criar arquivo de upload remoto: %v", err)})
			return
		}
		defer sftpFile.Close()

		_, err = io.Copy(sftpFile, uploadFile)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("falha ao transmitir arquivo por SFTP: %v", err)})
			return
		}
		remoteFilePath = realRemotePath
	} else {
		// Carrega do servidor
		if sourcePath == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "caminho do arquivo de origem SSH obrigatório"})
			return
		}
		var err error
		remoteFilePath, err = b.resolveHomePath(ctx, sourcePath)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("caminho inválido: %v", err)})
			return
		}
	}

	// Garante remoção do arquivo temporário se necessário
	defer func() {
		if isTempFile {
			_, _, _ = b.runSSHCommand(context.Background(), fmt.Sprintf("rm -f %s", shellQuote(remoteFilePath)), "")
		}
	}()

	// Comando de importação/restauração
	var cmd string
	isGzip := strings.HasSuffix(remoteFilePath, ".gz")

	switch engine {
	case "postgres":
		// Garante a existência do banco de dados lógico caso tenha sido fornecido
		if db != "" {
			var createCmd string
			if sourceType == "docker" {
				createCmd = fmt.Sprintf("docker exec -i -e PGPASSWORD=%s %s psql -U %s -c \"CREATE DATABASE %s;\" 2>/dev/null || true",
					shellQuote(pass), shellQuote(target), shellQuote(user), shellQuote(db))
			} else {
				createCmd = fmt.Sprintf("PGPASSWORD=%s psql -U %s -h localhost -c \"CREATE DATABASE %s;\" 2>/dev/null || true",
					shellQuote(pass), shellQuote(user), shellQuote(db))
			}
			_, _, _ = b.runSSHCommand(ctx, createCmd, "")
		}

		// Comando para restaurar
		var psqlCmd string
		if db != "" {
			if sourceType == "docker" {
				psqlCmd = fmt.Sprintf("docker exec -i -e PGPASSWORD=%s %s psql -U %s -d %s",
					shellQuote(pass), shellQuote(target), shellQuote(user), shellQuote(db))
			} else {
				psqlCmd = fmt.Sprintf("PGPASSWORD=%s psql -U %s -h localhost -d %s",
					shellQuote(pass), shellQuote(user), shellQuote(db))
			}
		} else {
			if sourceType == "docker" {
				psqlCmd = fmt.Sprintf("docker exec -i -e PGPASSWORD=%s %s psql -U %s",
					shellQuote(pass), shellQuote(target), shellQuote(user))
			} else {
				psqlCmd = fmt.Sprintf("PGPASSWORD=%s psql -U %s -h localhost",
					shellQuote(pass), shellQuote(user))
			}
		}

		if isGzip {
			cmd = fmt.Sprintf("gunzip -c %s | %s", shellQuote(remoteFilePath), psqlCmd)
		} else {
			cmd = fmt.Sprintf("cat %s | %s", shellQuote(remoteFilePath), psqlCmd)
		}

	case "mysql", "mariadb":
		// Garante a existência do banco
		if db != "" {
			var createCmd string
			if sourceType == "docker" {
				createCmd = fmt.Sprintf("docker exec -i %s mysql -u%s -p%s -e \"CREATE DATABASE IF NOT EXISTS %s;\" 2>/dev/null || true",
					shellQuote(target), shellQuote(user), shellQuote(pass), shellQuote(db))
			} else {
				createCmd = fmt.Sprintf("mysql -u%s -p%s -h localhost -e \"CREATE DATABASE IF NOT EXISTS %s;\" 2>/dev/null || true",
					shellQuote(user), shellQuote(pass), shellQuote(db))
			}
			_, _, _ = b.runSSHCommand(ctx, createCmd, "")
		}

		var mysqlCmd string
		if db != "" {
			if sourceType == "docker" {
				mysqlCmd = fmt.Sprintf("docker exec -i %s mysql -u%s -p%s %s",
					shellQuote(target), shellQuote(user), shellQuote(pass), shellQuote(db))
			} else {
				mysqlCmd = fmt.Sprintf("mysql -u%s -p%s -h localhost %s",
					shellQuote(user), shellQuote(pass), shellQuote(db))
			}
		} else {
			if sourceType == "docker" {
				mysqlCmd = fmt.Sprintf("docker exec -i %s mysql -u%s -p%s",
					shellQuote(target), shellQuote(user), shellQuote(pass))
			} else {
				mysqlCmd = fmt.Sprintf("mysql -u%s -p%s -h localhost",
					shellQuote(user), shellQuote(pass))
			}
		}

		if isGzip {
			cmd = fmt.Sprintf("gunzip -c %s | %s", shellQuote(remoteFilePath), mysqlCmd)
		} else {
			cmd = fmt.Sprintf("cat %s | %s", shellQuote(remoteFilePath), mysqlCmd)
		}

	case "sqlserver":
		if db == "" {
			db = "master"
		}

		// Para o SQL Server, o arquivo .bak precisa estar no local acessível pelo container
		// Se for docker, copiamos o arquivo para dentro do container
		var bakPathInDB string
		if sourceType == "docker" {
			bakPathInDB = fmt.Sprintf("/var/opt/mssql/data/restore_temp_%d.bak", time.Now().Unix())
			var copyCmd string
			if isGzip {
				copyCmd = fmt.Sprintf("gunzip -c %s | docker exec -i %s sh -c 'cat > %s'", shellQuote(remoteFilePath), shellQuote(target), bakPathInDB)
			} else {
				copyCmd = fmt.Sprintf("docker cp %s %s:%s", shellQuote(remoteFilePath), shellQuote(target), bakPathInDB)
			}
			_, stderr, err := b.runSSHCommand(ctx, copyCmd, "")
			if err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("falha ao copiar arquivo .bak para o container: %v (stderr: %s)", err, stderr)})
				return
			}
			defer func() {
				_, _, _ = b.runSSHCommand(context.Background(), fmt.Sprintf("docker exec -i %s rm -f %s", shellQuote(target), bakPathInDB), "")
			}()
		} else {
			bakPathInDB = remoteFilePath
			if isGzip {
				// Se for local e gzip, precisamos descompactar no host local
				tempBak := fmt.Sprintf("/tmp/restore_temp_%d.bak", time.Now().Unix())
				unzipCmd := fmt.Sprintf("gunzip -c %s > %s", shellQuote(remoteFilePath), shellQuote(tempBak))
				_, stderr, err := b.runSSHCommand(ctx, unzipCmd, "")
				if err != nil {
					writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("falha ao descompactar backup .bak.gz: %v (stderr: %s)", err, stderr)})
					return
				}
				bakPathInDB = tempBak
				defer func() {
					_, _, _ = b.runSSHCommand(context.Background(), fmt.Sprintf("rm -f %s", shellQuote(bakPathInDB)), "")
				}()
			}
		}

		// Executa RESTORE DATABASE
		restoreSQL := fmt.Sprintf("RESTORE DATABASE [%s] FROM DISK='%s' WITH REPLACE", db, bakPathInDB)

		if sourceType == "docker" {
			cmd = fmt.Sprintf("docker exec -i %s /opt/mssql-tools/bin/sqlcmd -S localhost -U %s -P %s -Q %s || docker exec -i %s /opt/mssql-tools18/bin/sqlcmd -S localhost -U %s -P %s -Q %s -h -1 -C",
				shellQuote(target), shellQuote(user), shellQuote(pass), shellQuote(restoreSQL),
				shellQuote(target), shellQuote(user), shellQuote(pass), shellQuote(restoreSQL))
		} else {
			cmd = fmt.Sprintf("sqlcmd -S localhost -U %s -P %s -Q %s || sqlcmd -S localhost -U %s -P %s -Q %s -h -1 -C",
				shellQuote(user), shellQuote(pass), shellQuote(restoreSQL),
				shellQuote(user), shellQuote(pass), shellQuote(restoreSQL))
		}
	}

	// Executa a restauração
	_, stderr, err := b.runSSHCommand(ctx, cmd, "")
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": fmt.Sprintf("falha na restauração do banco: %v (detalhes: %s)", err, stderr),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "success",
	})
}
