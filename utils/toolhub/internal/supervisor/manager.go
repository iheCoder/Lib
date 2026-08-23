// Package supervisor owns the small amount of process lifecycle state ToolHub
// needs to reproduce terminal-style start, Ctrl+C, logging, and status checks.
package supervisor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"Lib/utils/toolhub/internal/catalog"
)

const (
	StatusStopped   = "stopped"
	StatusStarting  = "starting"
	StatusRunning   = "running"
	StatusExternal  = "external"
	StatusUnhealthy = "unhealthy"
	StatusStopping  = "stopping"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	defaultLogLimit = 256 << 10
)

var (
	ErrAlreadyActive = errors.New("tool is already active")
	ErrNotOwned      = errors.New("tool is running outside ToolHub")
	ErrNotFound      = errors.New("tool not found")
)

// View is the immutable process snapshot returned to the HTTP layer.
type View struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Category    string          `json:"category"`
	Kind        string          `json:"kind"`
	URL         string          `json:"url,omitempty"`
	Status      string          `json:"status"`
	Owned       bool            `json:"owned"`
	PID         int             `json:"pid,omitempty"`
	StartedAt   *time.Time      `json:"startedAt,omitempty"`
	FinishedAt  *time.Time      `json:"finishedAt,omitempty"`
	Error       string          `json:"error,omitempty"`
	ExitCode    *int            `json:"exitCode,omitempty"`
	Inputs      []catalog.Input `json:"inputs,omitempty"`
}

// Logs is a bounded process-output snapshot.
type Logs struct {
	Content string    `json:"content"`
	Updated time.Time `json:"updatedAt,omitempty"`
}

type record struct {
	definition            catalog.Tool
	status                string
	command               *exec.Cmd
	logs                  *LogBuffer
	startedAt             *time.Time
	finishedAt            *time.Time
	lastError             string
	exitCode              *int
	stopRequested         bool
	terminatingForFailure bool
	generation            uint64
	healthFailures        int
}

// Manager coordinates commands but deliberately does not persist PIDs. After a
// ToolHub restart, healthy services are rediscovered as external and never
// killed, which is safer than trusting a stale process identifier.
type Manager struct {
	mutex   sync.RWMutex
	records map[string]*record
	order   []string
	client  *http.Client
}

// NewManager creates stopped records in catalog order for a stable dashboard.
func NewManager(tools []catalog.Tool) *Manager {
	manager := &Manager{
		records: make(map[string]*record, len(tools)),
		client:  &http.Client{Timeout: 800 * time.Millisecond},
	}
	for _, tool := range tools {
		manager.order = append(manager.order, tool.ID)
		manager.records[tool.ID] = &record{
			definition: tool,
			status:     StatusStopped,
			logs:       NewLogBuffer(defaultLogLimit),
		}
	}
	return manager
}

