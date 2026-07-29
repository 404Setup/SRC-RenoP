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
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/goccy/go-json"

	"github.com/llxisdsh/pb"
)

func toSlashFast(pathStr string) string {
	if strings.IndexByte(pathStr, '\\') == -1 {
		return pathStr
	}
	return filepath.ToSlash(pathStr)
}

func cleanPathFast(pathStr string) string {
	return path.Clean(toSlashFast(pathStr))
}

type FileInfo struct {
	Size    int64 `json:"size"`
	ModTime int64 `json:"mod_time"`
}

func (idx *FileIndex) ReadJSONFrom(r io.Reader) error {
	decoder := json.NewDecoder(r)
	opening, err := decoder.Token()
	if err != nil {
		return err
	}
	if delim, ok := opening.(json.Delim); !ok || delim != '{' {
		return errors.New("index root must be an object")
	}

	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := keyToken.(string)
		if !ok {
			return errors.New("index field name must be a string")
		}
		switch key {
		case "files":
			if err := readFilesFromJSON(decoder, idx); err != nil {
				return err
			}
		case "dirs":
			if err := readDirsFromJSON(decoder, idx); err != nil {
				return err
			}
		case "not_found":
			if err := readNotFoundFromJSON(decoder, idx); err != nil {
				return err
			}
		default:
			var ignored json.RawMessage
			if err := decoder.Decode(&ignored); err != nil {
				return err
			}
		}
	}
	_, err = decoder.Token()
	return err
}

func readFilesFromJSON(decoder *json.Decoder, idx *FileIndex) error {
	opening, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := opening.(json.Delim)
	if !ok || (delim != '{' && delim != '[') {
		return errors.New("index files must be an object or array")
	}
	if delim == '{' {
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			filePath, ok := keyToken.(string)
			if !ok {
				return errors.New("index file path must be a string")
			}
			var info FileInfo
			if err := decoder.Decode(&info); err != nil {
				return err
			}
			idx.InsertFile(filePath, info)
		}
		_, err = decoder.Token()
		return err
	}

	for decoder.More() {
		var filePath string
		if err := decoder.Decode(&filePath); err != nil {
			return err
		}
		var info FileInfo
		if stat, statErr := os.Stat(filepath.FromSlash(filePath)); statErr == nil {
			info = FileInfo{Size: stat.Size(), ModTime: stat.ModTime().UnixNano()}
		}
		idx.InsertFile(filePath, info)
	}
	_, err = decoder.Token()
	return err
}

func readDirsFromJSON(decoder *json.Decoder, idx *FileIndex) error {
	opening, err := decoder.Token()
	if err != nil {
		return err
	}
	if delim, ok := opening.(json.Delim); !ok || delim != '[' {
		return errors.New("index dirs must be an array")
	}
	for decoder.More() {
		var dir string
		if err := decoder.Decode(&dir); err != nil {
			return err
		}
		idx.InsertDir(dir)
	}
	_, err = decoder.Token()
	return err
}

func readNotFoundFromJSON(decoder *json.Decoder, idx *FileIndex) error {
	opening, err := decoder.Token()
	if err != nil {
		return err
	}
	if delim, ok := opening.(json.Delim); !ok || delim != '{' {
		return errors.New("index not_found must be an object")
	}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return err
		}
		filePath, ok := keyToken.(string)
		if !ok {
			return errors.New("negative cache path must be a string")
		}
		var expireAt int64
		if err := decoder.Decode(&expireAt); err != nil {
			return err
		}
		idx.InsertNotFound(filePath, expireAt)
	}
	_, err = decoder.Token()
	return err
}

type FileIndex struct {
	Files         pb.MapOf[string, FileInfo]              `json:"-"`
	Dirs          pb.MapOf[string, bool]                  `json:"-"`
	Children      map[string][]string                     `json:"-"`
	ChildrenMutex sync.RWMutex                            `json:"-"`
	FilesCount    atomic.Uint64                           `json:"-"`
	DirsCount     atomic.Uint64                           `json:"-"`
	NotFound      atomic.Pointer[pb.MapOf[string, int64]] `json:"-"`
	NotFoundCount atomic.Uint64                           `json:"-"`
	IsDirty       atomic.Bool                             `json:"-"`

	isSync       bool                        `json:"-"`
	OpChan       chan IndexOp                `json:"-"`
	SnapChan     chan chan FileIndexSnapshot `json:"-"`
	metadataLock sync.Mutex                  `json:"-"`
}

