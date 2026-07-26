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
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/panjf2000/ants/v2"

	"renop/core"
	"renop/index"
	"renop/utils"
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
						rc, _, err := DownloadFromS3(utils.GetS3Key(localFilePath))
						if err == nil {
							defer rc.Close()
							data, _ := io.ReadAll(rc)
							return true, c.Send(data)
						}
					} else {
						data, err := os.ReadFile(localFilePath)
						if err == nil {
							return true, c.Send(data)
						}
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
				bufPtr := bufferPool128k.Get().(*[]byte)
				_, copyErr := io.CopyBuffer(mw, r, *bufPtr)
				bufferPool128k.Put(bufPtr)
				if copyErr == nil {
					md5Str := hex.EncodeToString(hMd5.Sum(nil))
					sha1Str := hex.EncodeToString(hSha1.Sum(nil))
					sha256Str := hex.EncodeToString(hSha256.Sum(nil))
					sha512Str := hex.EncodeToString(hSha512.Sum(nil))

					hashes := map[string]string{
						".md5":    md5Str,
						".sha1":   sha1Str,
						".sha256": sha256Str,
						".sha512": sha512Str,
					}

					now := time.Now().UnixNano()
					var wg sync.WaitGroup
					for cExt, cHash := range hashes {
						wg.Add(1)
						extStr, hashStr := cExt, cHash
						err := ants.Submit(func() {
							defer wg.Done()
							cPath := basePath + extStr
							if IsS3Enabled(cPath) {
								s3Key := utils.GetS3Key(cPath)
								_ = UploadStreamToS3(s3Key, strings.NewReader(hashStr), int64(len(hashStr)), "text/plain")
							} else {
								_ = os.WriteFile(cPath, []byte(hashStr), 0644)
							}
							state.Inner.FileIndex.InsertFile(cPath, index.FileInfo{
								Size:    int64(len(hashStr)),
								ModTime: now,
							})
						})
						if err != nil {
							cPath := basePath + extStr
							if IsS3Enabled(cPath) {
								s3Key := utils.GetS3Key(cPath)
								_ = UploadStreamToS3(s3Key, strings.NewReader(hashStr), int64(len(hashStr)), "text/plain")
							} else {
								_ = os.WriteFile(cPath, []byte(hashStr), 0644)
							}
							state.Inner.FileIndex.InsertFile(cPath, index.FileInfo{
								Size:    int64(len(hashStr)),
								ModTime: now,
							})
							wg.Done()
						}
					}
					wg.Wait()
					computedSuccess = true
					reqHashStr := hashes[ext]
					c.Set(fiber.HeaderContentType, "text/plain")
					c.Set("Content-Security-Policy", "default-src 'none'; sandbox")
					return true, c.SendString(reqHashStr)
				}
			}
		}
	}
	return false, nil
}