// StartMonitor keeps externally started services and managed health state fresh
// without requiring the dashboard to mutate status through polling requests.
func (manager *Manager) StartMonitor(ctx context.Context) {
	manager.Refresh(ctx)
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				manager.Refresh(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// List returns a catalog-ordered immutable status snapshot.
func (manager *Manager) List() []View {
	manager.mutex.RLock()
	defer manager.mutex.RUnlock()
	views := make([]View, 0, len(manager.order))
	for _, id := range manager.order {
		views = append(views, viewOf(manager.records[id]))
	}
	return views
}

// Start reserves the tool immediately so concurrent clicks cannot spawn two
// instances, then starts work asynchronously from the HTTP request lifecycle.
func (manager *Manager) Start(id string, inputs map[string]string) error {
	record, generation, err := manager.reserveStart(id)
	if err != nil {
		return err
	}
	go manager.startReserved(record, generation, inputs)
	return nil
}

// reserveStart is the single transition gate from an inactive state to
// starting. Completed and failed tasks may be run again with fresh logs.
func (manager *Manager) reserveStart(id string) (*record, uint64, error) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	record, found := manager.records[id]
	if !found {
		return nil, 0, ErrNotFound
	}
	// A failed startup can spend a short bounded interval terminating its child.
	// Command ownership, not only the visible status, prevents an overlapping run.
	if record.command != nil || isActive(record.status) || record.status == StatusExternal {
		return nil, 0, ErrAlreadyActive
	}
	record.generation++
	record.status = StatusStarting
	record.lastError = ""
	record.exitCode = nil
	record.startedAt = nil
	record.finishedAt = nil
	record.stopRequested = false
	record.terminatingForFailure = false
	record.logs = NewLogBuffer(defaultLogLimit)
	return record, record.generation, nil
}

// startReserved checks for an existing service before constructing the exact
// argument vector and launching a new process group.
func (manager *Manager) startReserved(record *record, generation uint64, inputs map[string]string) {
	if record.definition.Kind == catalog.KindService && manager.isHealthy(record.definition.HealthURL) {
		manager.markExternal(record, generation)
		return
	}
	args, err := buildArguments(record.definition, inputs)
	if err != nil {
		manager.markStartFailure(record, generation, err)
		return
	}
	command := prepareCommand(record.definition, args, record.logs)
	if err := command.Start(); err != nil {
		manager.markStartFailure(record, generation, fmt.Errorf("start command: %w", err))
		return
	}
	manager.attachCommand(record, generation, command)
	go manager.waitForExit(record, generation, command)
	if record.definition.Kind == catalog.KindTask {
		manager.markRunning(record, generation)
		return
	}
	manager.awaitServiceHealth(record, generation, command)
}

// prepareCommand preserves the current environment, applies explicit catalog
// overrides, captures logs, and isolates the child process group for Ctrl+C.
func prepareCommand(tool catalog.Tool, args []string, logs *LogBuffer) *exec.Cmd {
	command := exec.Command(tool.Command, args...)
	command.Dir = tool.ResolvedDirectory
	command.Env = append([]string(nil), os.Environ()...)
	for key, value := range tool.Environment {
		command.Env = append(command.Env, key+"="+value)
	}
	command.Stdout = logs
	command.Stderr = logs
	configureProcessGroup(command)
	return command
}

// buildArguments converts form values into an argv array without invoking a
// shell. This makes spaces and Chinese paths ordinary values, not syntax.
func buildArguments(tool catalog.Tool, values map[string]string) ([]string, error) {
	arguments := append([]string(nil), tool.Args...)
	for _, input := range tool.Inputs {
		value := strings.TrimSpace(values[input.ID])
		if value == "" {
			value = input.Default
		}
		if input.Required && value == "" {
			return nil, fmt.Errorf("%s is required", input.Label)
		}
		if value == "" {
			continue
		}
		if input.Type == "boolean" {
			enabled, err := strconv.ParseBool(value)
			if err != nil {
				return nil, fmt.Errorf("%s must be true or false", input.Label)
			}
			if enabled {
				arguments = append(arguments, input.Flag)
			}
			continue
		}
		if input.Type == "select" && !contains(input.Options, value) {
			return nil, fmt.Errorf("%s has an unsupported value", input.Label)
		}
		if !input.Position {
			arguments = append(arguments, input.Flag)
		}
		arguments = append(arguments, value)
	}
	return arguments, nil
}

// attachCommand publishes process ownership only after Start returns a real PID.
func (manager *Manager) attachCommand(record *record, generation uint64, command *exec.Cmd) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if record.generation != generation {
		return
	}
	now := time.Now()
	record.command = command
	record.startedAt = &now
}

