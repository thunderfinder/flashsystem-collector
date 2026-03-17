package ssh

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Client representa una conexión SSH persistente y thread-safe.
// Una sola instancia puede ejecutar múltiples comandos en paralelo
// mediante sesiones independientes sobre la misma conexión TCP.
type Client struct {
	conn           *ssh.Client
	commandTimeout time.Duration
}

// ClientConfig agrupa los parámetros necesarios para crear un Client.
type ClientConfig struct {
	Host            string
	Port            int
	User            string
	Password        string // Vacío si se usa KeyFile
	KeyFile         string // Vacío si se usa Password
	DialTimeout     time.Duration
	CommandTimeout  time.Duration
	InsecureHostKey bool
	KnownHostsFile  string // Usado solo si InsecureHostKey == false
}

// New establece una conexión SSH al host configurado.
// Retorna error si la conexión o autenticación falla.
// El caller es responsable de llamar Close() cuando termine.
func New(cfg ClientConfig) (*Client, error) {

	// --- Construir método de autenticación ---
	authMethod, err := buildAuthMethod(cfg)
	if err != nil {
		return nil, fmt.Errorf("ssh auth setup failed: %w", err)
	}

	// --- Construir callback de host key ---
	hostKeyCallback, err := buildHostKeyCallback(cfg)
	if err != nil {
		return nil, fmt.Errorf("ssh host key setup failed: %w", err)
	}

	sshCfg := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            []ssh.AuthMethod{authMethod},
		HostKeyCallback: hostKeyCallback,
		Timeout:         cfg.DialTimeout,
	}

	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))

	// Usar net.DialTimeout para controlar el timeout de red de forma explícita,
	// luego pasar el conn al cliente SSH. Esto evita que ssh.Dial ignore
	// el timeout configurado en algunos sistemas.
	netConn, err := net.DialTimeout("tcp", addr, cfg.DialTimeout)
	if err != nil {
		return nil, fmt.Errorf("tcp dial to %s failed: %w", addr, err)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(netConn, addr, sshCfg)
	if err != nil {
		netConn.Close()
		return nil, fmt.Errorf("ssh handshake to %s failed: %w", addr, err)
	}

	conn := ssh.NewClient(sshConn, chans, reqs)

	return &Client{
		conn:           conn,
		commandTimeout: cfg.CommandTimeout,
	}, nil
}

// Run ejecuta un comando en el FlashSystem y retorna su stdout.
// Cada llamada abre una sesión SSH nueva sobre la conexión existente.
// El comando se cancela automáticamente después de CommandTimeout.
// Es seguro llamar Run concurrentemente desde múltiples goroutines.
func (c *Client) Run(cmd string) (string, error) {

	session, err := c.conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	// Canal para recibir el resultado del comando.
	type result struct {
		output string
		err    error
	}
	done := make(chan result, 1)

	// Ejecutar el comando en una goroutine para poder aplicar timeout.
	go func() {
		err := session.Run(cmd)
		if err != nil {
			// Incluir stderr en el error para facilitar troubleshooting.
			stderrStr := stderr.String()
			if stderrStr != "" {
				done <- result{"", fmt.Errorf("command %q failed: %w (stderr: %s)", cmd, err, stderrStr)}
			} else {
				done <- result{"", fmt.Errorf("command %q failed: %w", cmd, err)}
			}
			return
		}
		done <- result{stdout.String(), nil}
	}()

	// Aplicar timeout usando un context con deadline.
	ctx, cancel := context.WithTimeout(context.Background(), c.commandTimeout)
	defer cancel()

	select {
	case res := <-done:
		return res.output, res.err
	case <-ctx.Done():
		// Signal al servidor SSH que la sesión debe cerrarse.
		// session.Signal puede fallar en algunas implementaciones; lo ignoramos
		// porque la sesión se cerrará con defer session.Close() de todas formas.
		_ = session.Signal(ssh.SIGTERM)
		return "", fmt.Errorf("command %q timed out after %s", cmd, c.commandTimeout)
	}
}

// Close cierra la conexión SSH y libera todos los recursos asociados.
// Debe llamarse con defer inmediatamente después de New().
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Ping verifica que la conexión SSH sigue activa ejecutando un comando trivial.
// Útil para validar la conexión antes de comenzar la recolección.
func (c *Client) Ping() error {
	_, err := c.Run("svcinfo lssystem -nohdr 2>/dev/null | head -1")
	if err != nil {
		return fmt.Errorf("SSH ping failed: %w", err)
	}
	return nil
}

// buildAuthMethod construye el método de autenticación SSH según la configuración.
func buildAuthMethod(cfg ClientConfig) (ssh.AuthMethod, error) {

	if cfg.KeyFile != "" {
		// Autenticación por clave privada (recomendado para producción).
		keyBytes, err := os.ReadFile(cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("cannot read SSH key file %q: %w", cfg.KeyFile, err)
		}

		signer, err := ssh.ParsePrivateKey(keyBytes)
		if err != nil {
			// Puede ser una clave con passphrase; en ese caso el error es claro.
			return nil, fmt.Errorf("cannot parse SSH private key %q: %w", cfg.KeyFile, err)
		}

		return ssh.PublicKeys(signer), nil
	}

	if cfg.Password != "" {
		// Autenticación por password (aceptable en redes segmentadas).
		return ssh.Password(cfg.Password), nil
	}

	return nil, fmt.Errorf("no authentication method configured: provide KeyFile or Password")
}

// buildHostKeyCallback construye el callback de verificación de host key.
// Si InsecureHostKey es true, deshabilita la verificación (solo para testing/redes aisladas).
// Si KnownHostsFile está configurado, usa ese archivo.
// Si ninguno está configurado y InsecureHostKey es false, retorna error.

func buildHostKeyCallback(cfg ClientConfig) (ssh.HostKeyCallback, error) {
	if cfg.InsecureHostKey {
		return ssh.InsecureIgnoreHostKey(), nil //nolint:gosec
	}

	if cfg.KnownHostsFile != "" {
		cb, err := knownhosts.New(cfg.KnownHostsFile)
		if err != nil {
			return nil, fmt.Errorf("cannot load known_hosts from %q: %w",
				cfg.KnownHostsFile, err)
		}
		return cb, nil
	}

	// NUEVO: intentar fallback a ~/.ssh/known_hosts del usuario que ejecuta el proceso
	homeDir, err := os.UserHomeDir()
	if err == nil {
		defaultKnownHosts := filepath.Join(homeDir, ".ssh", "known_hosts")
		if _, err := os.Stat(defaultKnownHosts); err == nil {
			cb, err := knownhosts.New(defaultKnownHosts)
			if err == nil {
				return cb, nil
			}
		}
	}

	return nil, fmt.Errorf(
		"SSH host key verification not configured: " +
			"use -insecure (isolated networks) or " +
			"-known-hosts /path/to/known_hosts (production)",
	)
}
