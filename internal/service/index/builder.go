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
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// S3IndexBuilder replaces local directory scans when the active storage backend is S3.
var S3IndexBuilder func(basePath string, idx *FileIndex) error

const maxLocalScanWorkers = 8

func isTemporaryPath(pathStr string) bool {
	pathSlash := filepath.ToSlash(pathStr)
	if pathSlash == ".renop.tmp.gpg" || strings.Contains(pathSlash, "/.renop.tmp.gpg/") || strings.HasSuffix(pathSlash, "/.renop.tmp.gpg") {
		return true
	}
	name := filepath.Base(pathStr)
	if name == "" {
		return false
	}
	if strings.HasSuffix(name, ".tmp") || strings.Contains(name, ".tmp.") {
		return true
	}
	if strings.Contains(name, ".chunk.") {
		return true
	}
	return false
}

func entryFileInfo(entry os.DirEntry) FileInfo {
	var size int64
	var modTime int64
	if info, err := entry.Info(); err == nil {
		size = info.Size()
		modTime = info.ModTime().UnixNano()
	}
	return FileInfo{Size: size, ModTime: modTime}
}

type scanSink interface {
	addFile(path string, info FileInfo)
	addDir(path string)
}

type fileIndexScanSink struct {
	index *FileIndex
}

func (sink fileIndexScanSink) addFile(path string, info FileInfo) {
	sink.index.InsertFile(path, info)
}

func (sink fileIndexScanSink) addDir(path string) {
	sink.index.InsertDir(path)
}

func scanSingleDirTree(dirCleaned string, sink scanSink) {
	dirsToVisit := []string{dirCleaned}
	for len(dirsToVisit) > 0 {
		dir := dirsToVisit[len(dirsToVisit)-1]
		dirsToVisit = dirsToVisit[:len(dirsToVisit)-1]

		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if isTemporaryPath(entry.Name()) {
				continue
			}
			fullPath := dir + "/" + entry.Name()
			if entry.IsDir() {
				sink.addDir(fullPath)
				dirsToVisit = append(dirsToVisit, fullPath)
			} else {
				sink.addFile(fullPath, entryFileInfo(entry))
			}
		}
	}
}

func scanLocalDir(dirPath string, sink scanSink, skipRootFiles bool) {
	dirCleaned := toSlashFast(filepath.Clean(dirPath))
	entries, err := os.ReadDir(dirCleaned)
	if err != nil {
		return
	}

	var wg sync.WaitGroup
	slots := make(chan struct{}, maxLocalScanWorkers)
	for _, entry := range entries {
		if isTemporaryPath(entry.Name()) {
			continue
		}
		fullPath := dirCleaned + "/" + entry.Name()
		if entry.IsDir() {
			sink.addDir(fullPath)
			slots <- struct{}{}
			wg.Add(1)
			go func(path string) {
				defer wg.Done()
				defer func() { <-slots }()
				scanSingleDirTree(path, sink)
			}(fullPath)
		} else if !skipRootFiles {
			sink.addFile(fullPath, entryFileInfo(entry))
		}
	}
	wg.Wait()
}

// ScanLocalDir adds a local directory tree to idx.
func ScanLocalDir(dirPath string, idx *FileIndex, skipRootFiles bool) {
	if idx == nil {
		return
	}
	scanLocalDir(dirPath, fileIndexScanSink{index: idx}, skipRootFiles)
}

// BuildIndexSync scans one storage root before returning.
func BuildIndexSync(basePath string, idx *FileIndex) error {
	if S3IndexBuilder != nil {
		return S3IndexBuilder(basePath, idx)
	}

	baseCleaned := filepath.ToSlash(filepath.Clean(basePath))
	idx.InsertDir(baseCleaned)
	ScanLocalDir(baseCleaned, idx, true)
	return nil
}

type scanMaps struct {
	files map[string]FileInfo
	dirs  map[string]struct{}
	mu    sync.Mutex
}

func (s *scanMaps) addFile(path string, info FileInfo) {
	s.mu.Lock()
	s.files[path] = info
	s.mu.Unlock()
}

func (s *scanMaps) addDir(path string) {
	s.mu.Lock()
	s.dirs[path] = struct{}{}
	s.mu.Unlock()
}

func buildScanMaps(basePath string) *scanMaps {
	out := &scanMaps{
		files: make(map[string]FileInfo),
		dirs:  make(map[string]struct{}),
	}
	baseCleaned := filepath.ToSlash(filepath.Clean(basePath))
	out.dirs[baseCleaned] = struct{}{}
	scanLocalDir(baseCleaned, out, true)
	return out
}

