package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/danieljoos/wincred"
	"golang.org/x/sys/windows"
)

const credentialStorageName = "Windows Credential Manager"

func clientConfigDir() (string, error) {
	return windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
}

func (c *client) credential() (string, error) {
	credential, err := wincred.GetGenericCredential(c.service)
	if errors.Is(err, wincred.ErrElementNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fail("credential_storage", "Windows Credential Manager is denied or unavailable. No new login was started.", "Resolve the native credential storage restriction, then retry")
	}
	return string(credential.CredentialBlob), nil
}

func (c *client) save(token string) error {
	credential := wincred.NewGenericCredential(c.service)
	credential.UserName = "connection"
	credential.CredentialBlob = []byte(token)
	credential.Persist = wincred.PersistLocalMachine
	if err := credential.Write(); err != nil {
		return fail("credential_storage", "Browser approval succeeded, but the connection could not be saved. Setup is incomplete.", "Resolve the native credential storage restriction, then run connect")
	}
	got, err := c.credential()
	if err != nil {
		return err
	}
	if got != token {
		return fail("credential_storage", "Saved connection could not be verified", "connect")
	}
	c.token = token
	return nil
}

func (c *client) deleteCredential() error {
	err := wincred.NewGenericCredential(c.service).Delete()
	if err != nil && !errors.Is(err, wincred.ErrElementNotFound) {
		return fail("credential_storage", "Could not remove local credential", "")
	}
	return nil
}

func (c *client) openBrowser(link string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE)
	if err != nil && !errors.Is(err, syscall.Errno(windows.S_FALSE)) {
		return fail("browser", "Could not initialize the Windows browser environment", "Check the native browser configuration, then run connect")
	}
	defer windows.CoUninitialize()
	url, err := windows.UTF16PtrFromString(link)
	if err != nil {
		return fail("browser", "Invalid browser approval URL", "connect")
	}
	if err = windows.ShellExecute(0, nil, url, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		return fail("browser", "Could not open browser approval", "connect")
	}
	return nil
}

func privateDescriptor() (*windows.SECURITY_DESCRIPTOR, error) {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return nil, err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return nil, err
	}
	return windows.SecurityDescriptorFromString("D:P(A;;FA;;;SY)(A;;FA;;;" + user.User.Sid.String() + ")")
}

func privateWrite(path string, data []byte) error {
	if s, err := os.Lstat(path); err == nil && !s.Mode().IsRegular() {
		return fail("unsafe_path", "Refusing non-regular local state file", "")
	}
	target, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attributes, err := windows.GetFileAttributes(target)
	if err == nil && attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fail("unsafe_path", "Refusing a reparse-point local state file", "")
	}
	sd, err := privateDescriptor()
	if err != nil {
		return fail("private_file", "Could not establish private file permissions", "Use a local app folder with Windows access-control support")
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return err
	}
	name := filepath.Join(filepath.Dir(path), ".traceshield-write-"+hex.EncodeToString(nonce[:]))
	wideName, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	security := windows.SecurityAttributes{SecurityDescriptor: sd}
	security.Length = uint32(unsafe.Sizeof(security))
	handle, err := windows.CreateFile(wideName, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, &security, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return fail("private_file", "Could not create a private local file", "Use a writable local app folder with Windows access-control support")
	}
	f := os.NewFile(uintptr(handle), name)
	defer func() {
		f.Close()
		os.Remove(name)
	}()
	applied, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil || applied.String() != sd.String() {
		return fail("private_file", "The filesystem did not confirm private file permissions; no secret data was written", "Use a local app folder with Windows access-control support")
	}
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func sameGitRoot(root, dir string) bool {
	a, err := os.Stat(root)
	if err != nil {
		return false
	}
	b, err := os.Stat(dir)
	return err == nil && os.SameFile(a, b)
}

func gitHelperPath(path string) string { return filepath.ToSlash(path) }