var (
	pathInternPool pb.MapOf[string, string]
	pathInternSize atomic.Int64
)

func internString(s string) string {
	if s == "" {
		return ""
	}
	if val, ok := pathInternPool.Load(s); ok {
		return val
	}
	if pathInternSize.Load() >= 50000 {
		var evicted int64
		pathInternPool.Range(func(k string, _ string) bool {
			if _, loaded := pathInternPool.LoadAndDelete(k); loaded {
				evicted++
			}
			return evicted < 5000
		})
		if evicted > 0 {
			pathInternSize.Add(-evicted)
		}
	}
	cloned := strings.Clone(s)
	actual, loaded := pathInternPool.LoadOrStore(cloned, cloned)
	if !loaded {
		pathInternSize.Add(1)
		return cloned
	}
	return actual
}

func splitPathParentBase(filePath string) (string, string) {
	idx := strings.LastIndexByte(filePath, '/')
	if idx <= 0 {
		if idx == 0 {
			return "/", filePath[1:]
		}
		return "", filePath
	}
	return filePath[:idx], filePath[idx+1:]
}

func (idx *FileIndex) addChild(filePath string) {
	parent, base := splitPathParentBase(filePath)
	if parent == filePath || parent == "." || parent == "/" || parent == "" {
		return
	}

	parentInterned := internString(parent)
	baseInterned := internString(base)

	idx.ChildrenMutex.RLock()
	if idx.Children != nil {
		if list, ok := idx.Children[parentInterned]; ok {
			if slices.Contains(list, baseInterned) {
				idx.ChildrenMutex.RUnlock()
				return
			}
		}
	}
	idx.ChildrenMutex.RUnlock()

	idx.ChildrenMutex.Lock()
	defer idx.ChildrenMutex.Unlock()
	if idx.Children == nil {
		idx.Children = make(map[string][]string)
	}
	list := idx.Children[parentInterned]
	if slices.Contains(list, baseInterned) {
		return
	}
	idx.Children[parentInterned] = append(list, baseInterned)
}

func (idx *FileIndex) removeChild(filePath string) {
	parent, base := splitPathParentBase(filePath)

	idx.ChildrenMutex.Lock()
	defer idx.ChildrenMutex.Unlock()
	if idx.Children == nil {
		return
	}
	if list, ok := idx.Children[parent]; ok {
		for i, item := range list {
			if item == base {
				list = append(list[:i], list[i+1:]...)
				break
			}
		}
		if len(list) == 0 {
			delete(idx.Children, parent)
		} else {
			idx.Children[parent] = list
		}
	}
	delete(idx.Children, filePath)
}

type IndexOpType int

const (
	OpInsertFile IndexOpType = iota
	OpInsertDir
	OpRemoveFile
	OpRemoveDir
	OpInsertNotFound
	OpPruneNotFound
	OpUpdateMetadata
)

type IndexOp struct {
	Type     IndexOpType
	Path     string
	Info     FileInfo
	ExpireAt int64
	Callback func() error
	ErrChan  chan error
}

func NewFileIndex() *FileIndex {
	return NewFileIndexCustom(true)
}

func NewFileIndexCustom(isSync bool) *FileIndex {
	idx := &FileIndex{
		isSync:   isSync,
		OpChan:   make(chan IndexOp, 100),
		SnapChan: make(chan chan FileIndexSnapshot),
	}
	idx.FilesCount.Store(0)
	idx.DirsCount.Store(0)
	idx.NotFoundCount.Store(0)
	idx.IsDirty.Store(false)

	initialNotFound := pb.NewMapOf[string, int64]()
	idx.NotFound.Store(initialNotFound)

	if !isSync {
		go idx.consumerLoop()
	}

	return idx
}