func applyScanMaps(idx *FileIndex, scanned *scanMaps) {
	var toRemoveFiles []string
	idx.Files.Range(func(f string, _ FileInfo) bool {
		if _, ok := scanned.files[f]; !ok {
			toRemoveFiles = append(toRemoveFiles, f)
		}
		return true
	})
	for _, k := range toRemoveFiles {
		idx.RemoveFile(k)
	}

	for f, info := range scanned.files {
		oldInfo, exists := idx.GetFileInfo(f)
		if !exists || oldInfo != info {
			idx.InsertFile(f, info)
		}
	}

	var toRemoveDirs []string
	idx.Dirs.Range(func(d string, _ bool) bool {
		if _, ok := scanned.dirs[d]; !ok {
			toRemoveDirs = append(toRemoveDirs, d)
		}
		return true
	})
	for _, k := range toRemoveDirs {
		idx.RemoveDir(k)
	}

	for d := range scanned.dirs {
		if !idx.HasDir(d) {
			idx.InsertDir(d)
		}
	}
}

// applyFileIndexScan merges a temporary FileIndex (e.g. S3 listing) into the live index.
// The temporary index is not retained after the call returns.
func applyFileIndexScan(idx *FileIndex, scanned *FileIndex) {
	var toRemoveFiles []string
	idx.Files.Range(func(f string, _ FileInfo) bool {
		if !scanned.HasFile(f) {
			toRemoveFiles = append(toRemoveFiles, f)
		}
		return true
	})
	for _, k := range toRemoveFiles {
		idx.RemoveFile(k)
	}

	scanned.Files.Range(func(f string, info FileInfo) bool {
		oldInfo, exists := idx.GetFileInfo(f)
		if !exists || oldInfo != info {
			idx.InsertFile(f, info)
		}
		return true
	})

	var toRemoveDirs []string
	idx.Dirs.Range(func(d string, _ bool) bool {
		if !scanned.HasDir(d) {
			toRemoveDirs = append(toRemoveDirs, d)
		}
		return true
	})
	for _, k := range toRemoveDirs {
		idx.RemoveDir(k)
	}

	scanned.Dirs.Range(func(d string, _ bool) bool {
		if !idx.HasDir(d) {
			idx.InsertDir(d)
		}
		return true
	})
}

func replaceIndexFromScan(basePath string, idx *FileIndex) error {
	if S3IndexBuilder != nil {
		newIndex := NewFileIndexCustom(true)
		if err := S3IndexBuilder(basePath, newIndex); err != nil {
			return err
		}
		applyFileIndexScan(idx, newIndex)
		return nil
	}

	scanned := buildScanMaps(basePath)
	applyScanMaps(idx, scanned)
	return nil
}

type indexRebuildRequest struct {
	basePath string
	dirty    bool
}

func runIndexRebuilds(idx *FileIndex) {
	for {
		idx.rebuildMu.Lock()
		request := idx.rebuildNext
		idx.rebuildNext = nil
		if request == nil {
			idx.rebuildRunning = false
			idx.rebuildMu.Unlock()
			return
		}
		idx.rebuildMu.Unlock()

		if err := replaceIndexFromScan(request.basePath, idx); err != nil {
			log.Printf("Failed to rebuild artifact index: %v", err)
		} else if request.dirty {
			idx.IsDirty.Store(true)
		}
	}
}

func queueIndexRebuild(basePath string, idx *FileIndex, dirty bool) {
	if idx == nil {
		return
	}
	idx.rebuildMu.Lock()
	if idx.rebuildNext == nil {
		idx.rebuildNext = &indexRebuildRequest{basePath: basePath, dirty: dirty}
	} else {
		idx.rebuildNext.basePath = basePath
		idx.rebuildNext.dirty = idx.rebuildNext.dirty || dirty
	}
	if idx.rebuildRunning {
		idx.rebuildMu.Unlock()
		return
	}
	idx.rebuildRunning = true
	idx.rebuildMu.Unlock()
	go runIndexRebuilds(idx)
}

// RebuildIndexAsync queues a coalesced full index rebuild.
func RebuildIndexAsync(basePath string, idx *FileIndex) {
	queueIndexRebuild(basePath, idx, false)
}

// RebuildIndexDiff queues a coalesced rebuild and marks the resulting index dirty.
func RebuildIndexDiff(basePath string, idx *FileIndex) {
	queueIndexRebuild(basePath, idx, true)
}
