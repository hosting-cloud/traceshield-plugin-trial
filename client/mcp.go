package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// mcpAgent is set from the MCP client's name, since agents do not mark MCP servers like they mark shell commands.
var mcpAgent string

type mcpTool struct {
	name, description string
	props             map[string]any
	required          []string
	args              func(in map[string]any) []string
}

func str(in map[string]any, key string) string { s, _ := in[key].(string); return s }

var folderProp = map[string]any{"type": "string", "description": "Absolute path of the app root folder"}

// Each tool runs the same command as the CLI, so behavior and errors match `traceshield --help`.
var mcpTools = []mcpTool{
	{"status", "Check the saved Traceshield connection and, with app_folder, the folder's linked project.", map[string]any{"app_folder": folderProp}, nil,
		func(in map[string]any) []string { return []string{"--dir", dirOr(in), "status"} }},
	{"connect", "Sign in through the browser (reuses a saved connection). Waits up to 10 minutes for approval.", map[string]any{}, nil,
		func(map[string]any) []string { return []string{"connect"} }},
	{"tools_list", "List backend operations with descriptions and JSON input schemas.", map[string]any{"app_folder": folderProp}, nil,
		func(in map[string]any) []string { return []string{"--dir", dirOr(in), "tools", "list"} }},
	{"call", "Call one backend operation from tools_list with its JSON input (nonsecret values only).", map[string]any{
		"app_folder": folderProp, "name": map[string]any{"type": "string"}, "input": map[string]any{"type": "object"}}, []string{"name"},
		func(in map[string]any) []string {
			b, _ := json.Marshal(in["input"])
			if in["input"] == nil {
				b = []byte("{}")
			}
			return []string{"--dir", dirOr(in), "tools", "call", str(in, "name"), string(b)}
		}},
	{"project_link", "Link the app folder to an existing project slug (private, gitignored mapping).", map[string]any{
		"app_folder": folderProp, "slug": map[string]any{"type": "string"}}, []string{"app_folder", "slug"},
		func(in map[string]any) []string {
			return []string{"--dir", str(in, "app_folder"), "project", "link", str(in, "slug")}
		}},
	{"publish", "Optionally commit the listed files first (initializing Git at app_folder if it has no repository), then push the committed HEAD to the linked project's private Traceshield repository, deploy that exact SHA and wait for live or failed. Only committed work is sent.", map[string]any{
		"app_folder": folderProp,
		"files":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "App files to stage and commit, relative to app_folder; never secrets or unrelated work"},
		"message":    map[string]any{"type": "string", "description": "Commit message, required with files"}}, []string{"app_folder"},
		func(in map[string]any) []string { return []string{"--dir", str(in, "app_folder"), "publish"} }},
	{"deployment_wait", "Wait for one deployment to become live or failed.", map[string]any{
		"app_folder": folderProp, "deployment_id": map[string]any{"type": "string"}}, []string{"deployment_id"},
		func(in map[string]any) []string {
			return []string{"--dir", dirOr(in), "deployment", "wait", str(in, "deployment_id")}
		}},
	{"env_pull", "Write the project's private development environment to the folder's gitignored .env.local (values are not returned).", map[string]any{"app_folder": folderProp}, []string{"app_folder"},
		func(in map[string]any) []string { return []string{"--dir", str(in, "app_folder"), "env", "pull"} }},
}

func dirOr(in map[string]any) string {
	if d := str(in, "app_folder"); d != "" {
		return d
	}
	return "."
}

