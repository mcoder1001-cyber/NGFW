// Package cli implements the `vrx` command: one-shot commands, the interactive REPL (operational and configuration
// mode) and machine mode (--json). It is a thin client of vrx-api: every command maps to documented REST
// operations (see Command.Ops) and nothing here talks to the agent or VPP.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"ngfw/cli/internal/api"
	"ngfw/cli/internal/cpath"
	"ngfw/cli/internal/jschema"
	"ngfw/cli/internal/lineedit"
)

// Version is set at build time (-ldflags "-X ngfw/cli/internal/cli.Version=…").
var Version = "dev"

// DefaultAPI is the on-box API address.
const DefaultAPI = "http://127.0.0.1:3000"

// Mode is the REPL mode.
type Mode int

// REPL modes.
const (
	ModeOperational Mode = iota
	ModeConfig
)

// App is one vrx invocation.
type App struct {
	Stdin          *os.File
	Stdout, Stderr io.Writer
	Getenv         func(string) string

	apiURL       string
	jsonOut      bool
	debug        bool
	apiKeyFile   string
	user         string
	passwordFile string
	noSession    bool

	client        *api.Client
	credSource    string // "api-key-file", "env", "session", "login"
	refreshCookie string // in memory only (interactive login)
	schema        *jschema.Doc
	mode          Mode
	edit          []string
	interactive   bool
	editor        *lineedit.Editor
	hostname      string
	username      string

	candCache   any
	candCacheAt time.Time
}

// New returns an App bound to the process's stdio and environment.
func New() *App {
	return &App{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Getenv: os.Getenv}
}

// Main runs vrx with args (without the program name) and returns the exit code.
func (a *App) Main(args []string) int {
	fs := flag.NewFlagSet("vrx", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	fs.StringVar(&a.apiURL, "api", "", "API base URL (env VRX_API_URL; default "+DefaultAPI+")")
	fs.BoolVar(&a.jsonOut, "json", false, "machine mode: every command prints exactly one JSON document")
	fs.BoolVar(&a.debug, "debug", false, "print method, path and status of every API call to stderr (never credentials)")
	fs.StringVar(&a.apiKeyFile, "api-key-file", "", "read the API key from this file (mode 0600; env VRX_API_KEY_FILE)")
	fs.StringVar(&a.user, "user", "", "log in as this user for this invocation (password prompted without echo)")
	fs.StringVar(&a.passwordFile, "password-file", "", "with --user: read the password from this file (mode 0600) instead of prompting")
	fs.BoolVar(&a.noSession, "no-session", false, "do not read or write the login session file")
	showVersion := fs.Bool("version", false, "print the version and exit")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(a.Stderr, "usage: vrx [flags] [command …]   (no command: interactive shell)\n\nflags:\n")
		fs.PrintDefaults()
		fmt.Fprintf(a.Stderr, "\ncommands: vrx help\n")
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ExitOK
		}
		return ExitUsage
	}
	if *showVersion {
		fmt.Fprintf(a.Stdout, "vrx %s (API spec %s)\n", Version, api.SpecVersion)
		return ExitOK
	}
	if a.apiURL == "" {
		a.apiURL = a.env("VRX_API_URL", DefaultAPI)
	}
	if a.apiKeyFile == "" {
		a.apiKeyFile = a.Getenv("VRX_API_KEY_FILE")
	}
	c, err := api.New(a.apiURL)
	if err != nil {
		return report(a.Stderr, usagef("%v", err), a.jsonOut)
	}
	if a.debug {
		c.Debug = a.Stderr
	}
	a.client = c

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rest := fs.Args()
	if len(rest) == 0 {
		if !lineedit.IsTerminal(a.Stdin) && !a.jsonOut {
			// commands on stdin, one per line (scripts): same as the REPL without prompts
			return a.repl(ctx, false)
		}
		if a.jsonOut {
			return report(a.Stderr, usagef("--json needs a command (machine mode is one-shot)"), true)
		}
		return a.repl(ctx, true)
	}
	toks := make([]cpath.Token, len(rest))
	for i, r := range rest {
		// shell arguments are already unquoted: a word is a JSON literal only if it looks like one
		toks[i] = cpath.Token{Text: r, Quoted: strings.HasPrefix(r, "{") || strings.HasPrefix(r, "[")}
	}
	if err := a.execute(ctx, toks, true); err != nil {
		return report(a.Stderr, err, a.jsonOut)
	}
	return ExitOK
}

func (a *App) env(key, def string) string {
	if v := a.Getenv(key); v != "" {
		return v
	}
	return def
}

// ---- credentials ----

