package config

import (
	"github.com/fsnotify/fsnotify"
)

type configLoader struct {
	// The fsnotify watcher
	watcher *fsnotify.Watcher
	// version is the current version of the configuration
	version int64
}

func NewConfigLoader(path string) (*configLoader, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	cl := &configLoader{
		watcher: watcher,
	}

	cl.watcher.Add(path)

	go cl.handleEvents()

	return cl, nil
}

func (cl *configLoader) Close() error {
	return cl.watcher.Close()
}

func (cl *configLoader) handleEvents() {
	for {
		select {
		case event, ok := <-cl.watcher.Events:
			if !ok {
				return
			}

			if event.Op&fsnotify.Write == fsnotify.Write {
				cl.version++
			}
		}
	}
}
