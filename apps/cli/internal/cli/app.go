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
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"ngfw/cli/internal/api"
	"ngfw/cli/internal/cpath"
	"ngfw/cli/internal/jschema"
	"ngfw/cli/internal/lineedit"
	"ngfw/cli/internal/safe"
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

	// term is the unfiltered terminal (the line editor draws on it); Stdout/Stderr are wrapped in safe.Writer in
	// human mode so server-supplied text cannot inject terminal control sequences (review H1).
	term io.Writer

	apiURL       string
	jsonOut      bool
	debug        bool
	apiKeyFile   string
	user         string
	passwordFile string
	noSession    bool
	insecureHTTP bool
	sessionExp   time.Time

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

	pending    *pendingCommit // the applied-but-unconfirmed commit, if any (review M3)
	exitWarned bool
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
	fs.BoolVar(&a.insecureHTTP, "insecure-http", false, "allow http:// to a host that is not loopback (credentials travel in cleartext)")
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
	a.term = a.Stdout
	if !a.jsonOut {
		so, se := &safe.Writer{W: a.Stdout}, &safe.Writer{W: a.Stderr}
		a.Stdout, a.Stderr = so, se
		defer func() { _ = so.Flush(); _ = se.Flush() }()
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
	if c.Base.Scheme == "http" && !isLoopback(c.Base.Hostname()) && !a.insecureHTTP {
		// review M4: passwords and tokens must not cross the network in cleartext
		return report(a.Stderr, usagef("refusing http:// to %s: credentials would travel in cleartext — use https:// (trust a self-signed certificate with SSL_CERT_FILE) or --insecure-http", c.Base.Host), a.jsonOut)
	}
	if a.debug {
		c.Debug = a.Stderr
	}
	a.client = c

	rest := fs.Args()
	if len(rest) == 0 {
		if !lineedit.IsTerminal(a.Stdin) && !a.jsonOut {
			// commands on stdin, one per line (scripts): same as the REPL without prompts
			return a.repl(context.Background(), false)
		}
		if a.jsonOut {
			return report(a.Stderr, usagef("--json needs a command (machine mode is one-shot)"), true)
		}
		return a.repl(context.Background(), true)
	}
	// one-shot: the process is the command, so a signal cancels the process context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
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

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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
			a.client.Cred, a.credSource, a.username, a.sessionExp = api.Bearer(s.Token), "session", s.User, s.ExpiresAt
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
	b, err := readPrivate(path)
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
			if !a.interactive {
				return usagef("logged in, but the session was not saved: %v", err)
			}
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
	e := &lineedit.Editor{In: a.Stdin, Out: a.term}
	return e.ReadLine(prompt)
}

func (a *App) readSecret(prompt string) (string, error) {
	if a.editor != nil {
		return a.editor.ReadSecret(prompt)
	}
	e := &lineedit.Editor{In: a.Stdin, Out: a.term}
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

// sessionPath is $VRX_SESSION_FILE, else $XDG_RUNTIME_DIR/vrx/session.json, else /run/user/<uid>/vrx/session.json
// when that directory is ours; "" = no session persistence (never a shared temp directory — review M1).
func (a *App) sessionPath() string {
	if p := a.Getenv("VRX_SESSION_FILE"); p != "" {
		return p
	}
	dir := a.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		d := fmt.Sprintf("/run/user/%d", os.Getuid())
		if privateDir(d, false) != nil {
			return ""
		}
		dir = d
	}
	return filepath.Join(dir, "vrx", "session.json")
}