// ensureAuth makes sure the client carries a credential (API key file → VRX_API_KEY → --user login → session file
// → interactive login prompt).
func (a *App) ensureAuth(ctx context.Context) error {
	if a.client.Cred != nil {
		return nil
	}
	if a.apiKeyFile != "" {
		key, err := readSecretFile(a.apiKeyFile)
		if err != nil {
			return usagef("API key file: %v", err)
		}
		a.client.Cred, a.credSource = api.Key(key), "api-key-file"
		return nil
	}
	if k := strings.TrimSpace(a.Getenv("VRX_API_KEY")); k != "" {
		a.client.Cred, a.credSource = api.Key(k), "env"
		return nil
	}
	if a.user != "" {
		var pw string
		var err error
		if a.passwordFile != "" {
			if pw, err = readSecretFile(a.passwordFile); err != nil {
				return usagef("password file: %v", err)
			}
		}
		return a.login(ctx, a.user, pw, false)
	}
	if !a.noSession {
		if s, err := loadSession(a.sessionPath(), a.client.Base.String()); err == nil && s != nil {
			a.client.Cred, a.credSource, a.username = api.Bearer(s.Token), "session", s.User
			return nil
		}
	}
	if a.interactive {
		_, _ = fmt.Fprintln(a.Stdout, "Not logged in (no API key, no session).")
		return a.login(ctx, "", "", !a.noSession)
	}
	return &ExitErr{Code: ExitAuth, Err: errors.New("not logged in — run `vrx login`, or set VRX_API_KEY / --api-key-file")}
}

// readSecretFile reads a credential file that must not be readable by group or others.
func readSecretFile(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if st.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("%s is accessible by group/others (mode %04o); chmod 600 it", path, st.Mode().Perm())
	}
	b, err := os.ReadFile(path) //nolint:gosec // the user names the file on purpose
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(b))
	if v == "" {
		return "", fmt.Errorf("%s is empty", path)
	}
	return v, nil
}

type loginOut struct {
	AccessToken string `json:"accessToken"`
	ExpiresIn   int    `json:"expiresIn"`
	User        struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Role     string `json:"role"`
	} `json:"user"`
}

// login asks for missing username/password (password without echo), calls POST /auth/login, keeps the access
// token in memory, the refresh cookie in memory (interactive refresh), and — when save is set — the access token
// in the 0600 session file.
func (a *App) login(ctx context.Context, user, password string, save bool) error {
	var err error
	if user == "" {
		if !lineedit.IsTerminal(a.Stdin) {
			return usagef("login needs a username (vrx login <user>)")
		}
		if user, err = a.readLine("Username: "); err != nil {
			return err
		}
		user = strings.TrimSpace(user)
	}
	if password == "" {
		if password, err = a.readSecret("Password: "); err != nil {
			return err
		}
	}
	var out loginOut
	r, err := a.client.JSON(ctx, api.Call{Op: "Auth_login", NoAuth: true, Body: map[string]string{"username": user, "password": password}}, &out)
	if err != nil {
		return err
	}
	a.client.Cred, a.credSource, a.username = api.Bearer(out.AccessToken), "login", out.User.Username
	for _, c := range r.Cookies {
		if c.Name == "vrx_refresh" {
			a.refreshCookie = c.Name + "=" + c.Value
		}
	}
	if a.interactive && a.refreshCookie != "" {
		a.client.Refresh = a.refresh
	}
	if save && !a.noSession {
		exp := time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
		if err := saveSession(a.sessionPath(), session{API: a.client.Base.String(), User: out.User.Username, Role: out.User.Role, Token: out.AccessToken, ExpiresAt: exp}); err != nil {
			fmt.Fprintf(a.Stderr, "warning: session not saved: %v\n", err)
		}
	}
	return nil
}

// refresh rotates the in-memory refresh cookie (POST /auth/refresh) and returns a new bearer credential.
func (a *App) refresh(ctx context.Context) (api.Credential, error) {
	var out loginOut
	r, err := a.client.JSON(ctx, api.Call{Op: "Auth_refresh", NoAuth: true, Cookie: a.refreshCookie}, &out)
	if err != nil {
		return nil, err
	}
	for _, c := range r.Cookies {
		if c.Name == "vrx_refresh" {
			a.refreshCookie = c.Name + "=" + c.Value
		}
	}
	return api.Bearer(out.AccessToken), nil
}

func (a *App) readLine(prompt string) (string, error) {
	if a.editor != nil {
		return a.editor.ReadLine(prompt)
	}
	e := &lineedit.Editor{In: a.Stdin, Out: a.Stdout}
	return e.ReadLine(prompt)
}

func (a *App) readSecret(prompt string) (string, error) {
	if a.editor != nil {
		return a.editor.ReadSecret(prompt)
	}
	e := &lineedit.Editor{In: a.Stdin, Out: a.Stdout}
	return e.ReadSecret(prompt)
}

// ---- session file ($XDG_RUNTIME_DIR/vrx/session.json, 0600; access token only, expires with it) ----