// awaitServiceHealth waits only for the configured startup window. A process
// that never becomes ready is terminated so it cannot become a hidden orphan.
func (manager *Manager) awaitServiceHealth(record *record, generation uint64, command *exec.Cmd) {
	timeout := record.definition.StartupTimeout.Duration
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if manager.isHealthy(record.definition.HealthURL) {
			manager.markRunning(record, generation)
			return
		}
		if !manager.matchesActiveCommand(record, generation, command) {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	manager.failAndTerminate(record, generation, command, "服务未在启动时限内通过健康检查")
}

// waitForExit classifies expected cancellation, successful task completion,
// and unexpected service exits without losing the child's actual exit code.
func (manager *Manager) waitForExit(record *record, generation uint64, command *exec.Cmd) {
	err := command.Wait()
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if record.generation != generation || record.command != command {
		return
	}
	now := time.Now()
	record.finishedAt = &now
	record.exitCode = exitCodeOf(command)
	record.command = nil
	if record.terminatingForFailure {
		return
	}
	if record.stopRequested {
		record.status = StatusStopped
		record.lastError = ""
		// go run and npm may translate a delivered SIGINT into a non-zero wrapper
		// exit code. The user requested this termination, so it is not a failure
		// signal and should not be presented as one in the stopped state.
		record.exitCode = nil
		return
	}
	if record.definition.Kind == catalog.KindTask && err == nil {
		record.status = StatusSucceeded
		return
	}
	record.status = StatusFailed
	record.lastError = describeExit(err)
}

// Stop sends the same interrupt a foreground terminal would send. Only
// ToolHub-owned processes are eligible; external services are never guessed at.
func (manager *Manager) Stop(id string) error {
	manager.mutex.Lock()
	record, found := manager.records[id]
	if !found {
		manager.mutex.Unlock()
		return ErrNotFound
	}
	if record.status == StatusExternal {
		manager.mutex.Unlock()
		return ErrNotOwned
	}
	if record.command == nil || !isActive(record.status) {
		manager.mutex.Unlock()
		return nil
	}
	command := record.command
	generation := record.generation
	record.stopRequested = true
	record.status = StatusStopping
	timeout := record.definition.StopTimeout.Duration
	manager.mutex.Unlock()

	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	_ = interruptProcessGroup(command)
	go manager.forceKillAfter(record, generation, command, timeout)
	return nil
}

// forceKillAfter bounds shutdown for tools that ignore Ctrl+C and includes all
// child processes in the same group.
func (manager *Manager) forceKillAfter(record *record, generation uint64, command *exec.Cmd, timeout time.Duration) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	<-timer.C
	if manager.matchesActiveCommand(record, generation, command) {
		_ = killProcessGroup(command)
	}
}

// Refresh probes services without holding the manager lock across network I/O.
func (manager *Manager) Refresh(ctx context.Context) {
	for _, snapshot := range manager.serviceSnapshots() {
		select {
		case <-ctx.Done():
			return
		default:
			manager.applyHealth(snapshot.id, manager.isHealthy(snapshot.healthURL))
		}
	}
}

type serviceSnapshot struct {
	id        string
	healthURL string
}

// serviceSnapshots isolates immutable probe inputs from mutable runtime state.
func (manager *Manager) serviceSnapshots() []serviceSnapshot {
	manager.mutex.RLock()
	defer manager.mutex.RUnlock()
	var snapshots []serviceSnapshot
	for _, id := range manager.order {
		record := manager.records[id]
		if record.definition.Kind == catalog.KindService {
			snapshots = append(snapshots, serviceSnapshot{id: id, healthURL: record.definition.HealthURL})
		}
	}
	return snapshots
}

// applyHealth distinguishes external discovery from a managed process becoming
// unhealthy. Two consecutive failures avoid flashing on a transient request.
func (manager *Manager) applyHealth(id string, healthy bool) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	record := manager.records[id]
	if record.command == nil {
		// A reserved start has a short interval before its command is attached.
		// Its own startup probe owns that transition; the background monitor must
		// not misclassify the freshly launched process as an external instance.
		if record.status == StatusStarting {
			return
		}
		if healthy {
			record.status = StatusExternal
			record.lastError = ""
		} else if record.status == StatusExternal {
			record.status = StatusStopped
		}
		return
	}
	if record.status != StatusRunning && record.status != StatusUnhealthy {
		return
	}
	if healthy {
		record.healthFailures = 0
		record.status = StatusRunning
		record.lastError = ""
		return
	}
	record.healthFailures++
	if record.healthFailures >= 2 {
		record.status = StatusUnhealthy
		record.lastError = "进程仍在运行，但健康检查连续失败"
	}
}

