package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type fakeCommandRunner struct {
	outputs map[string]string
	started [][]string
	err     error
}

func (runner *fakeCommandRunner) Output(_ context.Context, _ string, args ...string) ([]byte, error) {
	if runner.err != nil {
		return nil, runner.err
	}
	return []byte(runner.outputs[args[len(args)-1]]), nil
}

func (runner *fakeCommandRunner) Start(name string, args ...string) error {
	runner.started = append(runner.started, append([]string{name}, args...))
	return runner.err
}

// TestLastPlayback verifies the preference-to-domain conversion, including the
// decimal position written by IINA and a path containing spaces.
func TestLastPlayback(t *testing.T) {
	videoPath := filepath.Join(t.TempDir(), "my video.mp4")
	if err := os.WriteFile(videoPath, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeCommandRunner{outputs: map[string]string{
		lastFileKey: videoPath + "\n", lastPositionKey: "211.4666666667\n",
	}}

	record, err := newIINAService(runner).LastPlayback(context.Background())
	if err != nil {
		t.Fatalf("LastPlayback() error = %v", err)
	}
	if record.Name != "my video.mp4" || record.PositionSeconds != 211.4666666667 || !record.Available {
		t.Fatalf("LastPlayback() record = %#v", record)
	}
}

// TestResumeAsksMacOSToOpenIINA locks down argument boundaries: the path stays
// one argument even when it contains whitespace or shell-looking characters.
func TestResumeAsksMacOSToOpenIINA(t *testing.T) {
	videoPath := filepath.Join(t.TempDir(), "video $(touch nope).mp4")
	if err := os.WriteFile(videoPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeCommandRunner{outputs: map[string]string{lastFileKey: videoPath}}

	if _, err := newIINAService(runner).Resume(context.Background()); err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	want := [][]string{{"open", "-a", iinaApplication, videoPath}}
	if !reflect.DeepEqual(runner.started, want) {
		t.Fatalf("started commands = %#v, want %#v", runner.started, want)
	}
}

func TestLastPlaybackWithoutHistory(t *testing.T) {
	runner := &fakeCommandRunner{err: errors.New("preference not found")}
	_, err := newIINAService(runner).LastPlayback(context.Background())
	if !errors.Is(err, errNoPlaybackHistory) {
		t.Fatalf("LastPlayback() error = %v, want errNoPlaybackHistory", err)
	}
}

func TestMissingLocalFileCannotResume(t *testing.T) {
	runner := &fakeCommandRunner{outputs: map[string]string{lastFileKey: "/missing/video.mp4"}}
	record, err := newIINAService(runner).Resume(context.Background())
	if !errors.Is(err, errSourceUnavailable) || record.Available {
		t.Fatalf("Resume() = (%#v, %v), want unavailable source", record, err)
	}
	if len(runner.started) != 0 {
		t.Fatalf("Resume() started an unavailable file: %#v", runner.started)
	}
}