// serveMCP speaks MCP over stdio: one JSON-RPC message per line.
func serveMCP(ctx context.Context) {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 4<<20)
	out := json.NewEncoder(os.Stdout)
	for in.Scan() {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(in.Bytes(), &req) != nil || req.ID == nil {
			continue // notifications need no reply
		}
		var result any
		var rpcErr any
		switch req.Method {
		case "initialize":
			var p struct {
				ProtocolVersion string                `json:"protocolVersion"`
				ClientInfo      struct{ Name string } `json:"clientInfo"`
			}
			_ = json.Unmarshal(req.Params, &p)
			if strings.Contains(strings.ToLower(p.ClientInfo.Name), "codex") {
				mcpAgent = "Codex"
			}
			if p.ProtocolVersion == "" {
				p.ProtocolVersion = "2025-06-18"
			}
			result = map[string]any{"protocolVersion": p.ProtocolVersion, "capabilities": map[string]any{"tools": map[string]any{}},
				"serverInfo": map[string]any{"name": "traceshield", "version": version}}
		case "ping":
			result = map[string]any{}
		case "tools/list":
			tools := []any{}
			for _, t := range mcpTools {
				schema := map[string]any{"type": "object", "properties": t.props}
				if t.required != nil {
					schema["required"] = t.required
				}
				tools = append(tools, map[string]any{"name": t.name, "description": t.description, "inputSchema": schema})
			}
			result = map[string]any{"tools": tools}
		case "tools/call":
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &p)
			result = callTool(ctx, p.Name, p.Arguments)
		default:
			rpcErr = map[string]any{"code": -32601, "message": "Method not found"}
		}
		resp := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		if rpcErr != nil {
			resp["error"] = rpcErr
		} else {
			resp["result"] = result
		}
		_ = out.Encode(resp)
	}
}

func callTool(ctx context.Context, name string, in map[string]any) any {
	if in == nil {
		in = map[string]any{}
	}
	var body any
	for _, t := range mcpTools {
		if t.name == name {
			var data any
			err := commitFiles(ctx, name, in)
			if err == nil {
				data, err = run(ctx, t.args(in))
			}
			if err == nil {
				body = map[string]any{"ok": true, "data": data}
			} else {
				var f *failure
				if !errors.As(err, &f) {
					f = &failure{Code: "local_error", Message: err.Error()}
				}
				body = map[string]any{"ok": false, "error": f}
			}
		}
	}
	if body == nil {
		body = map[string]any{"ok": false, "error": map[string]string{"code": "usage", "message": "Unknown tool"}}
	}
	b, _ := json.Marshal(body)
	ok, _ := body.(map[string]any)["ok"].(bool)
	return map[string]any{"content": []any{map[string]any{"type": "text", "text": string(b)}}, "isError": !ok}
}

// commitFiles lets publish commit for agents whose shell sandbox cannot write .git (Codex default mode).
func commitFiles(ctx context.Context, name string, in map[string]any) error {
	list, _ := in["files"].([]any)
	if name != "publish" || len(list) == 0 {
		return nil
	}
	dir, msg := str(in, "app_folder"), str(in, "message")
	if !filepath.IsAbs(dir) || msg == "" {
		return fail("usage", "files needs an absolute app_folder and a message", "")
	}
	files := []string{}
	for _, f := range list {
		if s, ok := f.(string); ok && s != "" && !strings.HasPrefix(s, "-") {
			files = append(files, s)
		}
	}
	git := func(args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir, cmd.Env = dir, gitEnvironment()
		out, err := cmd.CombinedOutput()
		if err != nil {
			return out, fail("git_commit", "git "+args[0]+" failed: "+strings.TrimSpace(string(out)), "")
		}
		return out, nil
	}
	if _, err := git("rev-parse", "--git-dir"); err != nil {
		if _, err := git("init", "-b", "main"); err != nil {
			return err
		}
	}
	if _, err := git(append([]string{"add", "--"}, files...)...); err != nil {
		return err
	}
	if _, err := git("diff", "--cached", "--quiet"); err == nil {
		return nil // nothing new to commit
	}
	args := []string{"commit", "-m", msg}
	if out, _ := git("config", "user.email"); len(bytes.TrimSpace(out)) == 0 {
		// Local per-commit identity only; the user's Git config stays untouched.
		args = append([]string{"-c", "user.name=Traceshield app", "-c", "user.email=app@traceshield.app"}, args...)
	}
	_, err := git(args...)
	return err
}
