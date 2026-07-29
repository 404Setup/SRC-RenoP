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
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var S3IndexBuilder func(basePath string, idx *FileIndex)

func isTemporaryPath(pathStr string) bool {
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

func scanSingleDirTree(dirCleaned string, idx *FileIndex) {
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
				idx.InsertDir(fullPath)
				dirsToVisit = append(dirsToVisit, fullPath)
			} else {
				var size int64
				var modTime int64
				if info, err := entry.Info(); err == nil {
					size = info.Size()
					modTime = info.ModTime().UnixNano()
				}
				idx.InsertFile(fullPath, FileInfo{Size: size, ModTime: modTime})
			}
		}
	}
}

func ScanLocalDir(dirPath string, idx *FileIndex, skipRootFiles bool) {
	dirCleaned := toSlashFast(filepath.Clean(dirPath))
	entries, err := os.ReadDir(dirCleaned)
	if err != nil {
		return
	}

	var subdirs []string
	for _, entry := range entries {
		if isTemporaryPath(entry.Name()) {
			continue
		}
		fullPath := dirCleaned + "/" + entry.Name()
		if entry.IsDir() {
			idx.InsertDir(fullPath)
			subdirs = append(subdirs, fullPath)
		} else if !skipRootFiles {
			var size int64
			var modTime int64
			if info, err := entry.Info(); err == nil {
				size = info.Size()
				modTime = info.ModTime().UnixNano()
			}
			idx.InsertFile(fullPath, FileInfo{Size: size, ModTime: modTime})
		}
	}

	if len(subdirs) == 0 {
		return
	}

	var wg sync.WaitGroup
	for _, subdir := range subdirs {
		wg.Add(1)
		sd := subdir
		go func() {
			defer wg.Done()
			scanSingleDirTree(sd, idx)
		}()
	}
	wg.Wait()
}

func BuildIndexSync(basePath string, idx *FileIndex) {
	if S3IndexBuilder != nil {
		S3IndexBuilder(basePath, idx)
		return
	}

	baseCleaned := filepath.ToSlash(filepath.Clean(basePath))
	idx.InsertDir(baseCleaned)
	ScanLocalDir(baseCleaned, idx, true)
}

func replaceIndexFromScan(basePath string, idx *FileIndex) {
	newIndex := NewFileIndexCustom(true)
	BuildIndexSync(basePath, newIndex)

	var toRemoveFiles []string
	idx.Files.Range(func(f string, _ FileInfo) bool {
		if !newIndex.HasFile(f) {
			toRemoveFiles = append(toRemoveFiles, f)
		}
		return true
	})
	for _, k := range toRemoveFiles {
		idx.RemoveFile(k)
	}

	newIndex.Files.Range(func(f string, info FileInfo) bool {
		oldInfo, exists := idx.GetFileInfo(f)
		if !exists || oldInfo != info {
			idx.InsertFile(f, info)
		}
		return true
	})

	var toRemoveDirs []string
	idx.Dirs.Range(func(d string, _ bool) bool {
		if !newIndex.HasDir(d) {
			toRemoveDirs = append(toRemoveDirs, d)
		}
		return true
	})
	for _, k := range toRemoveDirs {
		idx.RemoveDir(k)
	}

	newIndex.Dirs.Range(func(d string, _ bool) bool {
		if !idx.HasDir(d) {
			idx.InsertDir(d)
		}
		return true
	})
}

func RebuildIndexAsync(basePath string, idx *FileIndex) {
	go replaceIndexFromScan(basePath, idx)
}

func RebuildIndexDiff(basePath string, idx *FileIndex) {
	go func() {
		newIndex := NewFileIndexCustom(true)
		BuildIndexSync(basePath, newIndex)

		var toRemoveFiles []string
		idx.Files.Range(func(path string, _ FileInfo) bool {
			if !newIndex.HasFile(path) {
				toRemoveFiles = append(toRemoveFiles, path)
			}
			return true
		})
		for _, f := range toRemoveFiles {
			idx.RemoveFile(f)
		}

		newIndex.Files.Range(func(path string, info FileInfo) bool {
			oldInfo, exists := idx.GetFileInfo(path)
			if !exists || oldInfo != info {
				idx.InsertFile(path, info)
			}
			return true
		})

		var toRemoveDirs []string
		idx.Dirs.Range(func(path string, _ bool) bool {
			if !newIndex.HasDir(path) {
				toRemoveDirs = append(toRemoveDirs, path)
			}
			return true
		})
		for _, d := range toRemoveDirs {
			idx.RemoveDir(d)
		}

		newIndex.Dirs.Range(func(path string, _ bool) bool {
			if !idx.HasDir(path) {
				idx.InsertDir(path)
			}
			return true
		})

		idx.IsDirty.Store(true)
	}()
}
