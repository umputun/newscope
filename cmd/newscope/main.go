package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/fatih/color"
	"github.com/go-pkgz/lgr"
	"github.com/jessevdk/go-flags"

	"github.com/umputun/newscope/server"
)

// Opts with all CLI options
type Opts struct {
	Listen  string `short:"l" long:"listen" env:"LISTEN" default:":8080" description:"listen address"`
	Verbose bool   `short:"v" long:"verbose" description:"verbose mode"`

	// Common options
	Debug   bool `long:"dbg" env:"DEBUG" description:"debug mode"`
	Version bool `short:"V" long:"version" description:"show version info"`
	NoColor bool `long:"no-color" env:"NO_COLOR" description:"disable color output"`
}

var revision = "unknown"

func main() {
	var opts Opts
	parser := flags.NewParser(&opts, flags.Default)
	if _, err := parser.Parse(); err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		}
		os.Exit(1)
	}

	if opts.Version {
		fmt.Printf("Version: %s\nGolang: %s\n", revision, runtime.Version())
		os.Exit(0)
	}

	setupLog(opts.Debug)

	log.Printf("[INFO] starting newscope version %s", revision)

	ctx, cancel := context.WithCancel(context.Background())

	// handle termination signals
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		log.Print("[INFO] termination signal received")
		cancel()
	}()

	// initialize and run server
	srv := server.New(server.Config{
		Listen:  opts.Listen,
		Version: revision,
		Debug:   opts.Debug,
	})

	err := srv.Run(ctx)
	cancel()
	
	if err != nil {
		log.Printf("[ERROR] server failed: %v", err)
		os.Exit(1)
	}

	log.Print("[INFO] shutdown complete")
}

func setupLog(dbg bool, secs ...string) {
	logOpts := []lgr.Option{lgr.Out(io.Discard), lgr.Err(io.Discard)}
	if dbg {
		logOpts = []lgr.Option{lgr.Debug, lgr.Msec, lgr.LevelBraces, lgr.StackTraceOnError}
	}

	colorizer := lgr.Mapper{
		ErrorFunc:  func(s string) string { return color.New(color.FgHiRed).Sprint(s) },
		WarnFunc:   func(s string) string { return color.New(color.FgRed).Sprint(s) },
		InfoFunc:   func(s string) string { return color.New(color.FgYellow).Sprint(s) },
		DebugFunc:  func(s string) string { return color.New(color.FgWhite).Sprint(s) },
		CallerFunc: func(s string) string { return color.New(color.FgBlue).Sprint(s) },
		TimeFunc:   func(s string) string { return color.New(color.FgCyan).Sprint(s) },
	}
	logOpts = append(logOpts, lgr.Map(colorizer))
	if len(secs) > 0 {
		logOpts = append(logOpts, lgr.Secret(secs...))
	}
	lgr.SetupStdLogger(logOpts...)
	lgr.Setup(logOpts...)
}