func (idx *FileIndex) consumerLoop() {
	for {
		select {
		case op := <-idx.OpChan:
			pathSlash := filepath.ToSlash(op.Path)
			switch op.Type {
			case OpInsertFile:
				if _, loaded := idx.Files.LoadOrStore(pathSlash, op.Info); !loaded {
					idx.FilesCount.Add(1)
					idx.IsDirty.Store(true)
					idx.addChild(pathSlash)
				} else {
					idx.Files.Store(pathSlash, op.Info)
					idx.addChild(pathSlash)
				}
				idx.clearNotFound(pathSlash)
			case OpInsertDir:
				if _, loaded := idx.Dirs.LoadOrStore(pathSlash, true); !loaded {
					idx.DirsCount.Add(1)
					idx.IsDirty.Store(true)
					idx.addChild(pathSlash)
				}
				idx.clearNotFound(pathSlash)
			case OpRemoveFile:
				if _, loaded := idx.Files.LoadAndDelete(pathSlash); loaded {
					idx.FilesCount.Add(^uint64(0))
					idx.IsDirty.Store(true)
					idx.removeChild(pathSlash)
				}
				idx.clearNotFound(pathSlash)
			case OpRemoveDir:
				// Purge descendants first while Children[path] still lists them.
				idx.removeDescendants(pathSlash)
				if _, loaded := idx.Dirs.LoadAndDelete(pathSlash); loaded {
					idx.DirsCount.Add(^uint64(0))
					idx.IsDirty.Store(true)
				}
				idx.removeChild(pathSlash)
				idx.clearNotFound(pathSlash)
			case OpPruneNotFound:
				currentTime := time.Now().Unix()
				removedCount := 0
				currentNotFound := idx.NotFound.Load()
				if currentNotFound != nil {
					currentNotFound.Range(func(k string, v int64) bool {
						if v <= currentTime {
							currentNotFound.Delete(k)
							removedCount++
						}
						return true
					})
					if removedCount > 0 {
						idx.NotFoundCount.Add(^uint64(removedCount - 1))
					}
				}
			case OpInsertNotFound:
				currentNotFound := idx.NotFound.Load()
				if idx.NotFoundCount.Load() >= 10000 {
					newNotFound := pb.NewMapOf[string, int64]()
					idx.NotFound.Store(newNotFound)
					idx.NotFoundCount.Store(0)
					currentNotFound = newNotFound
				}
				if currentNotFound != nil {
					if _, loaded := currentNotFound.LoadOrStore(pathSlash, op.ExpireAt); !loaded {
						idx.NotFoundCount.Add(1)
					} else {
						currentNotFound.Store(pathSlash, op.ExpireAt)
					}
				}
			case OpUpdateMetadata:
				if op.Callback != nil {
					err := op.Callback()
					if op.ErrChan != nil {
						op.ErrChan <- err
					}
				}
			}
		case respCh := <-idx.SnapChan:
			files := make(map[string]FileInfo)
			idx.Files.Range(func(k string, v FileInfo) bool {
				files[k] = v
				return true
			})

			var dirs []string
			idx.Dirs.Range(func(k string, _ bool) bool {
				dirs = append(dirs, k)
				return true
			})

			notFounds := make(map[string]int64)
			currentNotFound := idx.NotFound.Load()
			if currentNotFound != nil {
				currentNotFound.Range(func(k string, v int64) bool {
					notFounds[k] = v
					return true
				})
			}

			respCh <- FileIndexSnapshot{
				Files:    files,
				Dirs:     dirs,
				NotFound: notFounds,
			}
		}
	}
}

func (idx *FileIndex) HasFile(pathStr string) bool {
	_, ok := idx.Files.Load(toSlashFast(pathStr))
	return ok
}

func (idx *FileIndex) HasDir(pathStr string) bool {
	_, ok := idx.Dirs.Load(toSlashFast(pathStr))
	return ok
}

func (idx *FileIndex) GetFileInfo(pathStr string) (FileInfo, bool) {
	val, ok := idx.Files.Load(toSlashFast(pathStr))
	return val, ok
}

func (idx *FileIndex) GetPathState(pathStr string) (isDir bool, info FileInfo, ok bool, isNotFound bool) {
	pathStr = toSlashFast(pathStr)

	currentNotFound := idx.NotFound.Load()
	if currentNotFound != nil {
		if val, exists := currentNotFound.Load(pathStr); exists {
			if time.Now().Unix() < val {
				return false, FileInfo{}, false, true
			}
		}
	}

	if val, exists := idx.Files.Load(pathStr); exists {
		return false, val, true, false
	}

	if _, exists := idx.Dirs.Load(pathStr); exists {
		return true, FileInfo{}, true, false
	}

	return false, FileInfo{}, false, false
}

