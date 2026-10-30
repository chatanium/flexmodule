package main

import (
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"
)

// TestMain keeps the integration-only CLI mode separate from ordinary tests.
// Invoke a root test executable with:
// --integration-runtime-return-hold <seconds:1..60> -- run <real run arguments>
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--integration-runtime-return-hold" {
		code := runIntegrationRuntimeReturnHold(os.Args[2:])
		if code == 2 {
			fmt.Fprintln(os.Stderr, "integration runtime return-hold invocation or marker failed")
		}
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func runIntegrationRuntimeReturnHold(args []string) int {
	if len(args) < 3 || args[1] != "--" || args[2] != "run" {
		return 2
	}
	seconds, err := strconv.Atoi(args[0])
	if err != nil || seconds < 1 || seconds > 60 {
		return 2
	}

	// This is the actual runtime owner, not a child process or a test result.
	// Do not serialize the returned error: it may contain operator data.
	code, class := 0, "ok"
	if runCLI(args[2:]) != nil {
		code, class = 1, "error"
	}
	_, markerErr := fmt.Fprintf(os.Stdout, "{\"integration\":\"runtime-return-hold\",\"phase\":\"returned\",\"pid\":%d,\"class\":\"%s\",\"exit_code\":%d}\n", os.Getpid(), class, code)

	// Retain this exact PID for a finite post-return observation window, even
	// after failure. No m.Run result can replace the actual runtime outcome.
	time.Sleep(time.Duration(seconds) * time.Second)
	if markerErr != nil {
		return 2
	}
	return code
}
