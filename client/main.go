package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const version = "0.3.2"
const maxResponse = 4 << 20

var validSlug = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var validDeployment = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var validTool = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

type failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Next    string `json:"next_action,omitempty"`
	Status  int    `json:"-"`
}

func (e *failure) Error() string { return e.Message }
func fail(code, message, next string) error {
	return &failure{Code: code, Message: message, Next: next}
}

type state struct {
	Installation string `json:"installation"`
}
type binding struct {
	Version    int               `json:"version"`
	Origin     string            `json:"origin"`
	Account    string            `json:"account"`
	ID         string            `json:"project_id"`
	Slug       string            `json:"slug"`
	URL        string            `json:"live_url"`
	LastUpload map[string]string `json:"last_upload,omitempty"`
}
type client struct {
	ctx                         context.Context
	origin, dir, service, label string
	http                        *http.Client
	token                       string
}

func emit(v any) { _ = json.NewEncoder(os.Stdout).Encode(v) }
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	result, err := run(ctx, os.Args[1:])
	if err == nil {
		if credential, ok := result.(gitCredentialResult); ok {
			if credential.password != "" {
				fmt.Fprintf(os.Stdout, "username=x-access-token\npassword=%s\n\n", credential.password)
			}
			return
		}
		emit(map[string]any{"ok": true, "data": result})
		return
	}
	var f *failure
	if !errors.As(err, &f) {
		f = &failure{Code: "local_error", Message: err.Error()}
	}
	if len(os.Args) >= 4 && os.Args[len(os.Args)-3] == "git" && os.Args[len(os.Args)-2] == "credential" {
		fmt.Fprintf(os.Stderr, "Traceshield Git [%s]: %s\n", f.Code, f.Message)
		if f.Next != "" {
			fmt.Fprintln(os.Stderr, "Next:", f.Next)
		}
	} else {
		emit(map[string]any{"ok": false, "error": f})
	}
	os.Exit(1)
}
func run(ctx context.Context, args []string) (any, error) {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || contains(args, "--help") {
		return help(), nil
	}
	if len(args) == 1 && args[0] == "--version" {
		return map[string]string{"version": version}, nil
	}
	fs := flag.NewFlagSet("traceshield", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dir := fs.String("dir", ".", "app folder")
	if err := fs.Parse(args); err != nil {
		return nil, fail("usage", "Invalid option; use --help", "")
	}
	args = fs.Args()

	valid := len(args) == 1 && contains([]string{"connect", "status", "logout", "publish"}, args[0]) || len(args) == 2 && (args[0] == "tools" && args[1] == "list" || args[0] == "source" && args[1] == "upload" || args[0] == "env" && args[1] == "pull" || args[0] == "git" && args[1] == "setup") || len(args) == 3 && (args[0] == "tools" && args[1] == "call" && validTool.MatchString(args[2]) || args[0] == "project" && args[1] == "link" && validSlug.MatchString(args[2]) || args[0] == "deployment" && args[1] == "wait" && validDeployment.MatchString(args[2]) || args[0] == "git" && args[1] == "credential" && contains([]string{"get", "store", "erase", "capability"}, args[2]))
	if !valid {
		return nil, fail("usage", "Unknown command or invalid arguments; use --help", "")
	}
	var input map[string]any
	if args[0] == "tools" && args[1] == "call" {
		b, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20+1))
		if err != nil || len(b) > 1<<20 {
			return nil, fail("usage", "JSON input exceeds1MiB or could not be read", "")
		}
		// Windows PowerShell 5.1 under chcp 65001 prefixes piped stdin with a UTF-8 BOM.
		b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
		if len(bytes.TrimSpace(b)) == 0 {
			b = []byte("{}")
		}
		if json.Unmarshal(b, &input) != nil || input == nil {
			return nil, fail("usage", "tools call expects one JSON object on stdin", "")
		}
	}
	if args[0] == "git" && args[1] == "credential" && args[2] != "get" {
		return gitCredentialResult{}, nil
	}
	c, err := newClient(ctx, *dir)
	if err != nil {
		return nil, err
	}
	if args[0] == "git" && args[1] == "credential" {
		return c.gitCredential()
	}
	switch args[0] {
	case "connect":
		return c.connect()
	case "logout":
		return c.logout()
	case "status":
		token, err := c.credential()
		if err != nil {
			return nil, err
		}
		if token == "" {
			return map[string]any{"connected": false, "reason": "not_connected", "next_action": "connect"}, nil
		}
		c.token = token
		me, err := c.request("GET", "/api/me", nil, true)
		var f *failure
		if errors.As(err, &f) && f.Status == 401 {
			return map[string]any{"connected": false, "reason": "expired_or_revoked", "next_action": "connect"}, nil
		}
		if err != nil {
			return nil, err
		}
		result := map[string]any{"connected": true, "account": me["email"], "origin": c.origin, "agent_name": "Claude Code"}
		if b, e := c.readBinding(); e == nil {
			if b.Account != me["email"] {
				result["project_binding"] = "account_mismatch"
			} else {
				current, err := c.call("project", map[string]any{"project": b.Slug})
				if err != nil {
					var missing *failure
					if errors.As(err, &missing) && missing.Status == 404 {
						result["project_binding"] = "project_unavailable"
						return result, nil
					}
					return nil, err
				}
				p, ok := current["result"].(map[string]any)
				if !ok || p["id"] != b.ID {
					return nil, fail("project_binding", "Project identity changed; folder was not relinked", "")
				}
				b.URL, _ = p["live_url"].(string)
				result["project"] = b
				result["project_status"] = p["status"]
			}
		} else if !os.IsNotExist(e) {
			return nil, e
		}
		return result, nil
	}
	if err := c.ensureAuth(); err != nil {
		return nil, err
	}
	switch args[0] {
	case "deployment":
		return c.waitDeployment(args[2])
	case "publish":
		return c.publish()
	case "tools":
		if args[1] == "list" {
			return c.authed("GET", "/api/agent-tools", nil)
		}

		return c.call(args[2], input)
	case "project":
		return c.link(args[2])
	case "git":
		return c.setupGit()
	case "source":
		return c.upload()
	case "env":
		return c.pullEnv()
	}
	return nil, fail("usage", "Unknown command", "")
}
func (c *client) waitDeployment(id string) (map[string]any, error) {
	waitCtx, cancel := context.WithTimeout(c.ctx, 5*time.Minute)
	defer cancel()
	defer func(ctx context.Context) { c.ctx = ctx }(c.ctx)
	c.ctx = waitCtx
	for {
		result, err := c.call("deployment_status", map[string]any{"deployment_id": id})
		if err != nil {
			return nil, err
		}
		deployment, ok := result["result"].(map[string]any)
		if !ok {
			return nil, fail("protocol", "Invalid deployment status response", "")
		}
		switch deployment["status"] {
		case "live", "failed":
			return result, nil
		case "queued", "building", "starting":
		default:
			return nil, fail("protocol", "Unknown deployment status", "")
		}
		select {
		case <-waitCtx.Done():
			return nil, fail("deployment_pending", "Deployment is still pending or waiting was cancelled; no new deployment was started", "Check deployment_status for the same deployment ID")
		case <-time.After(3 * time.Second):
		}
	}
}
func help() any {
	return map[string]any{"version": version, "usage": "traceshield [--dir APP_FOLDER] COMMAND", "commands": []string{"connect", "status", "tools list", "tools call NAME < input.json", "deployment wait ID", "project link SLUG", "publish", "git setup", "source upload", "env pull", "logout", "help", "--version"}, "discovery": "tools list supplies current action descriptions and JSON schemas. Call integration_guide for database/auth/deployment/debugging guidance. No need to inspect the binary.", "local_helpers": "publish is the normal way to put committed work online: it pushes committed HEAD directly to the linked app's private Traceshield repository (no remote added, origin and Git config untouched, never forced; a diverged history goes to refs/heads/traceshield/FULL_SHA), verifies the remote SHA, deploys that exact SHA and waits for live or failed. Uncommitted files are not sent. git setup is a manual fallback: it adds the traceshield remote and repository-URL-scoped credential helper without changing origin/global configuration. It does not initialize, stage, commit, push or deploy. Review intended files and reachable history, git push traceshield HEAD:refs/heads/main, verify the full SHA against the intended remote ref with git ls-remote, then deploy that exact SHA. For a non-fast-forward rejection preserve both histories by pushing HEAD:refs/heads/traceshield/FULL_SHA; never force. source upload remains a legacy snapshot-only alternative; do not mix snapshot and Git SHA receipts. env pull writes dev credentials privately to ignored .env.local. deployment wait polls one deployment for up to five minutes.", "help_is_read_only": true}
}
func contains(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
func newClient(ctx context.Context, dir string) (*client, error) {
	origin := strings.TrimRight(os.Getenv("TRACESHIELD_ORIGIN"), "/")
	if origin == "" {
		origin = "https://portal.traceshield.app"
	}
	u, e := url.Parse(origin)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1")) {
		return nil, fail("configuration", "Service must be an HTTPS origin (HTTP allowed only on localhost)", "")
	}
	dir, e = filepath.Abs(dir)
	if e != nil {
		return nil, e
	}
	dir, e = filepath.EvalSymlinks(dir)
	if e != nil {
		return nil, e
	}
	cfg, e := clientConfigDir()
	if e != nil {
		return nil, e
	}
	sum := sha256.Sum256([]byte(origin))
	cfg = filepath.Join(cfg, "Traceshield", hex.EncodeToString(sum[:8]))
	if e = os.MkdirAll(cfg, 0700); e != nil {
		return nil, e
	}
	if e = os.Chmod(cfg, 0700); e != nil {
		return nil, e
	}
	p := filepath.Join(cfg, "installation.json")
	var s state
	b, e := os.ReadFile(p)
	if os.IsNotExist(e) {
		id := make([]byte, 12)
		if _, e = rand.Read(id); e != nil {
			return nil, e
		}
		s.Installation = hex.EncodeToString(id)
		b, _ = json.Marshal(s)
		e = privateWrite(p, b)
	} else if e == nil {
		e = json.Unmarshal(b, &s)
	}
	if e != nil || !regexp.MustCompile(`^[a-f0-9]{24}$`).MatchString(s.Installation) {
		return nil, fail("configuration", "Invalid installation state; do not overwrite it automatically", "")
	}
	return &client{ctx: ctx, origin: origin, dir: dir, service: "app.traceshield.client." + hex.EncodeToString(sum[:8]) + "." + s.Installation, label: "Traceshield " + s.Installation, http: &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *client) request(method, path string, body io.Reader, auth bool) (map[string]any, error) {
	if !strings.HasPrefix(path, "/api/") || strings.Contains(path, "?") && strings.Contains(path, "//") {
		return nil, fail("protocol", "Invalid API path", "")
	}
	req, e := http.NewRequestWithContext(c.ctx, method, c.origin+path, body)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "traceshield/"+version)
	if strings.HasSuffix(path, "/source") {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	if auth {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, e := c.http.Do(req)
	if e != nil {
		return nil, fail("network", "Could not reach Traceshield. No automatic replay was attempted; a submitted operation may still have completed.", "Check connection/status before retrying a mutation")
	}
	defer res.Body.Close()
	b, e := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
	if e != nil || len(b) > maxResponse {
		return nil, fail("protocol", "Invalid or oversized API response", "")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		code := "service_error"
		next := "Read status/logs before retrying a mutation"
		if res.StatusCode == 401 {
			code = "authentication_required"
			next = "connect"
		}
		if res.StatusCode == 403 {
			code = "not_allowed"
			next = "Check account access"
		}
		message := fmt.Sprintf("Traceshield returned HTTP%d", res.StatusCode)
		var detail struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(b, &detail) == nil && detail.Error != "" && len(detail.Error) <= 1200 {
			safe := detail.Error
			if c.token != "" {
				safe = strings.ReplaceAll(safe, c.token, "<redacted>")
			}
			safe = regexp.MustCompile(`(?:gw_[A-Za-z0-9_-]{20,}|gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})`).ReplaceAllString(safe, "<redacted>")
			message += ": " + safe
		}
		return nil, &failure{Code: code, Message: message, Next: next, Status: res.StatusCode}
	}
	var result map[string]any
	if json.Unmarshal(b, &result) != nil || result == nil {
		return nil, fail("protocol", "Expected JSON object response", "")
	}
	return result, nil
}
func (c *client) connect() (any, error) {
	token, e := c.credential()
	if e != nil {
		return nil, e
	}
	if token != "" {
		c.token = token
		me, e := c.request("GET", "/api/me", nil, true)
		if e == nil {
			return map[string]any{"connected": true, "reused": true, "account": me["email"], "agent_name": "Claude Code"}, nil
		}
		var f *failure
		if !errors.As(e, &f) || f.Status != 401 {
			return nil, e
		}
	}
	b, _ := json.Marshal(map[string]string{"agent": c.label})
	login, e := c.request("POST", "/api/agent-login", bytes.NewReader(b), false)
	if e != nil {
		return nil, e
	}
	id, _ := login["login_id"].(string)
	link, _ := login["url"].(string)
	u, e := url.Parse(link)
	if e != nil || !strings.HasPrefix(link, c.origin+"/agent-login/") || u.RawQuery != "" || !regexp.MustCompile(`^[a-f0-9-]{36}$`).MatchString(id) {
		return nil, fail("protocol", "Invalid browser authorization response", "")
	}
	if e = c.openBrowser(link); e != nil {
		return nil, e
	}
	fmt.Fprintln(os.Stderr, "Approve Traceshield in the browser; I'll continue automatically.")
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-c.ctx.Done():
			return nil, fail("cancelled", "Connection cancelled", "connect")
		case <-time.After(2 * time.Second):
		}
		r, e := c.request("GET", "/api/agent-login/"+id, nil, false)
		if e != nil {
			return nil, e
		}
		if r["status"] == "denied" || r["status"] == "expired" {
			return nil, fail("authorization_incomplete", "Browser authorization was denied or expired", "connect")
		}
		if r["status"] == "approved" && r["token"] == nil {
			return nil, fail("authorization_pickup_expired", "Browser approval expired before the connection could be collected; no credential was saved", "Run connect to open a new approval")
		}
		if token, ok := r["token"].(string); ok && strings.HasPrefix(token, "gw_") {
			c.token = token
			me, e := c.request("GET", "/api/me", nil, true)
			if e != nil {
				return nil, e
			}
			if e = c.save(token); e != nil {
				return nil, e
			}
			return map[string]any{"connected": true, "account": me["email"], "credential_storage": credentialStorageName, "agent_label": c.label, "agent_name": "Claude Code"}, nil
		}
	}
	return nil, fail("authorization_incomplete", "Browser authorization was not completed within10minutes", "connect")
}
func (c *client) ensureAuth() error {
	t, e := c.credential()
	if e != nil {
		return e
	}
	if t == "" {
		_, e = c.connect()
		return e
	}
	c.token = t
	return nil
}
func (c *client) authed(method, path string, b []byte) (map[string]any, error) {
	r, e := c.request(method, path, bytes.NewReader(b), true)
	var f *failure
	if errors.As(e, &f) && f.Status == 401 {
		if _, e = c.connect(); e != nil {
			return nil, e
		}
		r, e = c.request(method, path, bytes.NewReader(b), true)
	}
	if e == nil && strings.HasPrefix(path, "/api/agent-tools") && r["protocol_version"] != float64(1) {
		return nil, fail("protocol_version", "The server/client discovery protocol is incompatible", "Update the plugin through its native manager")
	}
	return r, e
}
func (c *client) call(name string, input map[string]any) (map[string]any, error) {
	b, e := json.Marshal(input)
	if e != nil {
		return nil, e
	}
	return c.authed("POST", "/api/agent-tools/"+url.PathEscape(name), b)
}
func (c *client) logout() (any, error) {
	token, e := c.credential()
	if e != nil {
		return nil, e
	}
	revoked := false
	if token != "" {
		c.token = token
		_, e = c.request("DELETE", "/api/agents/"+url.PathEscape(c.label), nil, true)
		var f *failure
		revoked = e == nil || errors.As(e, &f) && (f.Status == 401 || f.Status == 404)
	}
	if e = c.deleteCredential(); e != nil {
		return nil, e
	}
	return map[string]any{"local_credential_removed": true, "remote_access_revoked": revoked}, nil
}
func (c *client) readBinding() (*binding, error) {
	var b binding
	data, e := os.ReadFile(filepath.Join(c.dir, ".traceshield-project.json"))
	if e != nil {
		return nil, e
	}
	if json.Unmarshal(data, &b) != nil || b.Version != 1 || !validSlug.MatchString(b.Slug) || b.Origin != c.origin {
		return nil, fail("project_binding", "Invalid or incompatible project mapping", "project link SLUG")
	}
	return &b, nil
}
func (c *client) link(slug string) (any, error) {
	if _, e := c.readBinding(); e == nil {
		return nil, fail("project_binding", "Folder is already linked; it was not overwritten", "")
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	me, e := c.authed("GET", "/api/me", nil)
	if e != nil {
		return nil, e
	}
	r, e := c.call("project", map[string]any{"project": slug})
	if e != nil {
		return nil, e
	}
	p, ok := r["result"].(map[string]any)
	if !ok {
		return nil, fail("protocol", "Invalid project response", "")
	}
	id, _ := p["id"].(string)
	if id == "" || p["slug"] != slug {
		return nil, fail("protocol", "Invalid project identity", "")
	}
	email, _ := me["email"].(string)
	live, _ := p["live_url"].(string)
	b := binding{Version: 1, Origin: c.origin, Account: email, ID: id, Slug: slug, URL: live}
	data, _ := json.MarshalIndent(b, "", "  ")
	if e = c.ignore(".traceshield-project.json"); e != nil {
		return nil, e
	}
	if e = privateWrite(filepath.Join(c.dir, ".traceshield-project.json"), data); e != nil {
		return nil, e
	}
	return b, nil
}
func (c *client) verifiedBinding() (*binding, error) {
	b, e := c.readBinding()
	if e != nil {
		return nil, fail("project_binding", "No valid project binding in selected folder", "project link SLUG")
	}
	me, e := c.authed("GET", "/api/me", nil)
	if e != nil {
		return nil, e
	}
	if me["email"] != b.Account {
		return nil, fail("account_mismatch", "Folder belongs to another connected account", "Connect the original account")
	}
	r, e := c.call("project", map[string]any{"project": b.Slug})
	if e != nil {
		return nil, e
	}
	p, ok := r["result"].(map[string]any)
	if !ok || p["id"] != b.ID {
		return nil, fail("project_binding", "Project identity changed; no action performed", "")
	}
	return b, nil
}
func (c *client) git(args ...string) ([]byte, error) {
	cmd := exec.CommandContext(c.ctx, "git", args...)
	cmd.Dir = c.dir
	cmd.Env = gitEnvironment()
	return cmd.Output()
}
func (c *client) ignore(name string) error {
	path := filepath.Join(c.dir, ".gitignore")
	b, e := os.ReadFile(path)
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	for _, line := range strings.Split(string(b), "\n") {
		if line == "/"+name {
			return nil
		}
	}
	text := string(b)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return privateWrite(path, []byte(text+"/"+name+"\n"))
}
func (c *client) upload() (any, error) {
	b, e := c.verifiedBinding()
	if e != nil {
		return nil, e
	}
	top, e := c.git("rev-parse", "--show-toplevel")
	if e != nil {
		return nil, fail("source", "Selected app folder is not a Git repository", "Prepare and commit intended app source first")
	}
	root, e := filepath.EvalSymlinks(strings.TrimSpace(string(top)))
	if e != nil || root != c.dir {
		return nil, fail("source", "Select the Git repository root explicitly; parent source was not uploaded", "Use --dir APP_ROOT")
	}
	if _, e = os.Stat(filepath.Join(c.dir, "Dockerfile")); e != nil {
		return nil, fail("source", "A Dockerfile is required at the selected app root", "Read deployment integration guide")
	}
	dirty, e := c.git("status", "--porcelain", "--untracked-files=no")
	if e != nil {
		return nil, e
	}
	if len(bytes.TrimSpace(dirty)) != 0 {
		return nil, fail("needs_commit", "Commit intended application changes before upload; no files were staged or committed", "Inspect changes and commit only intended app files")
	}
	source, e := c.git("rev-parse", "HEAD")
	if e != nil {
		return nil, fail("source", "No committed app source to upload", "Commit intended app files")
	}
	trees, e := c.git("rev-parse", strings.TrimSpace(string(source))+"^{tree}")
	if e != nil {
		return nil, fail("source", "No committed app source to upload", "Commit intended app files")
	}
	paths, e := c.git("ls-tree", "-r", "-z", "--name-only", strings.TrimSpace(string(trees)))
	if e != nil {
		return nil, e
	}
	for _, name := range strings.Split(string(paths), "\x00") {
		base := strings.ToLower(filepath.Base(name))
		secret := base == ".env" || strings.HasPrefix(base, ".env.") && !contains([]string{".env.example", ".env.sample", ".env.template"}, base) || contains([]string{"id_rsa", "id_ed25519", "credentials.json"}, base) || strings.HasSuffix(base, ".p12") || strings.HasSuffix(base, ".pfx") || strings.HasSuffix(base, ".key")
		if secret {
			return nil, fail("unsafe_source", "Potential credential file exists in the committed app snapshot. No source was uploaded.", "Remove credential files from the committed app snapshot before publishing")
		}
	}
	temp, e := os.MkdirTemp("", "traceshield-source-")
	if e != nil {
		return nil, e
	}
	defer os.RemoveAll(temp)
	date, e := c.git("show", "-s", "--format=%cI", strings.TrimSpace(string(source)))
	if e != nil {
		return nil, e
	}
	objects, e := c.git("rev-parse", "--git-path", "objects")
	if e != nil {
		return nil, e
	}
	objectPath := strings.TrimSpace(string(objects))
	if !filepath.IsAbs(objectPath) {
		objectPath = filepath.Join(c.dir, objectPath)
	}
	if strings.ContainsAny(objectPath, "\r\n") {
		return nil, fail("source", "Unsupported Git object directory path", "")
	}
	snapshotGit := func(args ...string) ([]byte, error) {
		cmd := exec.CommandContext(c.ctx, "git", args...)
		cmd.Dir = temp
		cmd.Env = append(gitEnvironment(), "GIT_AUTHOR_DATE="+strings.TrimSpace(string(date)), "GIT_COMMITTER_DATE="+strings.TrimSpace(string(date)))
		return cmd.Output()
	}
	if _, e = snapshotGit("init", "--bare", "--initial-branch=main", "."); e != nil {
		return nil, e
	}
	if e = os.WriteFile(filepath.Join(temp, "objects", "info", "alternates"), []byte(objectPath+"\n"), 0600); e != nil {
		return nil, e
	}
	snapshot, e := snapshotGit("-c", "user.name=Traceshield", "-c", "user.email=source@traceshield.app", "-c", "commit.gpgSign=false", "commit-tree", strings.TrimSpace(string(trees)), "-m", "App snapshot of "+strings.TrimSpace(string(source)))
	if e != nil {
		return nil, e
	}
	if _, e = snapshotGit("update-ref", "refs/heads/main", strings.TrimSpace(string(snapshot))); e != nil {
		return nil, e
	}
	path := filepath.Join(temp, "source.bundle")
	if _, e = snapshotGit("bundle", "create", path, "HEAD"); e != nil {
		return nil, fail("source", "Could not prepare the committed app snapshot", "")
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if info.Size() > 64<<20 {
		return nil, fail("source_limit", "Committed app snapshot exceeds64MiB; nothing uploaded", "Review committed app size; local history is not uploaded")
	}
	r, e := c.request("POST", "/api/projects/"+b.Slug+"/source", f, true)
	var auth *failure
	if errors.As(e, &auth) && auth.Status == 401 {
		if _, e = c.connect(); e != nil {
			return nil, e
		}
		if _, e = c.verifiedBinding(); e != nil {
			return nil, e
		}
		if _, e = f.Seek(0, io.SeekStart); e != nil {
			return nil, e
		}
		r, e = c.request("POST", "/api/projects/"+b.Slug+"/source", f, true)
	}
	if e != nil {
		return nil, e
	}
	if r["commit_sha"] != strings.TrimSpace(string(snapshot)) {
		return nil, fail("protocol", "Accepted source snapshot does not match the prepared snapshot; no deployment was started", "")
	}
	b.LastUpload = map[string]string{"source_commit_sha": strings.TrimSpace(string(source)), "source_tree_sha": strings.TrimSpace(string(trees)), "commit_sha": strings.TrimSpace(string(snapshot))}
	data, e := json.Marshal(b)
	if e != nil {
		return nil, e
	}
	if e = privateWrite(filepath.Join(c.dir, ".traceshield-project.json"), data); e != nil {
		return nil, fail("receipt_storage", "Source snapshot was uploaded as "+strings.TrimSpace(string(snapshot))+" but its local receipt could not be saved", "Check project-folder permissions; no deployment was started")
	}
	return map[string]any{"project": b.Slug, "commit_sha": r["commit_sha"], "source_ref": r["source_ref"], "source_commit_sha": strings.TrimSpace(string(source)), "source_tree_sha": strings.TrimSpace(string(trees)), "source_scope": "Committed app files only; Git history and untracked files are not uploaded", "uploaded": true, "deployed": false, "next_action": "Call deploy with this project/commit_sha, then deployment_status until terminal and verify public behavior"}, nil
}
func (c *client) pullEnv() (any, error) {
	b, e := c.verifiedBinding()
	if e != nil {
		return nil, e
	}
	r, e := c.authed("GET", "/api/projects/"+b.Slug+"/env?scope=dev", nil)
	if e != nil {
		return nil, e
	}
	items, ok := r["env"].([]any)
	if !ok || r["scope"] != "dev" {
		return nil, fail("protocol", "Invalid development environment response", "")
	}
	updates := map[string]string{}
	keys := []string{}
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fail("protocol", "Invalid environment item", "")
		}
		k, ok := m["key"].(string)
		v, okv := m["value"].(string)
		if !ok || !okv || !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(k) {
			return nil, fail("protocol", "Invalid environment entry", "")
		}
		if strings.ContainsAny(v, "\r\n\x00") || strings.Contains(v, "$") {
			return nil, fail("environment_encoding", "A dev value needs an explicit application env loader; file was not written", "Use a supported env encoding; do not print secret values")
		}
		if k == "DATABASE_URL" {
			database, err := url.Parse(v)
			if err != nil {
				return nil, fail("protocol", "Invalid development database URL", "")
			}
			query := database.Query()
			if query.Get("sslmode") != "" {
				trust, err := c.call("development_tls", map[string]any{"project": b.Slug})
				if err != nil {
					return nil, err
				}
				tls, ok := trust["result"].(map[string]any)
				certificate, _ := tls["ca_pem"].(string)
				if !ok || tls["required"] != true || tls["hostname"] != database.Hostname() || !strings.Contains(certificate, "-----BEGIN CERTIFICATE-----") {
					return nil, fail("database_trust", "Development database trust does not match its endpoint; no credentials written", "")
				}
				name := ".traceshield-postgres-ca.crt"
				if err = c.ignore(name); err != nil {
					return nil, err
				}
				path := filepath.Join(c.dir, name)
				if err = privateWrite(path, []byte(certificate)); err != nil {
					return nil, err
				}
				query.Set("sslmode", "verify-full")
				query.Set("sslrootcert", path)
				database.RawQuery = query.Encode()
				v = database.String()
			}
		}
		encoded, e := dotenvValue(v)
		if e != nil {
			return nil, e
		}
		updates[k] = k + "=" + encoded
		keys = append(keys, k)
	}
	path := filepath.Join(c.dir, ".env.local")
	if st, e := os.Lstat(path); e == nil && !st.Mode().IsRegular() {
		return nil, fail("unsafe_path", "Refusing non-regular local env file", "")
	}
	old, e := os.ReadFile(path)
	if e != nil && !os.IsNotExist(e) {
		return nil, e
	}
	lines := strings.Split(strings.TrimSuffix(string(old), "\n"), "\n")
	if len(old) == 0 {
		lines = nil
	}
	merged := []string{}
	written := map[string]bool{}
	for _, line := range lines {
		k, _, ok := strings.Cut(line, "=")
		key := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(k), "export "))
		if ok {
			if update, found := updates[key]; found {
				_, oldValue, _ := strings.Cut(line, "=")
				oldValue = strings.TrimSpace(oldValue)
				if len(oldValue) > 0 && strings.ContainsAny(oldValue[:1], "'\"`") && !strings.Contains(oldValue[1:], oldValue[:1]) {
					return nil, fail("environment_encoding", "Existing env value spans multiple lines; file was not overwritten", "Resolve multiline entries before pulling dev env")
				}
				if !written[key] {
					merged = append(merged, update)
					written[key] = true
				}
				continue
			}
		}
		merged = append(merged, line)
	}
	lines = merged
	for k := range written {
		delete(updates, k)
	}
	for _, k := range keys {
		if line, ok := updates[k]; ok {
			lines = append(lines, line)
			delete(updates, k)
		}
	}
	if e = c.ignore(".env.local"); e != nil {
		return nil, e
	}
	if inside, e := c.git("rev-parse", "--show-toplevel"); e == nil {
		repo := strings.TrimSpace(string(inside))
		relative, err := filepath.Rel(repo, path)
		if err != nil {
			return nil, err
		}
		cmd := exec.CommandContext(c.ctx, "git", "ls-files", "--error-unmatch", "--", relative)
		cmd.Dir = repo
		cmd.Env = gitEnvironment()
		if cmd.Run() == nil {
			return nil, fail("unsafe_environment", "Local env file is tracked by Git; no credentials written", "Remove credential file from tracking before pulling dev env")
		}
		cmd = exec.CommandContext(c.ctx, "git", "check-ignore", "--quiet", "--", relative)
		cmd.Dir = repo
		cmd.Env = gitEnvironment()
		if cmd.Run() != nil {
			return nil, fail("unsafe_environment", "Local env file is not ignored; no credentials written", "")
		}
	}
	if e = privateWrite(path, []byte(strings.Join(lines, "\n")+"\n")); e != nil {
		return nil, e
	}
	return map[string]any{"scope": "dev", "path": path, "keys": keys, "values_printed": false, "next_action": "Next.js reads .env.local; Node can run with --env-file=.env.local. Use DATABASE_URL unchanged; local data is separate from production. Do not display or partially mask .env.local; print check outcomes only."}, nil
}

func gitEnvironment() []string {
	var env []string
	for _, v := range os.Environ() {
		k, _, _ := strings.Cut(v, "=")
		if runtime.GOOS == "windows" {
			k = strings.ToUpper(k)
		}
		if !strings.HasPrefix(k, "GIT_") {
			env = append(env, v)
		}
	}
	return env
}

func dotenvValue(value string) (string, error) {
	for _, q := range []string{"'", "`", "\""} {
		if !strings.Contains(value, q) && !(q == "\"" && (strings.Contains(value, `\n`) || strings.Contains(value, `\r`))) {
			return q + value + q, nil
		}
	}
	return "", fail("environment_encoding", "A dev value cannot be represented safely for both Node and Next.js; no credentials were written", "Use an explicit application env loader for this value")
}