func (idx *FileIndex) InsertFile(pathStr string, info ...FileInfo) {
	pathStr = toSlashFast(pathStr)
	var fileInfo FileInfo
	if len(info) > 0 {
		fileInfo = info[0]
	}
	if idx.isSync {
		if _, loaded := idx.Files.LoadOrStore(pathStr, fileInfo); !loaded {
			idx.FilesCount.Add(1)
			idx.IsDirty.Store(true)
			idx.addChild(pathStr)
		} else {
			idx.Files.Store(pathStr, fileInfo)
			idx.addChild(pathStr)
		}
		idx.clearNotFound(pathStr)
		return
	}
	idx.OpChan <- IndexOp{Type: OpInsertFile, Path: pathStr, Info: fileInfo}
}

func (idx *FileIndex) InsertDir(pathStr string) {
	pathStr = toSlashFast(pathStr)
	if idx.isSync {
		if _, loaded := idx.Dirs.LoadOrStore(pathStr, true); !loaded {
			idx.DirsCount.Add(1)
			idx.IsDirty.Store(true)
			idx.addChild(pathStr)
		}
		idx.clearNotFound(pathStr)
		return
	}
	idx.OpChan <- IndexOp{Type: OpInsertDir, Path: pathStr}
}

func (idx *FileIndex) RemoveFile(pathStr string) {
	pathStr = toSlashFast(pathStr)
	if idx.isSync {
		if _, loaded := idx.Files.LoadAndDelete(pathStr); loaded {
			idx.FilesCount.Add(^uint64(0))
			idx.IsDirty.Store(true)
			idx.removeChild(pathStr)
		}
		idx.clearNotFound(pathStr)
		return
	}
	idx.OpChan <- IndexOp{Type: OpRemoveFile, Path: pathStr}
}

func (idx *FileIndex) RemoveDir(pathStr string) {
	pathStr = toSlashFast(pathStr)
	if idx.isSync {
		// Purge descendants first while Children[path] still lists them.
		// removeChild deletes that entry, so it must run after removeDescendants.
		idx.removeDescendants(pathStr)
		if _, loaded := idx.Dirs.LoadAndDelete(pathStr); loaded {
			idx.DirsCount.Add(^uint64(0))
			idx.IsDirty.Store(true)
		}
		idx.removeChild(pathStr)
		idx.clearNotFound(pathStr)
		return
	}
	idx.OpChan <- IndexOp{Type: OpRemoveDir, Path: pathStr}
}

func (idx *FileIndex) clearNotFound(pathStr string) {
	currentNotFound := idx.NotFound.Load()
	if currentNotFound == nil {
		return
	}
	if _, loaded := currentNotFound.LoadAndDelete(pathStr); loaded {
		idx.NotFoundCount.Add(^uint64(0))
		idx.IsDirty.Store(true)
	}
}

func (idx *FileIndex) removeDescendants(dirPath string) {
	dirCleaned := cleanPathFast(dirPath)
	children := idx.GetChildren(dirCleaned)
	for _, child := range children {
		childPath := dirCleaned + "/" + child
		if _, loaded := idx.Files.LoadAndDelete(childPath); loaded {
			idx.FilesCount.Add(^uint64(0))
			idx.removeChild(childPath)
		}
		if _, loaded := idx.Dirs.LoadAndDelete(childPath); loaded {
			idx.DirsCount.Add(^uint64(0))
			idx.removeDescendants(childPath)
			idx.removeChild(childPath)
		}
		idx.clearNotFound(childPath)
	}

	currentNotFound := idx.NotFound.Load()
	if currentNotFound == nil {
		return
	}
	prefix := dirCleaned + "/"
	currentNotFound.Range(func(k string, _ int64) bool {
		if k == dirCleaned || strings.HasPrefix(k, prefix) {
			if _, loaded := currentNotFound.LoadAndDelete(k); loaded {
				idx.NotFoundCount.Add(^uint64(0))
				idx.IsDirty.Store(true)
			}
		}
		return true
	})
}

func (idx *FileIndex) InsertNotFound(pathStr string, expireAt int64) {
	pathStr = toSlashFast(pathStr)
	if idx.isSync {
		currentNotFound := idx.NotFound.Load()
		if idx.NotFoundCount.Load() >= 10000 {
			idx.PruneNotFound()
			currentNotFound = idx.NotFound.Load()
			if idx.NotFoundCount.Load() >= 10000 {
				newNotFound := pb.NewMapOf[string, int64]()
				if idx.NotFound.CompareAndSwap(currentNotFound, newNotFound) {
					idx.NotFoundCount.Store(0)
					currentNotFound = newNotFound
				} else {
					currentNotFound = idx.NotFound.Load()
				}
			}
		}
		if currentNotFound != nil {
			if _, loaded := currentNotFound.LoadOrStore(pathStr, expireAt); !loaded {
				idx.NotFoundCount.Add(1)
			} else {
				currentNotFound.Store(pathStr, expireAt)
			}
		}
		return
	}
	idx.OpChan <- IndexOp{Type: OpInsertNotFound, Path: pathStr, ExpireAt: expireAt}
}

