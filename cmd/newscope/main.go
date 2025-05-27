package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/go-pkgz/lgr"
	"github.com/jessevdk/go-flags"

	"github.com/umputun/newscope/pkg/config"
	"github.com/umputun/newscope/pkg/content"
	"github.com/umputun/newscope/pkg/db"
	"github.com/umputun/newscope/pkg/feed"
	"github.com/umputun/newscope/pkg/llm"
	"github.com/umputun/newscope/pkg/scheduler"
	"github.com/umputun/newscope/server"
)

// Opts with all CLI options
type Opts struct {
	Config string `short:"c" long:"config" env:"CONFIG" default:"config.yml" description:"configuration file"`

	// common options
	Debug   bool `long:"dbg" env:"DEBUG" description:"debug mode"`
	Version bool `short:"V" long:"version" description:"show version info"`
	NoColor bool `long:"no-color" env:"NO_COLOR" description:"disable color output"`
}

var revision = "unknown"

func main() {
	var opts Opts
	parser := flags.NewParser(&opts, flags.Default)
	if _, err := parser.Parse(); err != nil {
		var flagsErr *flags.Error
		if errors.As(err, &flagsErr) && flagsErr.Type == flags.ErrHelp {
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

	// load configuration
	cfg, err := config.Load(opts.Config)
	if err != nil {
		log.Printf("[ERROR] failed to load config: %v", err)
		os.Exit(1)
	}

	// setup database
	dbCfg := db.Config{
		DSN:             cfg.Database.DSN,
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: time.Duration(cfg.Database.ConnMaxLifetime) * time.Second,
	}
	dbConn, err := db.New(context.Background(), dbCfg)
	if err != nil {
		log.Printf("[ERROR] failed to initialize database: %v", err)
		os.Exit(1)
	}
	defer dbConn.Close()

	ctx, cancel := context.WithCancel(context.Background())

	// handle termination signals
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		log.Print("[INFO] termination signal received")
		cancel()
	}()

	// setup feed parser and content extractor
	feedParser := feed.NewParser(cfg.Server.Timeout)

	var contentExtractor *content.HTTPExtractor
	if cfg.Extraction.Enabled {
		contentExtractor = content.NewHTTPExtractor(cfg.Extraction.Timeout, cfg.Extraction.UserAgent)
		if cfg.Extraction.FallbackURL != "" {
			contentExtractor.SetFallbackURL(cfg.Extraction.FallbackURL)
		}
		contentExtractor.SetOptions(cfg.Extraction.MinTextLength, cfg.Extraction.IncludeImages, cfg.Extraction.IncludeLinks)
	}

	// setup LLM classifier
	var classifier *llm.Classifier
	if cfg.LLM.Endpoint != "" && cfg.LLM.APIKey != "" {
		classifier = llm.NewClassifier(cfg.LLM)
		log.Printf("[INFO] LLM classifier enabled with model: %s", cfg.LLM.Model)
	} else {
		log.Printf("[WARN] LLM classifier disabled - no endpoint or API key configured")
	}

	// setup and start scheduler
	schedulerCfg := scheduler.Config{
		UpdateInterval:   time.Duration(cfg.Schedule.UpdateInterval) * time.Minute,
		ExtractInterval:  time.Duration(cfg.Schedule.ExtractInterval) * time.Minute,
		ClassifyInterval: time.Duration(cfg.Schedule.ClassifyInterval) * time.Minute,
		MaxWorkers:       cfg.Schedule.MaxWorkers,
	}
	sched := scheduler.NewScheduler(dbConn, feedParser, contentExtractor, classifier, schedulerCfg)
	sched.Start(ctx)
	defer sched.Stop()

	// setup and run server with database adapter
	dbAdapter := &server.DBAdapter{DB: dbConn}
	srv := server.New(cfg, dbAdapter, sched, revision, opts.Debug)
	if err := srv.Run(ctx); err != nil {
		log.Printf("[ERROR] server failed: %v", err)
		return
	}

	log.Print("[INFO] shutdown complete")
}

func setupLog(dbg bool, secs ...string) {
	logOpts := []lgr.Option{lgr.Msec, lgr.LevelBraces, lgr.StackTraceOnError}
	if dbg {
		logOpts = []lgr.Option{lgr.Debug, lgr.CallerFile, lgr.CallerFunc, lgr.Msec, lgr.LevelBraces, lgr.StackTraceOnError}
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
