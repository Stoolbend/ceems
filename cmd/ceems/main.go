package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"ceems/internal/auth"
	"ceems/internal/config"
	"ceems/internal/httpapi"
	"ceems/internal/jobs"
	"ceems/internal/output"
	"ceems/internal/store"
	"ceems/web"
)

const usage = "usage: ceems serve [-config path] | ceems import [-config path] <dir>\n" +
	"       ceems admin create [-config path] <username> | ceems admin reset-password [-config path] <username>"

func usageExit() {
	fmt.Fprintln(os.Stderr, usage)
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		usageExit()
	}
	cmd, args := os.Args[1], os.Args[2:]
	switch {
	case cmd == "admin" && len(args) > 0 && (args[0] == "create" || args[0] == "reset-password"):
		cmd, args = "admin "+args[0], args[1:]
	case cmd != "serve" && cmd != "import":
		usageExit()
	}
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	path := fs.String("config", "ceems.json", "path to the JSON config file")
	fs.Parse(args)
	if cmd != "serve" && fs.NArg() != 1 {
		usageExit()
	}

	var err error
	switch cmd {
	case "serve":
		err = serve(*path)
	case "import":
		err = importDir(*path, fs.Arg(0), os.Stdout)
	case "admin create":
		err = adminCreate(*path, fs.Arg(0), os.Stdout)
	case "admin reset-password":
		err = adminResetPassword(*path, fs.Arg(0), os.Stdout)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ceems:", err)
		os.Exit(1)
	}
}

// openStore loads the config, creates the data directory and opens the store.
func openStore(configPath string) (config.Config, *store.Store, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return config.Config{}, nil, err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return config.Config{}, nil, err
	}
	db, err := store.Open(cfg.DataDir)
	if err != nil {
		return config.Config{}, nil, err
	}
	return cfg, db, nil
}

func serve(configPath string) error {
	cfg, db, err := openStore(configPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := checkAgentDevListen(cfg.Listen); err != nil {
		return err
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	// The source is built at start so a bad key fails fast.
	var src jobs.Source = jobs.Unconfigured{}
	if cfg.GoogleKeyFile != "" {
		if src, err = jobs.NewSheets(context.Background(), cfg.GoogleKeyFile); err != nil {
			return fmt.Errorf("googleKeyFile: %w", err)
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	out, err := output.Open(cfg.PagesDir, db, log) // refuses to start on the ownership rule
	if err != nil {
		return err
	}
	db.OnPageChange(out.Notify) // registered before the scheduler's first sweep
	outDone := make(chan struct{})
	go func() {
		defer close(outDone)
		out.Run(ctx)
	}()
	runner := jobs.New(db, src, log)
	schedDone := make(chan struct{})
	go func() {
		defer close(schedDone)
		runner.Start(ctx)
	}()

	warnIfNoAdmin(db, log, configPath)
	log.Info("listening", "addr", cfg.Listen)
	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           httpapi.New(web.FS, log, db, cfg.SecureCookies, runner, cfg.TrustedProxyPrefixes, cfg.PagesDir),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second, // a run-now request is bounded by its own 60 s timeout
		IdleTimeout:       120 * time.Second,
	}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done() // the signal no longer ends the process by itself
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(sctx); err != nil {
			log.Warn("shutdown", "err", err)
		}
	}()
	err = srv.ListenAndServe()
	stop() // also ends the scheduler and the shutdown goroutine when the listener failed
	<-shutdownDone
	<-schedDone // a sweep in progress finishes or fails before the deferred db.Close
	<-outDone   // so does a reconcile in progress
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// warnIfNoAdmin logs the bootstrap command when no enabled admin exists; serving continues.
func warnIfNoAdmin(db *store.Store, log *slog.Logger, configPath string) {
	ok, err := db.HasAdmin()
	if err != nil {
		log.Error("checking for an admin user", "err", err)
		return
	}
	if !ok {
		log.Warn("no admin user exists", "run", "ceems admin create -config "+configPath+" <username>")
	}
}

// adminCreate creates an admin with a random password, printed once to out.
func adminCreate(configPath, username string, out io.Writer) error {
	name, err := auth.CheckUsername(username)
	if err != nil {
		return err
	}
	_, db, err := openStore(configPath)
	if err != nil {
		return err
	}
	defer db.Close()
	pw := auth.RandomPassword()
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	_, err = db.CreateUser(name, hash, true, store.Audit{Actor: "cli", Action: "user.create", Detail: name})
	if errors.Is(err, store.ErrUserExists) {
		return fmt.Errorf("user %q already exists", name)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "created admin %s\npassword: %s\n", name, pw)
	return nil
}

// adminResetPassword gives an existing user a new random password, printed once
// to out, and deletes their sessions.
func adminResetPassword(configPath, username string, out io.Writer) error {
	name, err := auth.CheckUsername(username)
	if err != nil {
		return err
	}
	_, db, err := openStore(configPath)
	if err != nil {
		return err
	}
	defer db.Close()
	pw := auth.RandomPassword()
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	err = db.SetPasswordHash(name, hash, store.Audit{Actor: "cli", Action: "user.reset_password", Detail: name})
	if errors.Is(err, store.ErrUserNotFound) {
		return fmt.Errorf("user %q not found", name)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "password for %s reset\npassword: %s\n", name, pw)
	return nil
}

// importDir loads every .tti file directly inside dir into the page store.
func importDir(configPath, dir string, out io.Writer) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var files []store.ImportFile
	for _, e := range entries {
		if !e.Type().IsRegular() || !strings.EqualFold(filepath.Ext(e.Name()), ".tti") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files = append(files, store.ImportFile{Path: path, TTI: b})
	}
	if len(files) == 0 {
		return fmt.Errorf("%s: no .tti files", dir)
	}
	_, db, err := openStore(configPath)
	if err != nil {
		return err
	}
	defer db.Close()
	added, err := db.ImportPages(files, "cli")
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "import: %d added, %d unchanged\n", added, len(files)-added)
	return nil
}