func (idx *FileIndex) PruneNotFound() {
	if idx.isSync {
		currentTime := time.Now().Unix()
		removedCount := 0
		currentNotFound := idx.NotFound.Load()
		if currentNotFound != nil {
			currentNotFound.Range(func(k string, v int64) bool {
				if v <= currentTime {
					currentNotFound.Delete(k)
					removedCount++
				}
				return true
			})
			if removedCount > 0 {
				idx.NotFoundCount.Add(^uint64(removedCount - 1))
			}
		}
		return
	}
	idx.OpChan <- IndexOp{Type: OpPruneNotFound}
}

func (idx *FileIndex) UpdateMetadataCallback(cb func() error) error {
	idx.metadataLock.Lock()
	defer idx.metadataLock.Unlock()
	return cb()
}

func appendFastQuote(buf []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' || c == '\\' || c < 0x20 || c >= 0x7f {
			return strconv.AppendQuote(buf, s)
		}
	}
	buf = append(buf, '"')
	buf = append(buf, s...)
	buf = append(buf, '"')
	return buf
}

func (idx *FileIndex) WriteJSONTo(w io.Writer) (returnErr error) {
	bw, ok := w.(*bufio.Writer)
	if !ok {
		bw = bufio.NewWriterSize(w, 64*1024)
		defer func() {
			if flushErr := bw.Flush(); returnErr == nil {
				returnErr = flushErr
			}
		}()
	}

	if _, err := bw.WriteString(`{"files":{`); err != nil {
		return err
	}

	first := true
	var rangeErr error
	valBuf := make([]byte, 0, 128)
	quoteBuf := make([]byte, 0, 256)

	idx.Files.Range(func(k string, v FileInfo) bool {
		if !first {
			if _, err := bw.WriteString(","); err != nil {
				rangeErr = err
				return false
			}
		}
		first = false

		quoteBuf = appendFastQuote(quoteBuf[:0], k)
		if _, err := bw.Write(quoteBuf); err != nil {
			rangeErr = err
			return false
		}
		if err := bw.WriteByte(':'); err != nil {
			rangeErr = err
			return false
		}

		valBuf = valBuf[:0]
		valBuf = append(valBuf, `{"size":`...)
		valBuf = strconv.AppendInt(valBuf, v.Size, 10)
		valBuf = append(valBuf, `,"mod_time":`...)
		valBuf = strconv.AppendInt(valBuf, v.ModTime, 10)
		valBuf = append(valBuf, '}')

		if _, err := bw.Write(valBuf); err != nil {
			rangeErr = err
			return false
		}
		return true
	})
	if rangeErr != nil {
		return rangeErr
	}

	if _, err := bw.WriteString(`},"dirs":[`); err != nil {
		return err
	}

	first = true
	idx.Dirs.Range(func(k string, _ bool) bool {
		if !first {
			if _, err := bw.WriteString(","); err != nil {
				rangeErr = err
				return false
			}
		}
		first = false

		quoteBuf = appendFastQuote(quoteBuf[:0], k)
		if _, err := bw.Write(quoteBuf); err != nil {
			rangeErr = err
			return false
		}
		return true
	})
	if rangeErr != nil {
		return rangeErr
	}

	if _, err := bw.WriteString(`]}`); err != nil {
		return err
	}

	return nil
}

type FileIndexSnapshot struct {
	Files    map[string]FileInfo `json:"files"`
	Dirs     []string            `json:"dirs"`
	NotFound map[string]int64    `json:"not_found"`
}

func (idx *FileIndex) IsNotFound(pathStr string) bool {
	currentNotFound := idx.NotFound.Load()
	if currentNotFound == nil {
		return false
	}
	val, exists := currentNotFound.Load(toSlashFast(pathStr))
	if !exists {
		return false
	}
	expireAt := val
	currentTime := time.Now().Unix()
	return currentTime < expireAt
}

