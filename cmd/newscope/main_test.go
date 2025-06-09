package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_MissingConfig(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	opts := Opts{
		Config: "non-existent-config.yml",
	}

	err := run(ctx, opts)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to load config")
}

func TestRun_InvalidConfig(t *testing.T) {
	// create a temporary invalid config file
	tmpFile, err := os.CreateTemp("", "invalid-config-*.yml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// write invalid yaml
	_, err = tmpFile.WriteString("invalid: yaml: content: [")
	require.NoError(t, err)
	tmpFile.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	opts := Opts{
		Config: tmpFile.Name(),
	}

	err = run(ctx, opts)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to load config")
}

func TestRun_ServerStartStop(t *testing.T) {
	// create temp directory for database
	tmpDir, err := os.MkdirTemp("", "newscope-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// set environment variable for config
	err = os.Setenv("DB_PATH", tmpDir)
	require.NoError(t, err)
	defer os.Unsetenv("DB_PATH")

	t.Logf("DB_PATH set to: %s", tmpDir)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	serverErr := make(chan error, 1)

	// get absolute path to config file
	wd, err := os.Getwd()
	require.NoError(t, err)
	configPath := wd + "/testdata/test_config.yml"

	opts := Opts{
		Config: configPath,
	}

	// start server
	go func() {
		err := run(ctx, opts)
		if err != nil {
			t.Logf("Server error: %v", err)
			if ctx.Err() == nil {
				serverErr <- err
			}
		}
		close(serverErr)
	}()

	// wait for server to start
	time.Sleep(2 * time.Second)

	// check if server failed to start
	select {
	case err := <-serverErr:
		t.Fatalf("Server failed to start: %v", err)
	default:
		// server is running
	}

	// test that server is running by making a request
	resp, err := http.Get("http://127.0.0.1:18765/ping")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "pong", string(body))

	// shutdown
	cancel()

	// wait for server to stop
	select {
	case err := <-serverErr:
		if err != nil {
			t.Logf("Server stopped with error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Server shutdown timeout")
	}
}

func TestSetupLog(t *testing.T) {
	t.Run("debug mode enabled", func(t *testing.T) {
		// capture log output to verify debug mode
		SetupLog(true)
		// the function should complete without panic
		// we can't easily test logger configuration directly
	})

	t.Run("debug mode disabled", func(t *testing.T) {
		SetupLog(false)
		// the function should complete without panic
	})

	t.Run("with secrets", func(t *testing.T) {
		// test that secrets are passed through to logger
		SetupLog(true, "secret1", "secret2")
		// the function should complete without panic
		// secrets configuration is internal to lgr
	})

	t.Run("no color mode", func(t *testing.T) {
		// test that the function works without color
		oldNoColor := os.Getenv("NO_COLOR")
		os.Setenv("NO_COLOR", "1")
		defer os.Setenv("NO_COLOR", oldNoColor)

		SetupLog(false)
		// the function should complete without panic
	})
}