type session struct {
	API       string    `json:"api"`
	User      string    `json:"user"`
	Role      string    `json:"role"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func (a *App) sessionPath() string {
	if p := a.Getenv("VRX_SESSION_FILE"); p != "" {
		return p
	}
	dir := a.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		if d, err := os.UserCacheDir(); err == nil {
			dir = d
		} else {
			dir = os.TempDir()
		}
	}
	return filepath.Join(dir, "vrx", "session.json")
}

func loadSession(path, apiURL string) (*session, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s has mode %04o; ignored", path, st.Mode().Perm())
	}
	b, err := os.ReadFile(path) //nolint:gosec // our own session file
	if err != nil {
		return nil, err
	}
	var s session
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	if s.API != apiURL || time.Now().After(s.ExpiresAt) || s.Token == "" {
		return nil, nil
	}
	return &s, nil
}

func saveSession(path string, s session) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ---- schema (live, from the API's OpenAPI document) ----

// Schema loads the configuration schema from GET /api/docs-json once per process.
func (a *App) Schema(ctx context.Context) (*jschema.Doc, error) {
	if a.schema != nil {
		return a.schema, nil
	}
	if err := a.ensureAuth(ctx); err != nil {
		return nil, err
	}
	b, err := a.client.Raw(ctx, "/api/docs-json")
	if err != nil {
		return nil, fmt.Errorf("loading the configuration schema: %w", err)
	}
	d, err := jschema.Load(b)
	if err != nil {
		return nil, err
	}
	a.schema = d
	return d, nil
}

// SetSchema injects a schema (tests).
func (a *App) SetSchema(d *jschema.Doc) { a.schema = d }

// ---- REPL ----

func (a *App) prompt() string {
	host := a.hostname
	if host == "" {
		host = "vrx"
	}
	user := a.username
	if user == "" {
		user = "vrx"
	}
	if a.mode == ModeConfig {
		return user + "@" + host + "# "
	}
	return user + "@" + host + "> "
}

func (a *App) repl(ctx context.Context, tty bool) int {
	a.interactive = tty
	a.editor = &lineedit.Editor{In: a.Stdin, Out: a.Stdout, Complete: a.complete, Help: a.help}
	histPath := a.historyPath()
	if tty {
		a.editor.History = loadHistory(histPath)
		if err := a.ensureAuth(ctx); err != nil {
			return report(a.Stderr, err, false)
		}
		a.greet(ctx)
	}
	last := ExitOK
	for {
		if tty && a.mode == ModeConfig {
			fmt.Fprintf(a.Stdout, "\n[edit%s]\n", editSuffix(a.edit))
		}
		p := ""
		if tty {
			p = a.prompt()
		}
		line, err := a.editor.ReadLine(p)
		if errors.Is(err, lineedit.ErrInterrupt) {
			continue
		}
		if err != nil {
			if tty {
				saveHistory(histPath, a.editor.History)
			}
			return last
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		a.editor.AddHistory(line)
		toks, terr := cpath.Tokenize(line)
		if terr != nil {
			last = report(a.Stderr, usagef("%v", terr), false)
			continue
		}
		if len(toks) == 1 && !toks[0].Quoted && (toks[0].Text == "quit" || (toks[0].Text == "exit" && a.mode == ModeOperational)) {
			if tty {
				saveHistory(histPath, a.editor.History)
			}
			return last
		}
		if err := a.execute(ctx, toks, false); err != nil {
			last = report(a.Stderr, err, false)
			if !tty {
				return last // scripts stop at the first failure
			}
			continue
		}
		last = ExitOK
	}
}

func editSuffix(segs []string) string {
	if len(segs) == 0 {
		return ""
	}
	return " " + cpath.Words(segs)
}

func (a *App) greet(ctx context.Context) {
	var me struct {
		Username      string `json:"username"`
		EffectiveRole string `json:"effectiveRole"`
		Via           string `json:"via"`
	}
	if _, err := a.client.JSON(ctx, api.Call{Op: "Auth_me"}, &me); err == nil {
		a.username = me.Username
		fmt.Fprintf(a.Stdout, "vrx %s — %s as %s (role %s, via %s). Type ? for help.\n", Version, a.client.Base.Host, me.Username, me.EffectiveRole, me.Via)
	}
	var host string
	if _, err := a.client.JSON(ctx, api.Call{Op: "Config_runningAt", Params: map[string]string{"path": "system/hostname"}}, &host); err == nil && host != "" {
		a.hostname = host
	}
}

func (a *App) historyPath() string {
	if p := a.Getenv("VRX_HISTORY_FILE"); p != "" {
		return p
	}
	dir := a.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "vrx", "history")
}

func loadHistory(path string) []string {
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path) //nolint:gosec // our own history file
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// saveHistory writes the history (0600) without lines that could carry a credential.
func saveHistory(path string, h []string) {
	if path == "" {
		return
	}
	var keep []string
	for _, l := range h {
		low := strings.ToLower(l)
		if strings.Contains(low, "password") || strings.Contains(low, "secret") || strings.Contains(low, "psk") || strings.Contains(low, "vrxk_") {
			continue
		}
		keep = append(keep, l)
	}
	if len(keep) > 1000 {
		keep = keep[len(keep)-1000:]
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	_ = os.WriteFile(path, []byte(strings.Join(keep, "\n")+"\n"), 0o600)
}
