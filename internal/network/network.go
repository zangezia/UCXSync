package network

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

// Service manages network share mounting on Linux
type Service struct {
	nodes        []string
	shares       []string
	username     string
	password     string
	baseMountDir string

	mu      sync.Mutex
	mounted map[string]bool // track mounted shares
}

// New creates a new network service
func New(nodes, shares []string, username, password string) *Service {
	return &Service{
		nodes:        nodes,
		shares:       shares,
		username:     username,
		password:     password,
		baseMountDir: "/mnt/ucx",
		mounted:      make(map[string]bool),
	}
}

// SetBaseMountDir sets the base directory for mounts
func (s *Service) SetBaseMountDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.baseMountDir = dir
}

// MountAll mounts all network shares
func (s *Service) MountAll() error {
	log.Info().Msg("Mounting network shares...")

	// Create base mount directory
	if err := os.MkdirAll(s.baseMountDir, 0755); err != nil {
		return fmt.Errorf("failed to create mount directory: %w", err)
	}

	// Create credentials file
	credFile := "/etc/ucxsync/credentials"
	if err := s.createCredentialsFile(credFile); err != nil {
		log.Warn().Err(err).Msg("Failed to create credentials file, will use inline credentials")
		credFile = ""
	}

	var errors []string
	mounted := 0

	for _, node := range s.nodes {
		for _, share := range s.shares {
			// Share name for mount point (without $)
			shareNameClean := strings.TrimSuffix(share, "$")

			// Create mount point
			mountPoint := filepath.Join(s.baseMountDir, node, shareNameClean)
			if err := os.MkdirAll(mountPoint, 0755); err != nil {
				errors = append(errors, fmt.Sprintf("%s/%s: %v", node, share, err))
				continue
			}

			// Check if already mounted
			if s.isMounted(mountPoint) {
				log.Debug().Str("node", node).Str("share", share).Msg("Already mounted")
				s.mu.Lock()
				s.mounted[fmt.Sprintf("%s/%s", node, share)] = true
				s.mu.Unlock()
				mounted++
				continue
			}

			// Mount the share - use original share name (with $ if present)
			uncPath := fmt.Sprintf("//%s/%s", node, share)
			if err := s.mountShare(uncPath, mountPoint, credFile); err != nil {
				errors = append(errors, fmt.Sprintf("%s/%s: %v", node, share, err))
				log.Warn().
					Str("node", node).
					Str("share", share).
					Err(err).
					Msg("Failed to mount share")
			} else {
				s.mu.Lock()
				s.mounted[fmt.Sprintf("%s/%s", node, share)] = true
				s.mu.Unlock()
				mounted++
				log.Info().
					Str("node", node).
					Str("share", share).
					Str("mount_point", mountPoint).
					Msg("Share mounted successfully")
			}
		}
	}

	log.Info().
		Int("mounted", mounted).
		Int("total", len(s.nodes)*len(s.shares)).
		Msg("Network share mounting completed")

	if len(errors) > 0 {
		return fmt.Errorf("failed to mount some shares:\n%s", strings.Join(errors, "\n"))
	}

	return nil
}

// UnmountAll unmounts all network shares
func (s *Service) UnmountAll() error {
	log.Info().Msg("Unmounting network shares...")

	s.mu.Lock()
	defer s.mu.Unlock()

	var errors []string

	for key := range s.mounted {
		parts := strings.Split(key, "/")
		if len(parts) != 2 {
			continue
		}

		node, share := parts[0], parts[1]
		shareName := strings.TrimSuffix(share, "$")
		mountPoint := filepath.Join(s.baseMountDir, node, shareName)

		if err := s.unmountShare(mountPoint); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", mountPoint, err))
		} else {
			delete(s.mounted, key)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to unmount some shares:\n%s", strings.Join(errors, "\n"))
	}

	return nil
}

// GetMountPoint returns the local mount point for a node/share
func (s *Service) GetMountPoint(node, share string) string {
	shareName := strings.TrimSuffix(share, "$")
	return filepath.Join(s.baseMountDir, node, shareName)
}

func (s *Service) mountShare(uncPath, mountPoint, credFile string) error {
	args := []string{
		"-t", "cifs",
		uncPath,
		mountPoint,
		"-o",
	}

	// Build options
	opts := []string{
		"rw",
		"file_mode=0755",
		"dir_mode=0755",
	}

	if credFile != "" {
		opts = append(opts, fmt.Sprintf("credentials=%s", credFile))
	} else {
		opts = append(opts, fmt.Sprintf("username=%s", s.username))
		opts = append(opts, fmt.Sprintf("password=%s", s.password))
	}

	// Minimal SMB1 options for Windows XP
	opts = append(opts, "vers=1.0")

	args = append(args, strings.Join(opts, ","))

	cmd := exec.Command("mount", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mount failed: %w (output: %s)", err, string(output))
	}

	return nil
}

func (s *Service) unmountShare(mountPoint string) error {
	cmd := exec.Command("umount", mountPoint)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("unmount failed: %w (output: %s)", err, string(output))
	}

	log.Debug().Str("mount_point", mountPoint).Msg("Unmounted")
	return nil
}

func (s *Service) isMounted(mountPoint string) bool {
	// Read /proc/mounts to check if path is mounted
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == mountPoint {
			return true
		}
	}

	return false
}

func (s *Service) createCredentialsFile(path string) error {
	// Create directory
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// Write credentials file
	content := fmt.Sprintf("username=%s\npassword=%s\n", s.username, s.password)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return err
	}

	log.Info().Str("path", path).Msg("Credentials file created")
	return nil
}

// CheckRequirements verifies that required tools are installed
func CheckRequirements() error {
	// Check if mount.cifs is available
	if _, err := exec.LookPath("mount.cifs"); err != nil {
		return fmt.Errorf("mount.cifs not found: please install cifs-utils (sudo apt-get install cifs-utils)")
	}

	// Check if running as root or have sudo
	if os.Geteuid() != 0 {
		return fmt.Errorf("mounting requires root privileges: please run with sudo")
	}

	return nil
}
