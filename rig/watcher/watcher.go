package watcher

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/gobwas/glob"
)

// Start a watcher with the provided options.
func Start(options ...Option) (Interface, error) {
	wr := &watcher{}
	for _, option := range options {
		err := option(wr)
		if err != nil {
			return nil, err
		}
	}
	err := wr.start()
	if err != nil {
		return nil, err
	}
	return wr, nil
}

// An Option is a function that can manipulate a watcher during construction
type Option func(*watcher) error

// Include specifies one or more file patterns to include in the watch.
// If no patterns are specified, all files not starting with a dot are included.
func Include(patterns ...string) Option {
	return func(wr *watcher) (err error) {
		wr.includes, err = appendPatterns(wr.includes, patterns...)
		return
	}
}

// Exclude specifies one or more file patterns to exclude from the watch.
// If no patterns are specified, only files starting with a dot are excluded.
// If a file matches both an include and an exclude pattern, it is excluded.
func Exclude(patterns ...string) Option {
	return func(wr *watcher) (err error) {
		wr.excludes, err = appendPatterns(wr.excludes, patterns...)
		return
	}
}

func appendPatterns(seq []glob.Glob, patterns ...string) ([]glob.Glob, error) {
	for _, pattern := range patterns {
		rx, err := glob.Compile(pattern, filepath.Separator)
		if err != nil {
			return nil, fmt.Errorf(`%w in %q`, err, pattern)
		}
		seq = append(seq, rx)
	}
	return seq, nil
}

// Directory specifies one or more directories to watch recursively.
// If no directories are specified, the current working directory is watched.
func Directory(paths ...string) Option {
	return func(wr *watcher) error {
		wr.directories = append(wr.directories, paths...)
		return nil
	}
}

// Interface describes the watcher interface
type Interface interface {
	Alert() <-chan struct{}
	Shutdown()
}

type watcher struct {
	includes    []glob.Glob
	excludes    []glob.Glob
	directories []string

	fsnotify   *fsnotify.Watcher
	alertCh    chan struct{} // sent when the watcher has observed a change
	shutdownCh chan struct{} // sent when the watcher should shut down
	doneCh     chan struct{} // closed when the watcher is done
}

func (wr *watcher) start() (err error) {
	wr.fsnotify, err = fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if len(wr.directories) == 0 {
		wr.directories = []string{`.`}
	}
	if len(wr.excludes) == 0 {
		wr.excludes = []glob.Glob{glob.MustCompile(`.*`, filepath.Separator)}
	}
	for _, dir := range wr.directories {
		err := filepath.WalkDir(dir, func(path string, info fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return wr.fsnotify.Add(path)
			}
			return nil
		})
		if err != nil {
			wr.fsnotify.Close()
			return err
		}
	}
	wr.alertCh = make(chan struct{})
	wr.shutdownCh = make(chan struct{})
	wr.doneCh = make(chan struct{})
	go wr.process()
	return nil
}

func (wr *watcher) Alert() <-chan struct{} {
	return wr.alertCh
}

func (wr *watcher) Shutdown() {
	select {
	case wr.shutdownCh <- struct{}{}:
	case <-wr.doneCh:
	}
}

func (wr *watcher) process() {
	for {
		select {
		case <-wr.shutdownCh:
			close(wr.doneCh)
			return
		case event := <-wr.fsnotify.Events:
			wr.processNotification(event)
		}
	}
}

func (wr *watcher) processNotification(event fsnotify.Event) {
	if event.Has(fsnotify.Create) {
		info, err := os.Stat(event.Name)
		if err != nil {
			return
		}
		if info.IsDir() {
			wr.fsnotify.Add(event.Name)
			return // creating a new directory should not issue an alert, but we should watch it
		}
	}

	if event.Has(fsnotify.Write) {
		wr.issueAlert(event.Name)
	} else if event.Has(fsnotify.Remove) {
		_ = wr.fsnotify.Remove(event.Name)
		wr.issueAlert(event.Name)
	} else if event.Has(fsnotify.Rename) {
		wr.issueAlert(event.Name)
	}
}

func (wr *watcher) issueAlert(name string) {
	if !wr.shouldInclude(name) {
		return
	}
	select {
	case <-wr.shutdownCh:
	case wr.alertCh <- struct{}{}:
	default:
	}
}

func (wr *watcher) shouldInclude(name string) bool {
	included := len(wr.includes) == 0
	for _, rx := range wr.includes {
		if rx.Match(name) {
			included = true
			break
		}
	}
	if !included {
		return false
	}
	for _, rx := range wr.excludes {
		if rx.Match(name) {
			return false
		}
	}
	return true
}

//TODO: BUG: if a directory is renamed, the watcher may not watch the new name.
