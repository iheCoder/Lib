package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeCommandRunner struct {
	outputs map[string]string
	started [][]string
	err     error
}

// Output selects fixtures by command name for plutil and by preference key for
// defaults, mirroring the two production read paths without shell execution.
func (runner *fakeCommandRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	if runner.err != nil {
		return nil, runner.err
	}
	if output, exists := runner.outputs[name]; exists {
		return []byte(output), nil
	}
	return []byte(runner.outputs[args[len(args)-1]]), nil
}

// Start records argv as discrete values so tests catch accidental shell joining
// or loss of IINA's separate-window option.
func (runner *fakeCommandRunner) Start(name string, args ...string) error {
	runner.started = append(runner.started, append([]string{name}, args...))
	return runner.err
}

func TestRecentSessionsFallsBackToLastPreference(t *testing.T) {
	videoPath := createVideoFile(t, "my video.mp4")
	runner := &fakeCommandRunner{outputs: map[string]string{
		lastFileKey: videoPath + "\n", lastPositionKey: "211.4666666667\n",
	}}

	sessions, err := newIINAService(runner, t.TempDir()).RecentSessions(context.Background())
	if err != nil {
		t.Fatalf("RecentSessions() error = %v", err)
	}
	record := sessions[0].Playbacks[0]
	if record.Name != "my video.mp4" || record.PositionSeconds != 211.4666666667 || !record.Available {
		t.Fatalf("RecentSessions() fallback record = %#v", record)
	}
}

// TestResumeSessionOpensEveryVideoSeparately locks down argv boundaries: paths
// containing whitespace and shell-looking text remain individual arguments.
func TestResumeSessionOpensEveryVideoSeparately(t *testing.T) {
	dataDirectory := t.TempDir()
	first := createVideoFileIn(t, dataDirectory, "a video.mp4")
	second := createVideoFileIn(t, dataDirectory, "b $(touch nope).mp4")
	closedAt := time.Date(2026, 7, 15, 19, 18, 53, 0, time.Local)
	runner := sessionFixture(t, dataDirectory, closedAt, []fixtureEntry{
		{Hash: "AAA", Path: first, Position: 10},
		{Hash: "BBB", Path: second, Position: 20},
	})
	service := newIINAService(runner, dataDirectory)

	sessions, err := service.RecentSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ResumeSession(context.Background(), sessions[0].ID); err != nil {
		t.Fatalf("ResumeSession() error = %v", err)
	}
	want := [][]string{{iinaCLIPath, "--no-stdin", "--separate-windows", first, second}}
	if !reflect.DeepEqual(runner.started, want) {
		t.Fatalf("started commands = %#v, want %#v", runner.started, want)
	}
}

func TestRecentSessionsGroupsShutdownBatches(t *testing.T) {
	dataDirectory := t.TempDir()
	newest := time.Date(2026, 7, 15, 19, 18, 53, 0, time.Local)
	entries := []fixtureEntry{
		{Hash: "AAA", Path: createVideoFileIn(t, dataDirectory, "one.mp4"), Position: 11, ModifiedAt: newest},
		{Hash: "BBB", Path: createVideoFileIn(t, dataDirectory, "two.mp4"), Position: 22, ModifiedAt: newest.Add(-time.Second)},
		{Hash: "CCC", Path: createVideoFileIn(t, dataDirectory, "older.mp4"), Position: 33, ModifiedAt: newest.Add(-20 * time.Second)},
	}
	runner := sessionFixture(t, dataDirectory, newest, entries)

	sessions, err := newIINAService(runner, dataDirectory).RecentSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 || len(sessions[0].Playbacks) != 2 || len(sessions[1].Playbacks) != 1 {
		t.Fatalf("unexpected session grouping: %#v", sessions)
	}
	if sessions[0].Playbacks[0].PositionSeconds != 11 || sessions[0].AvailableCount != 2 {
		t.Fatalf("unexpected newest session: %#v", sessions[0])
	}
}

