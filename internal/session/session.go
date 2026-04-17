package session

import (
	"context"
	"fmt"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"github.com/docker/docker/client"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// Credentials dados de autenticação SSH.
type Credentials struct {
	Host              string
	User              string
	Password          string
	KeyPath           string
	KeyPass           string
	KnownHostsFiles   []string // caminhos absolutos; separados na UI por |
	InsecureHostKey   bool     // se true, ignora known_hosts (inseguro)
}

// Session mantém SSH, SFTP e API Docker sobre o socket Unix remoto.
type Session struct {
	SSH    *ssh.Client
	SFTP   *sftp.Client
	Docker *client.Client

	hostAddr string
}

func (s *Session) Close() {
	if s.Docker != nil {
		_ = s.Docker.Close()
	}
	if s.SFTP != nil {
		_ = s.SFTP.Close()
	}
	if s.SSH != nil {
		_ = s.SSH.Close()
	}
}

func dialAddress(host string) string {
	h := strings.TrimSpace(host)
	if h == "" {
		return ""
	}
	if !strings.Contains(h, ":") {
		return net.JoinHostPort(h, "22")
	}
	return h
}

func authMethods(c Credentials) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if strings.TrimSpace(c.Password) != "" {
		methods = append(methods, ssh.Password(c.Password))
	}
	if strings.TrimSpace(c.KeyPath) != "" {
		pemData, err := os.ReadFile(c.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("ler chave: %w", err)
		}
		signer, err := parsePrivateKey(pemData, c.KeyPass)
		if err != nil {
			return nil, err
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if len(methods) == 0 {
		return nil, fmt.Errorf("informe senha ou arquivo de chave PEM/OpenSSH")
	}
	return methods, nil
}

// Connect abre SSH, subsistema SFTP e cliente Docker via Dial unix no host remoto.
func Connect(ctx context.Context, c Credentials) (*Session, error) {
	addr := dialAddress(c.Host)
	if addr == "" || strings.TrimSpace(c.User) == "" {
		return nil, fmt.Errorf("host e usuário são obrigatórios")
	}

	methods, err := authMethods(c)
	if err != nil {
		return nil, err
	}

	hostKeyCB, err := buildHostKeyCallback(c)
	if err != nil {
		return nil, err
	}

	cfg := &ssh.ClientConfig{
		User:            c.User,
		Auth:            methods,
		HostKeyCallback: hostKeyCB,
		Timeout:         30 * time.Second,
	}

	d := net.Dialer{Timeout: 30 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("tcp: %w", err)
	}
	cc, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ssh: %w", err)
	}
	sshClient := ssh.NewClient(cc, chans, reqs)

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		_ = sshClient.Close()
		return nil, fmt.Errorf("sftp: %w", err)
	}

	sshDial := func(ctx context.Context, _, _ string) (net.Conn, error) {
		return sshClient.DialContext(ctx, "unix", "/var/run/docker.sock")
	}

	dockerClient, err := client.NewClientWithOpts(
		client.WithHost("http://"+client.DummyHost),
		client.WithDialContext(sshDial),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		_ = sftpClient.Close()
		_ = sshClient.Close()
		return nil, fmt.Errorf("docker: %w", err)
	}

	// Ping na API (negocia versão).
	pingCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if _, err := dockerClient.Ping(pingCtx); err != nil {
		_ = dockerClient.Close()
		_ = sftpClient.Close()
		_ = sshClient.Close()
		return nil, fmt.Errorf("docker ping (permissão em /var/run/docker.sock?): %w", err)
	}

	return &Session{
		SSH:      sshClient,
		SFTP:     sftpClient,
		Docker:   dockerClient,
		hostAddr: addr,
	}, nil
}

// HostAddr retorna host:port usado no dial.
func (s *Session) HostAddr() string { return s.hostAddr }

// JoinHostPath junta segmentos POSIX (host Linux via SFTP).
func JoinHostPath(elem ...string) string {
	if len(elem) == 0 {
		return "."
	}
	return path.Clean(path.Join(elem...))
}
