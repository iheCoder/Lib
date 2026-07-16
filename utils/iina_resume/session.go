package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	historyFilename     = "history.plist"
	watchLaterDirectory = "watch_later"
	shutdownBatchWindow = 3 * time.Second
)

type historyRecord struct {
	Path string
	Name string
}

type checkpoint struct {
	Hash       string
	Path       string
	ModifiedAt time.Time
}

type sessionStore struct {
	runner        commandRunner
	dataDirectory string
}

// newSessionStore isolates knowledge of IINA's on-disk session artifacts from
// HTTP and launch behavior.
func newSessionStore(runner commandRunner, dataDirectory string) *sessionStore {
	return &sessionStore{runner: runner, dataDirectory: dataDirectory}
}

// Recent reconstructs shutdown batches from checkpoint modification times.
// IINA closes all PlayerCore instances together, causing their meaningful
// watch-later files to be written within one small time window.
func (store *sessionStore) Recent(ctx context.Context, limit int) ([]playbackSession, error) {
	history, err := store.readHistory(ctx)
	if err != nil {
		return nil, err
	}
	checkpoints, err := store.readCheckpoints(history)
	if err != nil {
		return nil, err
	}
	return groupCheckpoints(checkpoints, history, limit), nil
}

// readHistory asks plutil to normalize the binary keyed archive to XML, then
// resolves IINA's MD5-to-media references using the local archive decoder.
func (store *sessionStore) readHistory(ctx context.Context) (map[string]historyRecord, error) {
	path := filepath.Join(store.dataDirectory, historyFilename)
	data, err := store.runner.Output(ctx, "plutil", "-convert", "xml1", "-o", "-", path)
	if err != nil {
		return nil, fmt.Errorf("convert IINA playback history: %w", err)
	}
	return decodePlaybackHistory(data)
}

// readCheckpoints keeps only hashes present in IINA's playback history. mpv
// also creates `# redirect entry` marker files; those are not player sessions
// and deliberately have no matching history record.
func (store *sessionStore) readCheckpoints(history map[string]historyRecord) ([]checkpoint, error) {
	directory := filepath.Join(store.dataDirectory, watchLaterDirectory)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read IINA watch-later directory: %w", err)
	}

	checkpoints := make([]checkpoint, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !hasHistoryRecord(history, entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		checkpoints = append(checkpoints, checkpoint{
			Hash: strings.ToUpper(entry.Name()), Path: filepath.Join(directory, entry.Name()), ModifiedAt: info.ModTime(),
		})
	}
	sort.Slice(checkpoints, func(i, j int) bool { return checkpoints[i].ModifiedAt.After(checkpoints[j].ModifiedAt) })
	return checkpoints, nil
}

// groupCheckpoints starts a new session when adjacent writes exceed the batch
// window. The ID is derived from the newest checkpoint and contains no path.
func groupCheckpoints(items []checkpoint, history map[string]historyRecord, limit int) []playbackSession {
	if len(items) == 0 || limit <= 0 {
		return nil
	}
	groups := splitCheckpointGroups(items, limit)
	sessions := make([]playbackSession, 0, len(groups))
	for _, group := range groups {
		newest := group[0].ModifiedAt
		playbacks := make([]playbackRecord, 0, len(group))
		for _, item := range group {
			record := history[item.Hash]
			playback := buildPlaybackRecord(record.Path, readCheckpointPosition(item.Path))
			if record.Name != "" {
				playback.Name = record.Name
			}
			playbacks = append(playbacks, playback)
		}
		sessions = append(sessions, buildSession(sessionID(newest), newest.Format(time.RFC3339), playbacks))
	}
	return sessions
}

// splitCheckpointGroups compares adjacent writes rather than every item to the
// first write, accommodating a shutdown that serially closes many windows.
func splitCheckpointGroups(items []checkpoint, limit int) [][]checkpoint {
	groups := make([][]checkpoint, 0, limit)
	start := 0
	for index := 1; index <= len(items); index++ {
		atEnd := index == len(items)
		batchEnded := !atEnd && items[index-1].ModifiedAt.Sub(items[index].ModifiedAt) > shutdownBatchWindow
		if !atEnd && !batchEnded {
			continue
		}
		groups = append(groups, items[start:index])
		start = index
		if len(groups) == limit {
			break
		}
	}
	return groups
}

// readCheckpointPosition extracts mpv's `start` option. Other watch-later
// settings are intentionally ignored because IINA owns their restoration.
func readCheckpointPosition(path string) float64 {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, found := strings.Cut(scanner.Text(), "=")
		if !found || key != "start" {
			continue
		}
		position, err := strconv.ParseFloat(value, 64)
		if err == nil && position >= 0 {
			return position
		}
	}
	return 0
}

func hasHistoryRecord(history map[string]historyRecord, hash string) bool {
	_, exists := history[strings.ToUpper(hash)]
	return exists
}

func sessionID(modifiedAt time.Time) string {
	return strconv.FormatInt(modifiedAt.UnixNano(), 36)
}
