/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package storage

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v3"

	"renop/internal/core"
	"renop/internal/utils"
)

func handleChecksumFallback(c fiber.Ctx, localFilePath string, state *core.AppState) (bool, error) {
	ext := filepath.Ext(localFilePath)
	if ext == ".md5" || ext == ".sha1" || ext == ".sha256" || ext == ".sha512" {
		basePath := strings.TrimSuffix(localFilePath, ext)

		baseExt := filepath.Ext(basePath)
		isChecksumOfChecksum := baseExt == ".md5" || baseExt == ".sha1" || baseExt == ".sha256" || baseExt == ".sha512"

		baseExists := state.Inner.FileIndex.HasFile(basePath)
		if !isChecksumOfChecksum && baseExists {
			pathKey := filepath.ToSlash(basePath) + ".checksum_calc"
			dl, loaded := state.Inner.InFlightDownloads.LockPath(pathKey)
			if loaded {
				state.Inner.InFlightDownloads.Wait(dl)
				if state.Inner.FileIndex.HasFile(localFilePath) {
					c.Set(fiber.HeaderContentType, "text/plain")
					c.Set("Content-Security-Policy", "default-src 'none'; sandbox")
					if IsS3Enabled(localFilePath) {
						rc, info, err := DownloadFromS3(utils.GetS3Key(localFilePath))
						if err == nil {
							if info.Size > 0 {
								return true, c.SendStream(rc, int(info.Size))
							}
							return true, c.SendStream(rc)
						}
					} else if f, err := os.Open(localFilePath); err == nil {
						if st, stErr := f.Stat(); stErr == nil {
							return true, c.SendStream(f, int(st.Size()))
						}
						return true, c.SendStream(f)
					}
				}
				return false, nil
			}
			computedSuccess := false
			defer func() {
				state.Inner.InFlightDownloads.UnlockPath(pathKey, dl, computedSuccess)
			}()

			var r io.ReadCloser
			var err error
			if IsS3Enabled(basePath) {
				s3Key := utils.GetS3Key(basePath)
				r, _, err = DownloadFromS3(s3Key)
			} else {
				r, err = os.Open(basePath)
			}

			if err == nil {
				defer r.Close()

				hMd5 := md5.New()
				hSha1 := sha1.New()
				hSha256 := sha256.New()
				hSha512 := sha512.New()

				mw := io.MultiWriter(hMd5, hSha1, hSha256, hSha512)
				bufPtr := bufferPool128k.Get()
				_, copyErr := io.CopyBuffer(mw, r, *bufPtr)
				bufferPool128k.Put(bufPtr)
				if copyErr == nil {
					md5Str := hex.EncodeToString(hMd5.Sum(nil))
					sha1Str := hex.EncodeToString(hSha1.Sum(nil))
					sha256Str := hex.EncodeToString(hSha256.Sum(nil))
					sha512Str := hex.EncodeToString(hSha512.Sum(nil))

					checksums := [...]ArtifactChecksumEntry{
						{Ext: ".md5", Hash: md5Str},
						{Ext: ".sha1", Hash: sha1Str},
						{Ext: ".sha256", Hash: sha256Str},
						{Ext: ".sha512", Hash: sha512Str},
					}

					var wg sync.WaitGroup
					errChan := make(chan error, len(checksums))
					var reqHashStr string
					for i := range checksums {
						wg.Add(1)
						cs := checksums[i]
						if cs.Ext == ext {
							reqHashStr = cs.Hash
						}
						go func() {
							defer wg.Done()
							if err := SaveAndUploadChecksum(state, basePath, cs.Ext, cs.Hash); err != nil {
								errChan <- err
							}
						}()
					}
					wg.Wait()
					close(errChan)
					var persistErr error
					for err := range errChan {
						persistErr = errors.Join(persistErr, err)
					}
					if persistErr != nil {
						return false, persistErr
					}
					computedSuccess = true
					c.Set(fiber.HeaderContentType, "text/plain")
					c.Set("Content-Security-Policy", "default-src 'none'; sandbox")
					return true, c.SendString(reqHashStr)
				}
			}
		}
	}
	return false, nil
}