func TestRecentSessionsIgnoresRedirectCheckpoint(t *testing.T) {
	dataDirectory := t.TempDir()
	closedAt := time.Now().Add(-time.Minute)
	valid := fixtureEntry{Hash: "VALID", Path: createVideoFileIn(t, dataDirectory, "video.mp4"), Position: 12}
	runner := sessionFixture(t, dataDirectory, closedAt, []fixtureEntry{valid})
	writeCheckpoint(t, dataDirectory, "REDIRECT", "# redirect entry\n", closedAt)

	sessions, err := newIINAService(runner, dataDirectory).RecentSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || len(sessions[0].Playbacks) != 1 {
		t.Fatalf("redirect checkpoint leaked into sessions: %#v", sessions)
	}
}

func TestRecentSessionsWithoutHistory(t *testing.T) {
	runner := &fakeCommandRunner{err: errors.New("preference not found")}
	_, err := newIINAService(runner, t.TempDir()).RecentSessions(context.Background())
	if !errors.Is(err, errNoPlaybackHistory) {
		t.Fatalf("RecentSessions() error = %v, want errNoPlaybackHistory", err)
	}
}

type fixtureEntry struct {
	Hash       string
	Path       string
	Position   float64
	ModifiedAt time.Time
}

// sessionFixture writes realistic checkpoint files while keeping the keyed
// archive itself as an in-memory plutil response.
func sessionFixture(t *testing.T, dataDirectory string, defaultTime time.Time, entries []fixtureEntry) *fakeCommandRunner {
	t.Helper()
	for _, entry := range entries {
		modifiedAt := entry.ModifiedAt
		if modifiedAt.IsZero() {
			modifiedAt = defaultTime
		}
		content := fmt.Sprintf("start=%f\nvolume=100.000000\n", entry.Position)
		writeCheckpoint(t, dataDirectory, entry.Hash, content, modifiedAt)
	}
	return &fakeCommandRunner{outputs: map[string]string{"plutil": historyXMLFixture(entries)}}
}

func writeCheckpoint(t *testing.T, dataDirectory, hash, content string, modifiedAt time.Time) {
	t.Helper()
	directory := filepath.Join(dataDirectory, watchLaterDirectory)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, strings.ToUpper(hash))
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modifiedAt, modifiedAt); err != nil {
		t.Fatal(err)
	}
}

func createVideoFile(t *testing.T, name string) string {
	t.Helper()
	return createVideoFileIn(t, t.TempDir(), name)
}

func createVideoFileIn(t *testing.T, directory, name string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// historyXMLFixture models only the archive objects consumed by the decoder;
// each reference is still positional, preserving the real NSKeyedArchive shape.
func historyXMLFixture(entries []fixtureEntry) string {
	var objects strings.Builder
	objects.WriteString("<string>$null</string>")
	for index, entry := range entries {
		base := 1 + index*5
		rawURL := (&url.URL{Scheme: "file", Path: entry.Path}).String()
		fmt.Fprintf(&objects, `<dict><key>IINAPHUrl</key><dict><key>CF$UID</key><integer>%d</integer></dict><key>IINAPHNme</key><dict><key>CF$UID</key><integer>%d</integer></dict><key>IINAPHMpvmd5</key><dict><key>CF$UID</key><integer>%d</integer></dict></dict>`, base+1, base+3, base+4)
		fmt.Fprintf(&objects, `<dict><key>NS.relative</key><dict><key>CF$UID</key><integer>%d</integer></dict></dict>`, base+2)
		fmt.Fprintf(&objects, "<string>%s</string><string>%s</string><string>%s</string>", xmlEscape(rawURL), xmlEscape(filepath.Base(entry.Path)), xmlEscape(strings.ToLower(entry.Hash)))
	}
	return `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>$objects</key><array>` + objects.String() + `</array></dict></plist>`
}

func xmlEscape(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return replacer.Replace(value)
}