func loadSession(path, apiURL string) (*session, error) {
	if path == "" {
		return nil, nil
	}
	b, err := readPrivate(path)
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
	if path == "" {
		return errors.New("no private runtime directory ($XDG_RUNTIME_DIR unset) — set VRX_SESSION_FILE to a file in a 0700 directory you own, or use an API key")
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return writePrivate(path, b)
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
	host, user = safe.String(host), safe.String(user)
	if a.pending != nil { // review M3: countdown of the unconfirmed commit in the prompt
		left := int(time.Until(a.pending.Deadline).Round(time.Second).Seconds())
		if left < 0 {
			left = 0
		}
		host += fmt.Sprintf("[!%ds]", left)
	}
	if a.mode == ModeConfig {
		return user + "@" + host + "# "
	}
	return user + "@" + host + "> "
}

func (a *App) repl(base context.Context, tty bool) int {
	a.interactive = tty
	a.editor = &lineedit.Editor{In: a.Stdin, Out: a.term, Complete: a.complete, Help: a.help}
	histPath := a.historyPath()

	// review H2: Ctrl-C / SIGTERM cancel only the command that is running (each command gets its own context);
	// SIGTERM then ends the shell. Idle, the terminal is in raw mode (Ctrl-C is a key, not a signal); a SIGTERM
	// while idle restores the terminal and exits.
	var (
		mu        sync.Mutex
		cancelCur context.CancelFunc
		terminate bool
	)
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	defer func() { signal.Stop(sigs); close(sigs) }()
	go func() {
		for sig := range sigs {
			mu.Lock()
			if cancelCur != nil {
				cancelCur()
				if sig == syscall.SIGTERM {
					terminate = true
				}
			} else if sig == syscall.SIGTERM || !tty {
				a.editor.RestoreTerminal()
				code := 130
				if sig == syscall.SIGTERM {
					code = 143
				}
				os.Exit(code) //nolint:gocritic // idle shell: nothing to clean up but the terminal
			}
			mu.Unlock()
		}
	}()
	run := func(f func(ctx context.Context) error) error {
		ctx, cancel := context.WithCancel(base)
		mu.Lock()
		cancelCur = cancel
		mu.Unlock()
		err := f(ctx)
		mu.Lock()
		cancelCur = nil
		mu.Unlock()
		cancel()
		return err
	}

	if tty {
		a.editor.History = loadHistory(histPath)
		if err := run(a.ensureAuth); err != nil {
			return report(a.Stderr, err, false)
		}
		_ = run(func(ctx context.Context) error { a.greet(ctx); return nil })
	}
	last := ExitOK
	leaving := func() bool {
		// review M3: never leave silently while a confirmed commit is waiting to be confirmed
		if tty {
			_ = run(func(ctx context.Context) error { a.refreshPending(ctx); return nil })
		}
		if a.pending != nil && !a.exitWarned {
			a.exitWarned = true
			fmt.Fprintf(a.Stdout, "warning: %s\nThe shell stays open: type `confirm`, or `exit` again to leave and let it revert.\n", a.pendingNote())
			return false
		}
		if tty {
			saveHistory(histPath, a.editor.History)
		}
		return true
	}
	for {
		mu.Lock()
		stop := terminate
		mu.Unlock()
		if stop {
			if tty {
				saveHistory(histPath, a.editor.History)
			}
			return 143
		}
		p := ""
		if tty {
			_ = run(func(ctx context.Context) error { a.refreshPending(ctx); return nil })
			if a.mode == ModeConfig {
				fmt.Fprintf(a.Stdout, "\n[edit%s]\n", safe.String(editSuffix(a.edit)))
			}
			if a.pending != nil {
				fmt.Fprintf(a.Stdout, "! %s\n", a.pendingNote())
			}
			p = a.prompt()
		}
		line, err := a.editor.ReadLine(p)
		if errors.Is(err, lineedit.ErrInterrupt) {
			continue
		}
		if err != nil {
			if tty && !leaving() {
				continue
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
			a.discardTypeahead(tty)
			if !tty {
				return last
			}
			continue
		}
		if len(toks) == 1 && !toks[0].Quoted && (toks[0].Text == "quit" || (toks[0].Text == "exit" && a.mode == ModeOperational)) {
			if leaving() {
				return last
			}
			continue
		}
		a.exitWarned = false
		if err := run(func(ctx context.Context) error { return a.execute(ctx, toks, false) }); err != nil {
			last = report(a.Stderr, err, false)
			if !tty {
				return last // scripts stop at the first failure
			}
			a.discardTypeahead(tty)
			continue
		}
		last = ExitOK
	}
}

// discardTypeahead drops input typed or pasted ahead of a failed command (review M6: a pasted block stops at its
// first failing line; a trailing `commit` is not run).
func (a *App) discardTypeahead(tty bool) {
	if !tty {
		return
	}
	if n := a.editor.DiscardTypeahead(); n > 0 {
		fmt.Fprintf(a.Stderr, "(stopped: %d byte(s) of pasted/typed-ahead input after the failing line were discarded)\n", n)
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
	b, err := readPrivate(path)
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
	_ = writePrivate(path, []byte(strings.Join(keep, "\n")+"\n"))
}
