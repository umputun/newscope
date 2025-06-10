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
	"github.com/umputun/newscope/pkg/feed"
	"github.com/umputun/newscope/pkg/llm"
	"github.com/umputun/newscope/pkg/repository"
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
		if errors.As(err, &flagsErr) && errors.Is(flagsErr.Type, flags.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(1)
	}

	if opts.Version {
		fmt.Printf("Version: %s\nGolang: %s\n", revision, runtime.Version())
		os.Exit(0)
	}

	// handle termination signals
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	err := run(ctx, opts)
	cancel()

	if err != nil {
		log.Printf("[ERROR] %v", err)
		os.Exit(1)
	}

	log.Print("[INFO] shutdown complete")
}

func run(ctx context.Context, opts Opts) error {
	// load configuration first
	cfg, err := config.Load(opts.Config)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// setup logging with secrets for redaction
	SetupLog(opts.Debug, cfg.LLM.APIKey)

	log.Printf("[INFO] starting newscope version %s", revision)

	// setup database repositories
	repoCfg := repository.Config{
		DSN:             cfg.Database.DSN,
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: time.Duration(cfg.Database.ConnMaxLifetime) * time.Second,
	}
	repos, err := repository.NewRepositories(ctx, repoCfg)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer repos.Close()

	// setup LLM classifier - required for system to function
	if cfg.LLM.Endpoint == "" || cfg.LLM.APIKey == "" {
		return fmt.Errorf("LLM classifier is required - missing endpoint or API key configuration")
	}

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
	classifier := llm.NewClassifier(cfg.LLM)
	log.Printf("[INFO] LLM classifier enabled with model: %s", cfg.LLM.Model)

	// setup and start scheduler
	schedulerCfg := scheduler.Config{
		UpdateInterval:             cfg.Schedule.UpdateInterval,
		MaxWorkers:                 cfg.Schedule.MaxWorkers,
		PreferenceSummaryThreshold: cfg.LLM.Classification.PreferenceSummaryThreshold,
		CleanupAge:                 cfg.Schedule.CleanupAge,
		CleanupMinScore:            cfg.Schedule.CleanupMinScore,
		CleanupInterval:            cfg.Schedule.CleanupInterval,
		RetryAttempts:              cfg.Schedule.RetryAttempts,
		RetryInitialDelay:          cfg.Schedule.RetryInitialDelay,
		RetryMaxDelay:              cfg.Schedule.RetryMaxDelay,
		RetryJitter:                cfg.Schedule.RetryJitter,
	}

	// warn if jitter is disabled
	if cfg.Schedule.RetryJitter == 0 {
		log.Printf("[WARN] retry jitter is set to 0, this may cause thundering herd problems under high database contention")
	}
	deps := scheduler.Params{
		FeedManager:           repos.Feed,
		ItemManager:           repos.Item,
		ClassificationManager: repos.Classification,
		SettingManager:        repos.Setting,
		Parser:                feedParser,
		Extractor:             contentExtractor,
		Classifier:            classifier,
	}
	sched := scheduler.NewScheduler(deps, schedulerCfg)
	sched.Start(ctx)
	defer sched.Stop()

	// setup and run server with repository adapter
	repoAdapter := server.NewRepositoryAdapter(repos)
	srv := server.New(cfg, repoAdapter, sched, revision, opts.Debug)
	if err := srv.Run(ctx); err != nil {
		return fmt.Errorf("server failed: %w", err)
	}

	return nil
}

// SetupLog configures the logger
func SetupLog(dbg bool, secs ...string) {
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
