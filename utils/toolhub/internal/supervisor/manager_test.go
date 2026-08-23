package supervisor

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"

	"Lib/utils/toolhub/internal/catalog"
	"github.com/stretchr/testify/require"
)

// TestHelperProcess is re-entered in a subprocess by integration tests. Keeping
// the fixture in the test binary avoids depending on shell commands with
// platform-specific signal behavior.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("TOOLHUB_HELPER_PROCESS") != "1" {
		return
	}
	separator := indexOf(os.Args, "--")
	if separator < 0 || separator+1 >= len(os.Args) {
		os.Exit(2)
	}
	mode := os.Args[separator+1]
	switch mode {
	case "serve":
		runHelperServer(os.Args[separator+2])
	case "hang":
		waitForHelperInterrupt()
	case "fail":
		fmt.Fprintln(os.Stderr, "fixture failed deliberately")
		os.Exit(7)
	default:
		os.Exit(2)
	}
}

// waitForHelperInterrupt emulates a process that stays alive but never exposes
// the configured health endpoint.
func waitForHelperInterrupt() {
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)
	<-interrupts
}

// runHelperServer emulates a well-behaved web tool that releases its port when
// ToolHub sends the same SIGINT as a terminal Ctrl+C.
func runHelperServer(address string) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		os.Exit(3)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})}
	go func() { _ = server.Serve(listener) }()
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)
	<-interrupts
	_ = server.Shutdown(context.Background())
}

// TestManagerStartsAndStopsService covers process-group ownership, health-based
// readiness, and graceful Ctrl+C shutdown as one observable lifecycle.
func TestManagerStartsAndStopsService(t *testing.T) {
	address := reserveAddress(t)
	tool := helperTool(t, "service", catalog.KindService, "serve", address)
	manager := NewManager([]catalog.Tool{tool})

	require.NoError(t, manager.Start(tool.ID, nil))
	waitForStatus(t, manager, tool.ID, StatusRunning)
	view := manager.List()[0]
	require.True(t, view.Owned)
	require.Positive(t, view.PID)

	require.NoError(t, manager.Stop(tool.ID))
	waitForStatus(t, manager, tool.ID, StatusStopped)
	require.False(t, manager.List()[0].Owned)
	require.Nil(t, manager.List()[0].ExitCode)
}

// TestManagerDiscoversExternalService verifies the critical safety boundary:
// healthy ports are useful to open but never treated as ToolHub-owned.
func TestManagerDiscoversExternalService(t *testing.T) {
	server := http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
	})}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })

	tool := helperTool(t, "external", catalog.KindService, "serve", listener.Addr().String())
	manager := NewManager([]catalog.Tool{tool})
	manager.Refresh(context.Background())

	require.Equal(t, StatusExternal, manager.List()[0].Status)
	require.ErrorIs(t, manager.Stop(tool.ID), ErrNotOwned)
	require.ErrorIs(t, manager.Start(tool.ID, nil), ErrAlreadyActive)
}

// TestManagerReportsTaskFailure preserves both the exit code and stderr so a
// user can recover from a broken dependency or invalid command.
func TestManagerReportsTaskFailure(t *testing.T) {
	tool := helperTool(t, "task", catalog.KindTask, "fail", "")
	manager := NewManager([]catalog.Tool{tool})

	require.NoError(t, manager.Start(tool.ID, nil))
	waitForStatus(t, manager, tool.ID, StatusFailed)
	view := manager.List()[0]
	require.NotNil(t, view.ExitCode)
	require.Equal(t, 7, *view.ExitCode)
	logs, err := manager.Logs(tool.ID)
	require.NoError(t, err)
	require.Contains(t, logs.Content, "fixture failed deliberately")
}

// TestManagerTerminatesServiceThatNeverBecomesHealthy verifies startup timeout
// cleanup and ensures the failed process does not remain owned or orphaned.
func TestManagerTerminatesServiceThatNeverBecomesHealthy(t *testing.T) {
	address := reserveAddress(t)
	tool := helperTool(t, "unready", catalog.KindService, "hang", address)
	tool.StartupTimeout.Duration = 250 * time.Millisecond
	manager := NewManager([]catalog.Tool{tool})

	require.NoError(t, manager.Start(tool.ID, nil))
	waitForStatus(t, manager, tool.ID, StatusFailed)
	waitForOwnership(t, manager, tool.ID, false)
	require.Contains(t, manager.List()[0].Error, "健康检查")
}

