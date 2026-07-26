/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package proxy

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	"renop/core"
	"renop/index"
	"renop/utils"
)

// proxyStreamWriteBufSize is the size of the per-stream disk write buffer.
const proxyStreamWriteBufSize = 128 * 1024

// proxyStreamReader tees the upstream HTTP body into a temp file while
// simultaneously serving the bytes to the downstream caller (Fiber response
// writer). On Close it atomically renames the temp file to its final location
// when the full body was received successfully.
type proxyStreamReader struct {
	bodyReader io.ReadCloser
	tmpFile    *os.File
	writeBuf   []byte // owned 128 KiB buffer; nil when tmpFile is nil
	writePos   int    // bytes buffered in writeBuf pending the next flush

	tmpPath    string // path of the temp file
	tmpKeep    bool   // set true after a successful SafeRename
	targetPath string // final destination path

	inFlightMgr *core.InFlightManager
	pathStr     string
	dl          *core.InFlightDownload
	fileIndex   *index.FileIndex
	onSuccess   func(string) bool

	permit     chan struct{}
	permitOnce sync.Once
	cancel     context.CancelFunc

	expectedSize int64
	bytesWritten int64
	readEOF      bool
	success      bool

	closeOnce sync.Once
	closeErr  error
}

func (p *proxyStreamReader) releasePermit() {
	p.permitOnce.Do(func() {
		if p.permit != nil {
			<-p.permit
		}
	})
}

func (p *proxyStreamReader) Read(b []byte) (n int, err error) {
	n, err = p.bodyReader.Read(b)
	if n > 0 && p.tmpFile != nil {
		if wErr := p.writeToTmp(b[:n]); wErr != nil {
			p.success = false
		} else {
			p.bytesWritten += int64(n)
		}
	}
	if err != nil {
		if err == io.EOF {
			p.readEOF = true
		} else {
			p.success = false
		}
		p.releasePermit()
	}
	return n, err
}

// writeToTmp copies data into writeBuf, flushing a full buffer to tmpFile
// whenever the buffer is full.
func (p *proxyStreamReader) writeToTmp(data []byte) error {
	for len(data) > 0 {
		n := copy(p.writeBuf[p.writePos:], data)
		p.writePos += n
		data = data[n:]
		if p.writePos == len(p.writeBuf) {
			if _, err := p.tmpFile.Write(p.writeBuf); err != nil {
				return err
			}
			p.writePos = 0
		}
	}
	return nil
}

// flushTmp writes any bytes remaining in writeBuf to tmpFile.
func (p *proxyStreamReader) flushTmp() error {
	if p.writePos == 0 {
		return nil
	}
	_, err := p.tmpFile.Write(p.writeBuf[:p.writePos])
	p.writePos = 0
	return err
}

func (p *proxyStreamReader) Close() error {
	p.closeOnce.Do(func() {
		success := p.success && p.readEOF && (p.expectedSize < 0 || p.bytesWritten == p.expectedSize)

		defer func() {
			if p.inFlightMgr != nil {
				p.inFlightMgr.UnlockPath(p.pathStr, p.dl, success)
			}
			p.releasePermit()
			if p.cancel != nil {
				p.cancel()
			}
			p.writeBuf = nil
		}()

		p.closeErr = p.bodyReader.Close()
		if p.closeErr != nil {
			success = false
		}

		if p.tmpFile != nil {
			if ferr := p.flushTmp(); ferr != nil {
				success = false
				if p.closeErr == nil {
					p.closeErr = ferr
				}
			}
			if cerr := p.tmpFile.Close(); cerr != nil && p.closeErr == nil {
				p.closeErr = cerr
			}
			p.tmpFile = nil
		}

		if success && p.tmpPath != "" {
			if err := utils.SafeRename(p.tmpPath, p.targetPath); err == nil {
				p.tmpKeep = true
				if p.fileIndex != nil {
					p.fileIndex.EnsureParentDirs(p.targetPath)
					size := p.bytesWritten
					if stat, err := os.Stat(p.targetPath); err == nil {
						size = stat.Size()
					}
					p.fileIndex.InsertFile(p.targetPath, index.FileInfo{
						Size:    size,
						ModTime: time.Now().UnixNano(),
					})
				}
				if p.onSuccess != nil {
					if !p.onSuccess(p.targetPath) {
						success = false
						_ = os.Remove(p.targetPath)
						if p.fileIndex != nil {
							p.fileIndex.RemoveFile(p.targetPath)
						}
					}
				}
			} else {
				success = false
				if p.closeErr == nil {
					p.closeErr = err
				}
			}
		}

		if p.tmpPath != "" && !p.tmpKeep {
			_ = os.Remove(p.tmpPath)
		}
	})

	return p.closeErr
}

// CreateProxyStream returns an io.ReadCloser that tees the upstream body to
// a temp file. The caller reads from the stream as normal; on Close the temp
// file is renamed to localFilePath when the body was fully and correctly received.
func CreateProxyStream(
	bodyReader io.ReadCloser,
	expectedSize int64,
	localFilePath string,
	inFlightMgr *core.InFlightManager,
	pathStr string,
	dl *core.InFlightDownload,
	permit chan struct{},
	cancel context.CancelFunc,
	fileIndex *index.FileIndex,
	onSuccess func(string) bool,
) io.ReadCloser {
	dir := filepath.Dir(localFilePath)
	_ = os.MkdirAll(dir, 0755)

	uniqueID := uuid.New().String()
	tmpPath := localFilePath + ".tmp." + uniqueID

	file, err := os.Create(tmpPath)

	var writeBuf []byte
	if err == nil {
		writeBuf = make([]byte, proxyStreamWriteBufSize)
	}

	return &proxyStreamReader{
		bodyReader:   bodyReader,
		tmpFile:      file,
		writeBuf:     writeBuf,
		tmpPath:      tmpPath,
		targetPath:   localFilePath,
		inFlightMgr:  inFlightMgr,
		pathStr:      pathStr,
		dl:           dl,
		permit:       permit,
		cancel:       cancel,
		fileIndex:    fileIndex,
		onSuccess:    onSuccess,
		expectedSize: expectedSize,
		success:      err == nil,
	}
}
