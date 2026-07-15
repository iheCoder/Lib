package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const (
	iinaBundleID       = "com.colliderli.iina"
	lastFileKey        = "iinaLastPlayedFilePath"
	lastPositionKey    = "iinaLastPlayedFilePosition"
	iinaApplication    = "IINA"
	localFileKind      = "file"
	remoteResourceKind = "url"
)

var (
	errNoPlaybackHistory = errors.New("IINA has no previous playback record")
	errSourceUnavailable = errors.New("the previous video is no longer available")
)

// commandRunner is the only operating-system boundary. Keeping command
// execution behind this interface makes tests deterministic and prevents an
// HTTP handler test from accidentally opening IINA.
type commandRunner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
	Start(name string, args ...string) error
}

type osCommandRunner struct{}

// newCommandRunner centralizes the production adapter choice so application
// wiring does not construct operating-system dependencies ad hoc.
func newCommandRunner() commandRunner { return osCommandRunner{} }

// Output runs short-lived read commands synchronously because the response is
// not meaningful until the current IINA preference value is known.
func (osCommandRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// Start intentionally detaches from the IINA process: the API reports success
// once macOS accepts the open request and must not wait for the player to exit.
func (osCommandRunner) Start(name string, args ...string) error {
	return exec.Command(name, args...).Start()
}

type playbackRecord struct {
	Path            string  `json:"path"`
	Name            string  `json:"name"`
	PositionSeconds float64 `json:"positionSeconds"`
	Kind            string  `json:"kind"`
	Available       bool    `json:"available"`
}

type iinaService struct {
	runner commandRunner
}

// newIINAService requires its side-effect boundary explicitly; there is no
// hidden package-level executor that tests or future callers must replace.
func newIINAService(runner commandRunner) *iinaService {
	return &iinaService{runner: runner}
}

// LastPlayback reads IINA at request time rather than mirroring its history.
// IINA therefore remains the single source of truth even if files are played
// while this helper is stopped.
func (service *iinaService) LastPlayback(ctx context.Context) (playbackRecord, error) {
	path, err := service.readPreference(ctx, lastFileKey)
	if err != nil || path == "" {
		return playbackRecord{}, errNoPlaybackHistory
	}

	position := service.readPosition(ctx)
	return buildPlaybackRecord(path, position), nil
}

// Resume validates the record immediately before opening it. This closes the
// race where an external disk is removed after the page initially loads.
func (service *iinaService) Resume(ctx context.Context) (playbackRecord, error) {
	record, err := service.LastPlayback(ctx)
	if err != nil {
		return playbackRecord{}, err
	}
	if !record.Available {
		return record, errSourceUnavailable
	}

	if err := service.runner.Start("open", "-a", iinaApplication, record.Path); err != nil {
		return record, fmt.Errorf("ask macOS to open IINA: %w", err)
	}
	return record, nil
}

// readPreference uses `defaults` instead of parsing IINA's plist directly.
// This respects macOS preference caching and works across IINA storage-format
// changes as long as its public preference keys remain stable.
func (service *iinaService) readPreference(ctx context.Context, key string) (string, error) {
	output, err := service.runner.Output(ctx, "defaults", "read", iinaBundleID, key)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// readPosition is optional metadata: an unreadable value must not hide a valid
// video. IINA's own watch-later data still controls the exact resume position.
func (service *iinaService) readPosition(ctx context.Context) float64 {
	rawPosition, err := service.readPreference(ctx, lastPositionKey)
	if err != nil {
		return 0
	}
	position, err := strconv.ParseFloat(rawPosition, 64)
	if err != nil || position < 0 {
		return 0
	}
	return position
}

// buildPlaybackRecord explicitly separates URLs from local files. Only local
// files can be checked synchronously without causing network side effects.
func buildPlaybackRecord(path string, position float64) playbackRecord {
	kind := localFileKind
	available := true
	name := localDisplayName(path)

	if parsed, err := url.Parse(path); err == nil && parsed.Scheme != "" {
		kind = remoteResourceKind
		name = remoteDisplayName(parsed)
	} else if _, err := os.Stat(path); err != nil {
		available = false
	}

	return playbackRecord{
		Path: path, Name: name, PositionSeconds: position,
		Kind: kind, Available: available,
	}
}

// localDisplayName avoids importing path/filepath semantics for remote URLs
// and keeps a useful fallback for malformed or root-only paths.
func localDisplayName(path string) string {
	trimmed := strings.TrimRight(path, "/")
	if index := strings.LastIndex(trimmed, "/"); index >= 0 && index+1 < len(trimmed) {
		return trimmed[index+1:]
	}
	return trimmed
}

func remoteDisplayName(resource *url.URL) string {
	// A URL can represent either a file-like resource or a host-level stream.
	// Prefer the final path segment but retain the host as a useful fallback.
	if name := localDisplayName(resource.Path); name != "" {
		return name
	}
	return resource.Host
}
