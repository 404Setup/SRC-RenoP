/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package index

import (
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"
)

var OnStorageUpdated func()

const directoryScanQueueSize = 64

func queueDirectoryScan(requests chan<- string, scanAll *atomic.Bool, path string) {
	select {
	case requests <- path:
	default:
		scanAll.Store(true)
	}
}

func runDirectoryScanWorker(basePath string, requests <-chan string, scanAll *atomic.Bool,
	done <-chan struct{}, scan func(string),
) {
	for {
		select {
		case <-done:
			return
		default:
		}
		select {
		case <-done:
			return
		case path := <-requests:
			if scanAll.Swap(false) {
				path = basePath
			drain:
				for {
					select {
					case <-requests:
					default:
						break drain
					}
				}
			}
			scan(path)
		}
	}
}

func scanWatchedDirectory(pathCleaned string, watcher *fsnotify.Watcher, idx *FileIndex, done <-chan struct{}) {
	_ = filepath.WalkDir(pathCleaned, func(path string, d fs.DirEntry, err error) error {
		select {
		case <-done:
			return fs.SkipAll
		default:
		}
		if err != nil {
			return nil
		}
		pathNorm := filepath.ToSlash(filepath.Clean(path))
		if isTemporaryPath(pathNorm) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			idx.InsertDir(pathNorm)
			_ = watcher.Add(pathNorm)
		} else if info, err := d.Info(); err == nil {
			idx.InsertFile(pathNorm, FileInfo{Size: info.Size(), ModTime: info.ModTime().UnixNano()})
		}
		return nil
	})
}

func StartFileWatcher(basePath string, idx *FileIndex) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	baseCleaned := filepath.ToSlash(filepath.Clean(basePath))
	directoryScans := make(chan string, directoryScanQueueSize)
	var scanAll atomic.Bool
	done := make(chan struct{})
	go runDirectoryScanWorker(baseCleaned, directoryScans, &scanAll, done, func(path string) {
		scanWatchedDirectory(path, watcher, idx, done)
	})

	go func() {
		defer close(done)
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				pathStr := event.Name
				if isTemporaryPath(pathStr) {
					continue
				}

				pathCleaned := filepath.ToSlash(filepath.Clean(pathStr))
				if OnStorageUpdated != nil {
					OnStorageUpdated()
				}

				if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) {
					info, err := os.Stat(pathCleaned)
					if err == nil {
						if info.IsDir() {
							idx.InsertDir(pathCleaned)
							if event.Has(fsnotify.Create) {
								queueDirectoryScan(directoryScans, &scanAll, pathCleaned)
							}
						} else {
							parentCleaned := filepath.ToSlash(filepath.Dir(pathCleaned))
							if parentCleaned != baseCleaned {
								idx.InsertFile(pathCleaned, FileInfo{Size: info.Size(), ModTime: info.ModTime().UnixNano()})
							}
						}
					} else if event.Has(fsnotify.Create) {
						parentCleaned := filepath.ToSlash(filepath.Dir(pathCleaned))
						if parentCleaned != baseCleaned {
							idx.InsertFile(pathCleaned)
						}
					}
				} else if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
					idx.RemoveFile(pathCleaned)
					idx.RemoveDir(pathCleaned)
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("watch error:", err)
			}
		}
	}()

	err = filepath.WalkDir(basePath, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			pathNorm := filepath.ToSlash(filepath.Clean(path))
			if isTemporaryPath(pathNorm) {
				return filepath.SkipDir
			}
			_ = watcher.Add(pathNorm)
		}
		return nil
	})
	if err != nil {
		_ = watcher.Close()
		return nil, err
	}

	return watcher, nil
}
