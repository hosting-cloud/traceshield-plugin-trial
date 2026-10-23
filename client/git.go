package main

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type gitCredentialResult struct{ password string }

func (c *client) gitRepository() (*binding, string, error) {
	b, err := c.verifiedBinding()
	if err != nil {
		return nil, "", err
	}
	r, err := c.call("project", map[string]any{"project": b.Slug})
	if err != nil {
		return nil, "", err
	}
	p, ok := r["result"].(map[string]any)
	if !ok {
		return nil, "", fail("protocol", "Invalid project response", "")
	}
	repo, _ := p["repo_full_name"].(string)
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}$`).MatchString(parts[0]) || parts[0] == "local" || parts[1] != b.Slug {
		return nil, "", fail("git_repository", "A managed GitHub repository is required for Git publishing", "")
	}
	return b, "https://github.com/" + repo + ".git", nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func (c *client) setupGit() (any, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fail("git_required", "Git is not available on this session's PATH", "Install standard Git for your operating system and make git available to this coding session, then retry; no separate Git Bash session is required")
	}
	top, err := c.git("rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fail("git_repository", "Prepare a Git repository in the selected app folder first", "Preserve existing Git; initialize only a non-Git app when publishing is requested")
	}
	root, err := filepath.EvalSymlinks(strings.TrimSpace(string(top)))
	if err != nil || !sameGitRoot(root, c.dir) {
		return nil, fail("git_repository", "Select the Git repository root explicitly; no configuration changed", "Use --dir APP_ROOT")
	}
	b, remote, err := c.gitRepository()
	if err != nil {
		return nil, err
	}
	remotes, err := c.git("remote")
	if err != nil {
		return nil, err
	}
	exists := contains(strings.Fields(string(remotes)), "traceshield")
	if exists {
		for _, args := range [][]string{{"remote", "get-url", "--all", "traceshield"}, {"remote", "get-url", "--push", "--all", "traceshield"}} {
			out, e := c.git(args...)
			if e != nil || strings.TrimSpace(string(out)) != remote {
				return nil, fail("git_remote_conflict", "Existing traceshield remote points elsewhere or has multiple destinations; it was not changed", "Review the existing remote before connecting this project")
			}
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return nil, err
	}
	helper := "!TRACESHIELD_ORIGIN=" + shellQuote(c.origin) + " " + shellQuote(gitHelperPath(exe)) + " --dir " + shellQuote(gitHelperPath(c.dir)) + " git credential"
	if !exists {
		if _, err = c.git("remote", "add", "traceshield", remote); err != nil {
			return nil, fail("git_configuration", "Could not add the traceshield remote", "Check repository configuration permissions")
		}
	}
	key := "credential." + remote
	for _, args := range [][]string{{"config", "--local", "--replace-all", key + ".useHttpPath", "true"}, {"config", "--local", "--replace-all", key + ".helper", ""}, {"config", "--local", "--add", key + ".helper", helper}} {
		if _, err = c.git(args...); err != nil {
			return nil, fail("git_configuration", "Git setup is incomplete; no push or deployment was started", "Resolve repository config permissions, then rerun git setup")
		}
	}
	return map[string]any{"project": b.Slug, "remote": "traceshield", "url": remote, "configured": true, "pushed": false, "deployed": false, "source_scope": "A normal Git push transfers the committed files and reachable Git history, not untracked files", "next_action": "Review intended source and history, commit intended changes, then git push traceshield HEAD:refs/heads/main. Verify the full HEAD SHA against git ls-remote traceshield refs/heads/main (or the alternate ref), then deploy that exact SHA. For non-fast-forward rejection only, push HEAD:refs/heads/traceshield/FULL_SHA to preserve both histories; never force-push."}, nil
}

func (c *client) gitCredential() (any, error) {
	input, err := io.ReadAll(io.LimitReader(os.Stdin, (1<<20)+1))
	if err != nil || len(input) > 1<<20 {
		return nil, fail("git_protocol", "Git credential request could not be read or exceeds1MiB", "Update the plugin if this persists; reconnecting will not repair a protocol error")
	}
	fields := map[string]string{}
	for _, line := range strings.Split(string(input), "\n") {
		if line == "" {
			break
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" || strings.ContainsRune(line, '\x00') || len(line)+1 > 65535 {
			return nil, fail("git_protocol", "Malformed Git credential request", "Update the plugin if this persists; no login was started")
		}
		if !contains([]string{"protocol", "host", "path"}, key) {
			continue
		}
		if _, duplicate := fields[key]; duplicate {
			return nil, fail("git_protocol", "Git supplied an ambiguous credential destination", "Check the repository credential configuration; no login was started")
		}
		fields[key] = value
	}
	if fields["protocol"] != "https" || fields["host"] != "github.com" {
		return nil, fail("git_destination", "Git credential destination is not supported", "Run git setup for the linked app; credentials are restricted to its managed GitHub repository")
	}
	token, err := c.credential()
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, fail("authentication_required", "Connect Traceshield before pushing", "connect")
	}
	c.token = token
	me, err := c.request("GET", "/api/me", nil, true)
	if err != nil {
		return nil, err
	}
	b, err := c.readBinding()
	if err != nil {
		return nil, fail("project_binding", "This folder has no valid Traceshield project mapping", "Select the app root and link its project before git setup")
	}
	if b.Account != me["email"] {
		return nil, fail("account_mismatch", "Git credential account does not match this folder", "Connect the original account; the folder mapping was not changed")
	}
	r, err := c.request("POST", "/api/agent-tools/project", bytes.NewBufferString(fmt.Sprintf(`{"project":%q}`, b.Slug)), true)
	if err != nil {
		return nil, err
	}
	p, ok := r["result"].(map[string]any)
	if !ok || p["id"] != b.ID {
		return nil, fail("project_binding", "Git project identity changed", "")
	}
	repo, _ := p["repo_full_name"].(string)
	if repo == "" || fields["path"] != repo+".git" && fields["path"] != repo {
		return nil, fail("git_destination", "Git credential is restricted to the linked project repository", "Run git setup for the linked app; do not replace a different remote")
	}
	r, err = c.request("GET", "/api/projects/"+b.Slug+"/push", nil, true)
	if err != nil {
		return nil, err
	}
	command, _ := r["command"].(string)
	args := strings.Fields(command)
	if len(args) != 4 || args[0] != "git" || args[1] != "push" || args[3] != "HEAD:main" {
		return nil, fail("protocol", "Unsupported Git credential response", "")
	}
	u, err := url.Parse(args[2])
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.Path != "/"+repo+".git" || u.RawQuery != "" || u.Fragment != "" || u.User == nil || u.User.Username() != "x-access-token" {
		return nil, fail("protocol", "Unexpected Git credential destination", "")
	}
	password, ok := u.User.Password()
	if !ok || password == "" || strings.ContainsAny(password, "\r\n\x00") {
		return nil, fail("protocol", "Invalid Git credential response", "")
	}
	return gitCredentialResult{password: password}, nil
}