// Logs returns output even after the process has stopped, which is essential
// for diagnosing startup and unexpected-exit failures.
func (manager *Manager) Logs(id string) (Logs, error) {
	manager.mutex.RLock()
	record, found := manager.records[id]
	manager.mutex.RUnlock()
	if !found {
		return Logs{}, ErrNotFound
	}
	content, updated := record.logs.Snapshot()
	return Logs{Content: content, Updated: updated}, nil
}

// isHealthy accepts any 2xx-3xx response and closes bodies immediately so
// frequent local probes cannot leak sockets.
func (manager *Manager) isHealthy(rawURL string) bool {
	request, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return false
	}
	response, err := manager.client.Do(request)
	if err != nil {
		return false
	}
	_ = response.Body.Close()
	return response.StatusCode >= 200 && response.StatusCode < 400
}

// markExternal completes startup without claiming ownership when the declared
// health endpoint was already available.
func (manager *Manager) markExternal(record *record, generation uint64) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if record.generation == generation {
		record.status = StatusExternal
		record.lastError = ""
		record.startedAt = nil
	}
}

// markRunning accepts readiness only for the current start generation.
func (manager *Manager) markRunning(record *record, generation uint64) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if record.generation == generation && record.status == StatusStarting {
		record.status = StatusRunning
		record.healthFailures = 0
	}
}

// markStartFailure publishes argument, directory, or exec errors for the UI.
func (manager *Manager) markStartFailure(record *record, generation uint64, err error) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if record.generation == generation {
		record.status = StatusFailed
		record.lastError = err.Error()
	}
}

// failAndTerminate preserves the startup diagnosis while cleaning up the
// process that failed to become healthy.
func (manager *Manager) failAndTerminate(record *record, generation uint64, command *exec.Cmd, message string) {
	manager.mutex.Lock()
	if record.generation == generation && record.command == command {
		record.status = StatusFailed
		record.lastError = message
		record.terminatingForFailure = true
	}
	manager.mutex.Unlock()
	_ = interruptProcessGroup(command)
	go manager.forceKillAfter(record, generation, command, time.Second)
}

// matchesActiveCommand prevents delayed goroutines from touching a later run.
func (manager *Manager) matchesActiveCommand(record *record, generation uint64, command *exec.Cmd) bool {
	manager.mutex.RLock()
	defer manager.mutex.RUnlock()
	return record.generation == generation && record.command == command
}

// viewOf copies mutable record state into a transport-safe value.
func viewOf(record *record) View {
	view := View{
		ID: record.definition.ID, Name: record.definition.Name,
		Description: record.definition.Description, Category: record.definition.Category,
		Kind: record.definition.Kind, URL: record.definition.URL,
		Status: record.status, Owned: record.command != nil, StartedAt: record.startedAt,
		FinishedAt: record.finishedAt, Error: record.lastError,
		ExitCode: record.exitCode, Inputs: record.definition.Inputs,
	}
	if record.command != nil && record.command.Process != nil {
		view.PID = record.command.Process.Pid
	}
	return view
}

// isActive lists states in which a second start would violate process ownership.
func isActive(status string) bool {
	return status == StatusStarting || status == StatusRunning ||
		status == StatusUnhealthy || status == StatusStopping
}

// exitCodeOf returns nil only when the operating system never produced state.
func exitCodeOf(command *exec.Cmd) *int {
	if command.ProcessState == nil {
		return nil
	}
	code := command.ProcessState.ExitCode()
	return &code
}

// describeExit distinguishes a surprising clean service exit from a non-zero
// process error while keeping both actionable in Chinese UI copy.
func describeExit(err error) string {
	if err == nil {
		return "服务进程意外结束"
	}
	return "进程异常退出：" + err.Error()
}

// contains validates one select value against its central catalog enum.
func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