// TestBuildArgumentsValidatesStructuredInputs ensures task forms cannot smuggle
// shell syntax because every value remains one literal argv element.
func TestBuildArgumentsValidatesStructuredInputs(t *testing.T) {
	tool := catalog.Tool{
		Args: []string{"base"},
		Inputs: []catalog.Input{
			{ID: "video", Label: "视频", Type: "text", Position: true, Required: true},
			{ID: "language", Label: "语言", Type: "select", Flag: "--language", Options: []string{"ja", "auto"}},
			{ID: "offline", Label: "离线", Type: "boolean", Flag: "--offline"},
		},
	}
	arguments, err := buildArguments(tool, map[string]string{
		"video": "/tmp/a video; touch nope.mp4", "language": "ja", "offline": "true",
	})

	require.NoError(t, err)
	require.Equal(t, []string{"base", "/tmp/a video; touch nope.mp4", "--language", "ja", "--offline"}, arguments)
}

// TestRefreshDoesNotStealReservedStart protects the small interval between a
// click reserving a tool and exec.Start publishing its owned process handle.
func TestRefreshDoesNotStealReservedStart(t *testing.T) {
	manager := NewManager([]catalog.Tool{{
		ID: "service", Name: "Service", Kind: catalog.KindService,
		HealthURL: "http://127.0.0.1:1", URL: "http://127.0.0.1:1",
	}})
	record, _, err := manager.reserveStart("service")
	require.NoError(t, err)

	manager.applyHealth("service", true)
	require.Equal(t, StatusStarting, record.status)
}

// TestStartRejectsFailedProcessStillBeingCleanedUp closes the overlap window
// between publishing a startup failure and reaping its terminated child.
func TestStartRejectsFailedProcessStillBeingCleanedUp(t *testing.T) {
	manager := NewManager([]catalog.Tool{{ID: "task", Name: "Task", Kind: catalog.KindTask}})
	manager.records["task"].status = StatusFailed
	manager.records["task"].command = &exec.Cmd{}

	require.ErrorIs(t, manager.Start("task", nil), ErrAlreadyActive)
}

// TestConsecutiveHealthFailuresMarkOwnedServiceUnhealthy confirms transient
// failures are tolerated once and become visible when repeated.
func TestConsecutiveHealthFailuresMarkOwnedServiceUnhealthy(t *testing.T) {
	manager := NewManager([]catalog.Tool{{ID: "service", Name: "Service", Kind: catalog.KindService}})
	record := manager.records["service"]
	record.status = StatusRunning
	record.command = &exec.Cmd{}

	manager.applyHealth("service", false)
	require.Equal(t, StatusRunning, record.status)
	manager.applyHealth("service", false)
	require.Equal(t, StatusUnhealthy, record.status)
	require.NotEmpty(t, record.lastError)
}

// helperTool creates a catalog entry that re-enters this test binary.
func helperTool(t *testing.T, id, kind, mode, address string) catalog.Tool {
	t.Helper()
	tool := catalog.Tool{
		ID: id, Name: id, Kind: kind, Command: os.Args[0],
		Args:              []string{"-test.run=TestHelperProcess", "--", mode},
		Environment:       map[string]string{"TOOLHUB_HELPER_PROCESS": "1"},
		ResolvedDirectory: t.TempDir(),
		StartupTimeout:    catalog.Duration{Duration: 3 * time.Second},
		StopTimeout:       catalog.Duration{Duration: time.Second},
	}
	if kind == catalog.KindService {
		tool.Args = append(tool.Args, address)
		tool.URL = "http://" + address
		tool.HealthURL = tool.URL
	}
	return tool
}

// reserveAddress obtains a currently free loopback port for a child fixture.
func reserveAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	return address
}

// waitForStatus bounds asynchronous assertions and reports the latest view on
// timeout instead of leaving a hanging test process.
func waitForStatus(t *testing.T, manager *Manager, id, expected string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, view := range manager.List() {
			if view.ID == id && view.Status == expected {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("tool %s did not reach %s; latest=%+v", id, expected, manager.List())
}

// waitForOwnership handles cleanup that may finish after the visible failure
// transition but must still remain bounded.
func waitForOwnership(t *testing.T, manager *Manager, id string, expected bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, view := range manager.List() {
			if view.ID == id && view.Owned == expected {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("tool %s ownership did not become %t; latest=%+v", id, expected, manager.List())
}

// indexOf locates the argument separator used by the re-executed test binary.
func indexOf(values []string, target string) int {
	for index, value := range values {
		if strings.TrimSpace(value) == target {
			return index
		}
	}
	return -1
}
