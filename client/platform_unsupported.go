//go:build !darwin && !windows

package main

const credentialStorageName = "unsupported"

func unsupportedPlatform() error {
	return fail("unsupported_platform", "This client supports native macOS and Windows only; WSL/Linux are not supported yet", "")
}

func clientConfigDir() (string, error) { return "", unsupportedPlatform() }

func (c *client) credential() (string, error) { return "", unsupportedPlatform() }
func (c *client) save(string) error           { return unsupportedPlatform() }
func (c *client) deleteCredential() error     { return unsupportedPlatform() }
func (c *client) openBrowser(string) error    { return unsupportedPlatform() }
func privateWrite(string, []byte) error       { return unsupportedPlatform() }
func sameGitRoot(root, dir string) bool       { return root == dir }
func gitHelperPath(path string) string        { return path }
