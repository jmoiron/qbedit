package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"net/http"

	"github.com/jmoiron/qbedit/internal/app"
	flag "github.com/spf13/pflag"
)

// version is set at build time via -ldflags; defaults to dev.
var version = "dev"

func main() {
	var (
		listen      string
		mcVersion   string
		showVersion bool
		verbose     int
		quit        bool
	)

	flag.StringVar(&listen, "addr", "0.0.0.0:8222", "listen address for the web UI (host:port)")
	flag.StringVar(&mcVersion, "mcv", "1.20.1", "Minecraft version (e.g., 1.20.1)")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.CountVarP(&verbose, "verbose", "v", "increase verbosity; repeat for more detail")
	flag.BoolVarP(&quit, "quit", "q", false, "initialize (load templates, parse chapters), then exit without serving")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: qbedit [options] <ftbquests-dir>\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if showVersion {
		fmt.Println(version)
		return
	}

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	root := flag.Arg(0)
	abs, err := filepath.Abs(root)
	if err != nil {
		log.Fatalf("resolve dir: %v", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		log.Fatalf("invalid directory: %v", err)
	}
	if !info.IsDir() {
		log.Fatalf("not a directory: %s", abs)
	}

	debugf := func(format string, args ...any) {
		if verbose > 0 {
			log.Printf(format, args...)
		}
	}

	debugf("verbosity: %d", verbose)
	fmt.Printf("qbedit %s\n", version)

	// Start app server
	a, err := app.New(abs, mcVersion, verbose)
	if err != nil {
		log.Fatalf("init: %v", err)
	}
	log.Printf("scan summary: %d parsed, %d failed", len(a.QB.Chapters), 0)
	if quit {
		log.Printf("initialized successfully; loaded %d chapters; quitting (--quit)", len(a.QB.Chapters))
		return
	}
	log.Printf("listening on http://%s (mc %s)", listen, mcVersion)
	if err := httpListenAndServe(listen, a.Router()); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// httpListenAndServe exists to facilitate testing/mocking if desired.
var httpListenAndServe = func(addr string, h http.Handler) error {
	return http.ListenAndServe(addr, h)
}
