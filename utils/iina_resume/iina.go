package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	iinaBundleID        = "com.colliderli.iina"
	lastFileKey         = "iinaLastPlayedFilePath"
	lastPositionKey     = "iinaLastPlayedFilePosition"
	iinaCLIPath         = "/Applications/IINA.app/Contents/MacOS/iina-cli"
	localFileKind       = "file"
	remoteResourceKind  = "url"
	defaultSessionLimit = 10
)

var (
	errNoPlaybackHistory = errors.New("IINA has no previous playback record")
	errSessionNotFound   = errors.New("the requested IINA session no longer exists")
	errSourceUnavailable = errors.New("the previous videos are no longer available")
)

// commandRunner is the only command-execution boundary. Tests can inspect
// launch arguments without accidentally opening IINA or reading real defaults.
type commandRunner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
	Start(name string, args ...string) error
}

type osCommandRunner struct{}

// newCommandRunner centralizes the production adapter choice so application
// wiring does not construct operating-system dependencies ad hoc.
func newCommandRunner() commandRunner { return osCommandRunner{} }

// Output blocks only for short local reads whose result is required by the
// current API response.
func (osCommandRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// Start detaches from iina-cli: the helper confirms that macOS accepted the
// launch request but never waits for all restored player windows to close.
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

type playbackSession struct {
	ID             string           `json:"id"`
	ClosedAt       string           `json:"closedAt,omitempty"`
	Playbacks      []playbackRecord `json:"playbacks"`
	AvailableCount int              `json:"availableCount"`
}

type iinaService struct {
	runner   commandRunner
	sessions *sessionStore
}

// newIINAService makes both side-effect boundaries explicit. The data directory
// is injectable because test fixtures must not depend on the user's IINA state.
func newIINAService(runner commandRunner, dataDirectory string) *iinaService {
	return &iinaService{
		runner: runner, sessions: newSessionStore(runner, dataDirectory),
	}
}

// RecentSessions first reconstructs shutdown batches from watch-later files.
// The legacy preference remains a fallback for disabled or missing IINA history.
func (service *iinaService) RecentSessions(ctx context.Context) ([]playbackSession, error) {
	sessions, err := service.sessions.Recent(ctx, defaultSessionLimit)
	if err == nil && len(sessions) > 0 {
		return sessions, nil
	}

	record, fallbackErr := service.lastPlayback(ctx)
	if fallbackErr != nil {
		return nil, fallbackErr
	}
	return []playbackSession{buildSession("latest", "", []playbackRecord{record})}, nil
}

// ResumeSession re-reads the catalog and accepts only an opaque session ID.
// Paths never cross the HTTP trust boundary, preventing arbitrary file launches.
func (service *iinaService) ResumeSession(ctx context.Context, sessionID string) (playbackSession, error) {
	sessions, err := service.RecentSessions(ctx)
	if err != nil {
		return playbackSession{}, err
	}
	session, found := findSession(sessions, sessionID)
	if !found {
		return playbackSession{}, errSessionNotFound
	}
	return session, service.openAvailable(session)
}

// openAvailable restores every reachable item in separate IINA windows. Missing
// external-disk files do not block the rest of a recoverable session.
func (service *iinaService) openAvailable(session playbackSession) error {
	arguments := []string{"--no-stdin", "--separate-windows"}
	for _, playback := range session.Playbacks {
		if playback.Available {
			arguments = append(arguments, playback.Path)
		}
	}
	if len(arguments) == 2 {
		return errSourceUnavailable
	}
	if err := service.runner.Start(iinaCLIPath, arguments...); err != nil {
		return fmt.Errorf("ask IINA CLI to restore session: %w", err)
	}
	return nil
}

// lastPlayback preserves compatibility when playback history recording is off.
// It cannot represent multiple windows and is therefore intentionally fallback-only.
func (service *iinaService) lastPlayback(ctx context.Context) (playbackRecord, error) {
	path, err := service.readPreference(ctx, lastFileKey)
	if err != nil || path == "" {
		return playbackRecord{}, errNoPlaybackHistory
	}
	return buildPlaybackRecord(path, service.readPosition(ctx)), nil
}

// readPreference delegates plist caching semantics to macOS `defaults` rather
// than reading the preferences file behind cfprefsd's back.
func (service *iinaService) readPreference(ctx context.Context, key string) (string, error) {
	output, err := service.runner.Output(ctx, "defaults", "read", iinaBundleID, key)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// readPosition is optional metadata: a malformed value must not hide a valid
// fallback video because IINA's watch-later file controls the actual resume.
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

// buildPlaybackRecord separates URLs from local files because only local paths
// can be availability-checked without adding network traffic to a page read.
func buildPlaybackRecord(path string, position float64) playbackRecord {
	kind, available, name := localFileKind, true, filepath.Base(path)
	if parsed, err := url.Parse(path); err == nil && parsed.Scheme != "" {
		kind, name = remoteResourceKind, remoteDisplayName(parsed)
	} else if _, err := os.Stat(path); err != nil {
		available = false
	}
	return playbackRecord{
		Path: path, Name: name, PositionSeconds: position,
		Kind: kind, Available: available,
	}
}

// buildSession derives availability once so both the UI and launch decision use
// the same count. Alphabetical ordering keeps reconstructed sessions stable.
func buildSession(id, closedAt string, playbacks []playbackRecord) playbackSession {
	sort.Slice(playbacks, func(i, j int) bool { return playbacks[i].Name < playbacks[j].Name })
	availableCount := 0
	for _, playback := range playbacks {
		if playback.Available {
			availableCount++
		}
	}
	return playbackSession{
		ID: id, ClosedAt: closedAt, Playbacks: playbacks, AvailableCount: availableCount,
	}
}

// findSession performs an exact opaque-ID lookup; an empty or stale ID never
// falls back to a different session that the user did not choose.
func findSession(sessions []playbackSession, id string) (playbackSession, bool) {
	for _, session := range sessions {
		if session.ID == id {
			return session, true
		}
	}
	return playbackSession{}, false
}

// remoteDisplayName prefers a file-like path segment while retaining the host
// as a useful label for host-level streams.
func remoteDisplayName(resource *url.URL) string {
	if name := filepath.Base(resource.Path); name != "." && name != "/" {
		return name
	}
	return resource.Host
}

// defaultIINADataDirectory resolves the current user's sandbox-free application
// support location used by the standard IINA distribution.
func defaultIINADataDirectory() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "Application Support", iinaBundleID)
}
