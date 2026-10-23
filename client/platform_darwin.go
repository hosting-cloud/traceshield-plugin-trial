package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const credentialStorageName = "macOS Keychain"

func clientConfigDir() (string, error) { return os.UserConfigDir() }

func privateWrite(path string, data []byte) error {
	if s, e := os.Lstat(path); e == nil && !s.Mode().IsRegular() {
		return fail("unsafe_path", "Refusing non-regular local state file", "")
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".traceshield-write-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e != nil {
		f.Close()
		return e
	}
	if _, e = f.Write(data); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(name, path)
}
func (c *client) credential() (string, error) {
	out, e := exec.CommandContext(c.ctx, "/usr/bin/security", "find-generic-password", "-s", c.service, "-a", "connection", "-w").Output()
	if e == nil {
		return strings.TrimSpace(string(out)), nil
	}
	var exit *exec.ExitError
	if errors.As(e, &exit) && exit.ExitCode() == 44 {
		return "", nil
	}
	return "", fail("credential_storage", "macOS Keychain is locked, denied access or unavailable. No new login was started.", "Unlock or approve the native Keychain request, then retry")
}
func (c *client) save(token string) error {
	cmd := exec.CommandContext(c.ctx, "/usr/bin/security", "-i")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("add-generic-password -U -a connection -s %s -X %s\n", c.service, hex.EncodeToString([]byte(token))))
	if e := cmd.Run(); e != nil {
		return fail("credential_storage", "Browser approval succeeded, but the connection could not be saved. Setup is incomplete.", "Resolve the native Keychain request, then run connect")
	}
	got, e := c.credential()
	if e != nil {
		return e
	}
	if got != token {
		return fail("credential_storage", "Saved connection could not be verified", "connect")
	}
	c.token = token
	return nil
}
func (c *client) deleteCredential() error {
	cmd := exec.CommandContext(c.ctx, "/usr/bin/security", "delete-generic-password", "-s", c.service, "-a", "connection")
	e := cmd.Run()
	if e != nil {
		var x *exec.ExitError
		if !errors.As(e, &x) || x.ExitCode() != 44 {
			return fail("credential_storage", "Could not remove local credential", "")
		}
	}
	return nil
}

func (c *client) openBrowser(link string) error {
	if e := exec.CommandContext(c.ctx, "/usr/bin/open", link).Run(); e != nil {
		return fail("browser", "Could not open browser approval", "connect")
	}
	return nil
}

func sameGitRoot(root, dir string) bool { return root == dir }
func gitHelperPath(path string) string  { return path }