func (idx *FileIndex) EnsureParentDirs(basePath string) {
	basePath = toSlashFast(basePath)
	var pathsToInsert []string
	currentPath := basePath
	for {
		parent := path.Dir(currentPath)
		if parent == currentPath || parent == "." || parent == "/" || parent == "" {
			break
		}
		if idx.HasDir(parent) {
			break
		}
		pathsToInsert = append(pathsToInsert, parent)
		currentPath = parent
	}
	for _, p := range pathsToInsert {
		idx.InsertDir(p)
	}
}

func (idx *FileIndex) Snapshot() FileIndexSnapshot {
	if idx.isSync {
		files := make(map[string]FileInfo)
		idx.Files.Range(func(k string, v FileInfo) bool {
			files[k] = v
			return true
		})

		var dirs []string
		idx.Dirs.Range(func(k string, _ bool) bool {
			dirs = append(dirs, k)
			return true
		})

		notFounds := make(map[string]int64)
		currentNotFound := idx.NotFound.Load()
		if currentNotFound != nil {
			currentNotFound.Range(func(k string, v int64) bool {
				notFounds[k] = v
				return true
			})
		}

		return FileIndexSnapshot{
			Files:    files,
			Dirs:     dirs,
			NotFound: notFounds,
		}
	}
	respCh := make(chan FileIndexSnapshot)
	idx.SnapChan <- respCh
	return <-respCh
}

func (idx *FileIndex) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	if err := idx.WriteJSONTo(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (idx *FileIndex) UnmarshalJSON(data []byte) error {
	var raw struct {
		Files    json.RawMessage  `json:"files"`
		Dirs     []string         `json:"dirs"`
		NotFound map[string]int64 `json:"not_found"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	for _, dir := range raw.Dirs {
		dirSlash := filepath.ToSlash(dir)
		idx.Dirs.Store(dirSlash, true)
		idx.DirsCount.Add(1)
		idx.addChild(dirSlash)
	}

	currentNotFound := idx.NotFound.Load()
	if currentNotFound == nil {
		currentNotFound = pb.NewMapOf[string, int64]()
		idx.NotFound.Store(currentNotFound)
	}
	for pathStr, expireAt := range raw.NotFound {
		pathSlash := filepath.ToSlash(pathStr)
		currentNotFound.Store(pathSlash, expireAt)
		idx.NotFoundCount.Add(1)
	}

	if len(raw.Files) > 0 {
		if raw.Files[0] == '[' {
			var fileList []string
			if err := json.Unmarshal(raw.Files, &fileList); err != nil {
				return err
			}
			for _, file := range fileList {
				fileSlash := filepath.ToSlash(file)
				var size int64
				var modTime int64
				if info, err := os.Stat(fileSlash); err == nil {
					size = info.Size()
					modTime = info.ModTime().UnixNano()
				}
				idx.Files.Store(fileSlash, FileInfo{Size: size, ModTime: modTime})
				idx.FilesCount.Add(1)
				idx.addChild(fileSlash)
			}
		} else if raw.Files[0] == '{' {
			var fileMap map[string]FileInfo
			if err := json.Unmarshal(raw.Files, &fileMap); err != nil {
				return err
			}
			for file, info := range fileMap {
				fileSlash := filepath.ToSlash(file)
				idx.Files.Store(fileSlash, info)
				idx.FilesCount.Add(1)
				idx.addChild(fileSlash)
			}
		}
	}

	return nil
}

func (idx *FileIndex) GetChildren(dirPath string) []string {
	dirPath = cleanPathFast(dirPath)
	idx.ChildrenMutex.RLock()
	defer idx.ChildrenMutex.RUnlock()
	if idx.Children == nil {
		return []string{}
	}
	list, ok := idx.Children[dirPath]
	if !ok || len(list) == 0 {
		return []string{}
	}
	children := make([]string, len(list))
	copy(children, list)
	return children
}

func (idx *FileIndex) Walk(root string, walkFn func(pathStr string, info FileInfo, isDir bool) bool) {
	root = cleanPathFast(root)

	var traverse func(string) bool
	traverse = func(current string) bool {
		if idx.HasDir(current) {
			if !walkFn(current, FileInfo{}, true) {
				return false
			}
			children := idx.GetChildren(current)
			for _, child := range children {
				childPath := path.Join(current, child)
				if !traverse(childPath) {
					return false
				}
			}
		} else if info, ok := idx.GetFileInfo(current); ok {
			if !walkFn(current, info, false) {
				return false
			}
		}
		return true
	}

	traverse(root)
}
