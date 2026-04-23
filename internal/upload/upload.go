package upload

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/redhat-consulting-services/ocp-support-web/internal/k8s"
)

const (
	defaultSFTPHost    = "sftp.access.redhat.com"
	defaultSFTPPort    = "22"
	secretName         = "ocp-support-upload-creds"
	hostKeySecretName  = "ocp-support-web-sftp-known-hosts"
)

type Manager struct {
	k8s       *k8s.Client
	namespace string

	mu          sync.RWMutex
	configured  bool
	username    string
	password    string
	hostKey     ssh.PublicKey
}

func NewManager(k8sClient *k8s.Client, namespace string) *Manager {
	m := &Manager{
		k8s:       k8sClient,
		namespace: namespace,
	}
	m.refreshCredentials()
	return m
}

func (m *Manager) refreshCredentials() {
	path := "/api/v1/namespaces/" + m.namespace + "/secrets/" + secretName
	data, err := m.k8s.Get(path)
	if err != nil {
		m.mu.Lock()
		m.configured = false
		m.username = ""
		m.password = ""
		m.mu.Unlock()
		return
	}

	secretData, ok := data["data"].(map[string]interface{})
	if !ok {
		return
	}

	m.mu.Lock()
	m.username = k8s.DecodeBase64(secretData, "username")
	m.password = k8s.DecodeBase64(secretData, "password")
	m.configured = m.username != "" && m.password != ""
	m.mu.Unlock()

	m.refreshHostKey()
}

func (m *Manager) refreshHostKey() {
	path := "/api/v1/namespaces/" + m.namespace + "/secrets/" + hostKeySecretName
	data, err := m.k8s.Get(path)
	if err != nil {
		return
	}
	secretData, ok := data["data"].(map[string]interface{})
	if !ok {
		return
	}
	raw := k8s.DecodeBase64(secretData, "known-hosts")
	if raw == "" {
		return
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// known_hosts format: "hostname key-type base64-key"
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(parts[1]))
		if err != nil {
			log.Printf("upload: failed to parse SFTP host key: %v", err)
			continue
		}
		m.mu.Lock()
		m.hostKey = key
		m.mu.Unlock()
		return
	}
}

func (m *Manager) IsConfigured() bool {
	m.refreshCredentials()
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.configured
}

func (m *Manager) Upload(caseID, filePath string, internalUser bool) error {
	m.mu.RLock()
	username := m.username
	password := m.password
	configured := m.configured
	hk := m.hostKey
	m.mu.RUnlock()

	if !configured {
		return fmt.Errorf("upload credentials not configured — create secret %s with username and password", secretName)
	}
	if hk == nil {
		return fmt.Errorf("SFTP host key not configured — create secret %s with known-hosts entry", hostKeySecretName)
	}
	if caseID == "" {
		return fmt.Errorf("case ID is required")
	}
	if _, err := os.Stat(filePath); err != nil {
		return fmt.Errorf("file not found: %s", filePath)
	}

	sshConfig := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: ssh.FixedHostKey(hk),
	}

	addr := net.JoinHostPort(defaultSFTPHost, defaultSFTPPort)
	log.Printf("Connecting to %s for case %s upload", addr, caseID)

	conn, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return fmt.Errorf("sftp connection failed: %w", err)
	}
	defer conn.Close()

	client, err := sftp.NewClient(conn)
	if err != nil {
		return fmt.Errorf("sftp client: %w", err)
	}
	defer client.Close()

	localFile, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open local file: %w", err)
	}
	defer localFile.Close()

	baseName := filepath.Base(filePath)
	remoteName := caseID + "_" + baseName
	if strings.HasPrefix(baseName, caseID+"_") {
		remoteName = baseName
	}

	remoteDir := "/incoming/" + caseID
	if internalUser {
		remoteDir = "/case-mgmt/" + caseID
	}
	remotePath := remoteDir + "/" + remoteName

	if err := client.MkdirAll(remoteDir); err != nil {
		log.Printf("mkdir %s (may already exist): %v", remoteDir, err)
	}

	remoteFile, err := client.Create(remotePath)
	if err != nil {
		return fmt.Errorf("create remote file: %w", err)
	}
	defer remoteFile.Close()

	written, err := io.Copy(remoteFile, localFile)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	log.Printf("Uploaded %s (%d bytes) to %s:%s", baseName, written, defaultSFTPHost, remotePath)
	return nil
}
