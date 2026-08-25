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
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/llxisdsh/pb"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/cargodocs"
	"renop/internal/service/gpg"
	"renop/internal/service/index"
	"renop/internal/service/javadocs"
	"renop/internal/service/proxy"
	"renop/internal/utils"
)

var (
	s3Clients     pb.MapOf[string, *minio.Client]
	currentConfig atomic.Pointer[config.Config]
)

const (
	s3TransferTimeout = 30 * time.Minute
	s3RequestTimeout  = 30 * time.Second
)

type cancelReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
	once   sync.Once
	err    error
}

func (r *cancelReadCloser) Close() error {
	r.once.Do(func() {
		r.err = r.ReadCloser.Close()
		r.cancel()
	})
	return r.err
}

func init() {
	index.S3IndexBuilder = BuildS3IndexSync
	proxy.IsS3Enabled = func(repo *config.Repository) bool {
		return repo != nil && repo.S3 != nil && repo.S3.Enabled
	}
	proxy.UploadToS3 = func(repo *config.Repository, localPath string, s3Key string) error {
		if repo == nil || repo.S3 == nil {
			return nil
		}
		return UploadToS3Direct(repo.S3, localPath, s3Key)
	}
	proxy.UploadStreamToS3 = func(repo *config.Repository, s3Key string, reader io.Reader, size int64, contentType string) error {
		if repo == nil || repo.S3 == nil {
			return nil
		}
		return UploadStreamToS3(s3Key, reader, size, contentType)
	}
	proxy.OnArtifactStored = func(localPath string) {
		if strings.HasSuffix(localPath, "-javadoc.jar") {
			javadocs.CleanupJavadoc(localPath)
		}
	}
	proxy.OnArtifactStoredWithState = func(state *core.AppState, repo *config.Repository, localPath string) {
		if state == nil || repo == nil {
			return
		}
		if gpg.IsProtectedArtifact(filepath.ToSlash(localPath)) {
			gpgReleaseStorageMutation.Lock()
			err := RemoveArtifactGPGSignature(state, localPath)
			gpgReleaseStorageMutation.Unlock()
			if err != nil {
				log.Printf("failed to invalidate stale GPG signature for mirrored artifact %s: %v", localPath, err)
			}
		}
		if isMavenMetadataPath(localPath) {
			state.InvalidateFileCache(localPath)
			if err := cleanupSnapshotArtifactsFromMetadata(state, localPath); err != nil {
				log.Printf("failed to reconcile Maven SNAPSHOT metadata %s: %v", localPath, err)
			}
			return
		}
		if isSnapshotArtifactPath(localPath) && !isArtifactCompanionPath(localPath) {
			if err := cleanupSupersededUniqueSnapshots(state, localPath); err != nil {
				log.Printf("failed to clean superseded Maven SNAPSHOT artifact %s: %v", localPath, err)
			}
		}
	}
	javadocs.IsS3Enabled = IsS3Enabled
	javadocs.DownloadFromS3 = DownloadFromS3
	cargodocs.IsS3Enabled = IsS3Enabled
	cargodocs.DownloadFromS3 = DownloadFromS3
}

func InitS3(cfg *config.Config) {
	currentConfig.Store(cfg)
	s3Clients.Range(func(key string, _ *minio.Client) bool {
		s3Clients.Delete(key)
		return true
	})
}

func GetS3Client(repoS3 *config.S3Config) (*minio.Client, error) {
	if repoS3 == nil || !repoS3.Enabled {
		return nil, nil
	}

	pathStyle := "0"
	if repoS3.ForcePathStyle {
		pathStyle = "1"
	}
	keyMaterial := repoS3.Endpoint + "\x00" + repoS3.Bucket + "\x00" + repoS3.Region + "\x00" +
		repoS3.AccessKeyID + "\x00" + repoS3.SecretAccessKey + "\x00" + strings.ToLower(repoS3.Endpoint) +
		"\x00" + pathStyle
	keyHash := sha256.Sum256([]byte(keyMaterial))
	key := string(keyHash[:])
	if client, ok := s3Clients.Load(key); ok {
		return client, nil
	}

	endpoint := repoS3.Endpoint
	u, err := url.Parse(endpoint)
	var endpointHost string
	useSSL := true
	if err == nil && u.Host != "" {
		if (u.Scheme != "http" && u.Scheme != "https") || u.User != nil ||
			(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
			return nil, errors.New("S3 endpoint must be an http(s) origin without credentials, path, query, or fragment")
		}
		endpointHost = u.Host
		if u.Scheme == "http" {
			useSSL = false
		}
	} else {
		if strings.Contains(endpoint, "://") {
			return nil, errors.New("invalid S3 endpoint")
		}
		endpointHost = endpoint
	}
	transport, err := minio.DefaultTransport(useSSL)
	if err != nil {
		return nil, err
	}
	transport.MaxConnsPerHost = 64
	transport.MaxIdleConnsPerHost = 4
	transport.MaxIdleConns = 16
	transport.IdleConnTimeout = 15 * time.Second
	transport.DisableCompression = true
	transport.ResponseHeaderTimeout = 30 * time.Second
	transport.ExpectContinueTimeout = time.Second
	transport.ForceAttemptHTTP2 = false
	if transport.DialContext == nil {
		transport.DialContext = (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext
	}

	creds := credentials.NewStaticV4(repoS3.AccessKeyID, repoS3.SecretAccessKey, "")
	opts := &minio.Options{
		Creds:        creds,
		Secure:       useSSL,
		Transport:    transport,
		Region:       repoS3.Region,
		BucketLookup: minio.BucketLookupAuto,
		MaxRetries:   3,
	}
	if repoS3.ForcePathStyle {
		opts.BucketLookup = minio.BucketLookupPath
	}

	client, err := minio.New(endpointHost, opts)
	if err != nil {
		return nil, err
	}

	s3Clients.Store(key, client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	exists, err := client.BucketExists(ctx, repoS3.Bucket)
	if err == nil && !exists {
		_ = client.MakeBucket(ctx, repoS3.Bucket, minio.MakeBucketOptions{Region: repoS3.Region})
	}

	return client, nil
}

func GetS3ConfigForPath(path string) *config.S3Config {
	cfg := currentConfig.Load()
	if cfg == nil {
		return nil
	}
	rel := filepath.ToSlash(path)
	rel = strings.TrimPrefix(rel, "./")
	rel = strings.TrimPrefix(rel, "/")
	storagePrefix := filepath.ToSlash(cfg.StoragePath)
	storagePrefix = strings.TrimPrefix(storagePrefix, "./")
	storagePrefix = strings.TrimPrefix(storagePrefix, "/")
	if rel == storagePrefix {
		rel = ""
	} else if strings.HasPrefix(rel, storagePrefix+"/") {
		rel = strings.TrimPrefix(rel, storagePrefix)
		rel = strings.TrimPrefix(rel, "/")
	}
	parts := strings.Split(rel, "/")
	if len(parts) > 0 {
		repoName := parts[0]
		if repo, ok := cfg.Maven.Repositories[repoName]; ok {
			if repo.S3 != nil && repo.S3.Enabled {
				return repo.S3
			}
		}
	}
	return nil
}

func IsS3Enabled(path ...string) bool {
	if len(path) == 0 {
		return false
	}
	return GetS3ConfigForPath(path[0]) != nil
}

// NormalizeS3KeyPrefix validates and canonicalizes the optional bucket prefix.
func NormalizeS3KeyPrefix(raw string) (string, error) {
	prefix := strings.Trim(strings.TrimSpace(raw), "/")
	if prefix == "" {
		return "", nil
	}
	if strings.Contains(prefix, `\`) {
		return "", errors.New("S3 key prefix cannot contain backslashes")
	}
	for _, segment := range strings.Split(prefix, "/") {
		if segment == "" || segment == "." || segment == ".." || strings.TrimSpace(segment) != segment ||
			strings.IndexFunc(segment, unicode.IsControl) >= 0 {
			return "", errors.New("S3 key prefix contains an invalid path segment")
		}
	}
	return prefix, nil
}

func s3ObjectKey(s3Cfg *config.S3Config, logicalKey string) (string, error) {
	prefix, err := NormalizeS3KeyPrefix(s3Cfg.KeyPrefix)
	if err != nil {
		return "", err
	}
	if prefix == "" {
		return logicalKey, nil
	}

	cfg := currentConfig.Load()
	if cfg == nil {
		return "", errors.New("S3 configuration is not initialized")
	}
	key := utils.GetS3Key(logicalKey)
	storageKey := strings.TrimSuffix(utils.GetS3Key(filepath.Clean(cfg.StoragePath)), "/")
	relative := key
	if storageKey != "" && storageKey != "." {
		switch {
		case key == storageKey:
			relative = ""
		case strings.HasPrefix(key, storageKey+"/"):
			relative = strings.TrimPrefix(key, storageKey+"/")
		default:
			return "", errors.New("S3 key is outside the configured storage path")
		}
	}
	if relative == "" {
		return prefix, nil
	}
	return prefix + "/" + relative, nil
}

func UploadToS3(localPath string, s3Key string) error {
	s3Cfg := GetS3ConfigForPath(localPath)
	if s3Cfg == nil {
		return nil
	}
	return UploadToS3Direct(s3Cfg, localPath, s3Key)
}

func UploadToS3Direct(s3Cfg *config.S3Config, localPath string, s3Key string) error {
	objectKey, err := s3ObjectKey(s3Cfg, s3Key)
	if err != nil {
		return err
	}
	client, err := GetS3Client(s3Cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), s3TransferTimeout)
	defer cancel()
	_, err = client.FPutObject(ctx, s3Cfg.Bucket, objectKey, localPath, minio.PutObjectOptions{})
	return err
}

func DownloadFromS3(s3Key string) (io.ReadCloser, index.FileInfo, error) {
	s3Cfg := GetS3ConfigForPath(s3Key)
	if s3Cfg == nil {
		return nil, index.FileInfo{}, errors.New("S3 not enabled for path")
	}
	return DownloadFromS3Direct(s3Cfg, s3Key)
}

func DownloadFromS3Direct(s3Cfg *config.S3Config, s3Key string) (io.ReadCloser, index.FileInfo, error) {
	objectKey, err := s3ObjectKey(s3Cfg, s3Key)
	if err != nil {
		return nil, index.FileInfo{}, err
	}
	client, err := GetS3Client(s3Cfg)
	if err != nil {
		return nil, index.FileInfo{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), s3TransferTimeout)
	obj, err := client.GetObject(ctx, s3Cfg.Bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		cancel()
		return nil, index.FileInfo{}, err
	}
	stat, err := obj.Stat()
	if err != nil {
		obj.Close()
		cancel()
		return nil, index.FileInfo{}, err
	}
	return &cancelReadCloser{ReadCloser: obj, cancel: cancel}, index.FileInfo{
		Size:    stat.Size,
		ModTime: stat.LastModified.UnixNano(),
	}, nil
}

func DownloadRangeFromS3(s3Key string, start, end int64) (io.ReadCloser, error) {
	s3Cfg := GetS3ConfigForPath(s3Key)
	if s3Cfg == nil {
		return nil, errors.New("S3 not enabled for path")
	}
	objectKey, err := s3ObjectKey(s3Cfg, s3Key)
	if err != nil {
		return nil, err
	}
	client, err := GetS3Client(s3Cfg)
	if err != nil {
		return nil, err
	}
	var options minio.GetObjectOptions
	if err := options.SetRange(start, end); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), s3TransferTimeout)
	obj, err := client.GetObject(ctx, s3Cfg.Bucket, objectKey, options)
	if err != nil {
		cancel()
		return nil, err
	}
	return &cancelReadCloser{ReadCloser: obj, cancel: cancel}, nil
}

func GetS3PresignedURL(s3Key string, expires time.Duration) (string, error) {
	s3Cfg := GetS3ConfigForPath(s3Key)
	if s3Cfg == nil {
		return "", errors.New("S3 not enabled for path")
	}
	objectKey, err := s3ObjectKey(s3Cfg, s3Key)
	if err != nil {
		return "", err
	}
	client, err := GetS3Client(s3Cfg)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), s3RequestTimeout)
	defer cancel()
	reqParams := make(url.Values)
	u, err := client.PresignedGetObject(ctx, s3Cfg.Bucket, objectKey, expires, reqParams)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func UploadStreamToS3(s3Key string, reader io.Reader, size int64, contentType string) error {
	s3Cfg := GetS3ConfigForPath(s3Key)
	if s3Cfg == nil {
		return nil
	}
	objectKey, err := s3ObjectKey(s3Cfg, s3Key)
	if err != nil {
		return err
	}
	client, err := GetS3Client(s3Cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), s3TransferTimeout)
	defer cancel()
	_, err = client.PutObject(ctx, s3Cfg.Bucket, objectKey, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

func DeleteFromS3(s3Key string) error {
	s3Cfg := GetS3ConfigForPath(s3Key)
	if s3Cfg == nil {
		return nil
	}
	objectKey, err := s3ObjectKey(s3Cfg, s3Key)
	if err != nil {
		return err
	}
	client, err := GetS3Client(s3Cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), s3RequestTimeout)
	defer cancel()
	return client.RemoveObject(ctx, s3Cfg.Bucket, objectKey, minio.RemoveObjectOptions{})
}

func DeletePrefixFromS3(s3Prefix string) error {
	return DeletePrefixFromS3Config(GetS3ConfigForPath(s3Prefix), s3Prefix)
}

// DeletePrefixFromS3Config removes all objects under s3Prefix using an explicit
// S3 config (needed when the repository has already been removed from live config).
func DeletePrefixFromS3Config(s3Cfg *config.S3Config, s3Prefix string) error {
	if s3Cfg == nil || !s3Cfg.Enabled {
		return nil
	}
	objectPrefix, err := s3ObjectKey(s3Cfg, s3Prefix)
	if err != nil {
		return err
	}
	client, err := GetS3Client(s3Cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), s3TransferTimeout)
	defer cancel()
	objectsCh := make(chan minio.ObjectInfo)
	go func() {
		defer close(objectsCh)
		for object := range client.ListObjects(ctx, s3Cfg.Bucket, minio.ListObjectsOptions{
			Prefix:    objectPrefix,
			Recursive: true,
		}) {
			if object.Err != nil {
				log.Printf("Error listing S3 object under prefix %s: %v", objectPrefix, object.Err)
				continue
			}
			select {
			case objectsCh <- object:
			case <-ctx.Done():
				return
			}
		}
	}()
	errorCh := client.RemoveObjects(ctx, s3Cfg.Bucket, objectsCh, minio.RemoveObjectsOptions{})
	for err := range errorCh {
		if err.Err != nil {
			return err.Err
		}
	}
	return nil
}

func StatS3(s3Key string) (index.FileInfo, error) {
	s3Cfg := GetS3ConfigForPath(s3Key)
	if s3Cfg == nil {
		return index.FileInfo{}, errors.New("S3 not enabled for path")
	}
	objectKey, err := s3ObjectKey(s3Cfg, s3Key)
	if err != nil {
		return index.FileInfo{}, err
	}
	client, err := GetS3Client(s3Cfg)
	if err != nil {
		return index.FileInfo{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), s3RequestTimeout)
	defer cancel()
	objInfo, err := client.StatObject(ctx, s3Cfg.Bucket, objectKey, minio.StatObjectOptions{})
	if err != nil {
		return index.FileInfo{}, err
	}
	return index.FileInfo{
		Size:    objInfo.Size,
		ModTime: objInfo.LastModified.UnixNano(),
	}, nil
}

func BuildS3IndexSync(storagePath string, idx *index.FileIndex) error {
	cfg := currentConfig.Load()
	if cfg == nil {
		return errors.New("S3 configuration is not initialized")
	}

	idx.InsertDir(storagePath)

	for repoName, repo := range cfg.Maven.Repositories {
		repoDir := filepath.Join(storagePath, repoName)
		idx.InsertDir(repoDir)

		if repo.S3 != nil && repo.S3.Enabled {
			client, err := GetS3Client(repo.S3)
			if err != nil {
				return fmt.Errorf("get S3 client for repository %q: %w", repoName, err)
			}

			prefix, err := s3ObjectKey(repo.S3, utils.GetS3Key(repoDir))
			if err != nil {
				return fmt.Errorf("resolve S3 key prefix for repository %q: %w", repoName, err)
			}
			if !strings.HasSuffix(prefix, "/") {
				prefix += "/"
			}
			ctx, cancel := context.WithTimeout(context.Background(), s3TransferTimeout)

			for object := range client.ListObjects(ctx, repo.S3.Bucket, minio.ListObjectsOptions{
				Prefix:    prefix,
				Recursive: true,
			}) {
				if object.Err != nil {
					cancel()
					return fmt.Errorf("list S3 objects for repository %q: %w", repoName, object.Err)
				}

				localKey, ok := localPathFromS3Object(repoDir, prefix, object.Key)
				if !ok {
					log.Printf("Ignoring invalid S3 object key %q for repo %q", object.Key, repoName)
					continue
				}
				idx.EnsureParentDirs(localKey)
				idx.InsertFile(localKey, index.FileInfo{
					Size:    object.Size,
					ModTime: object.LastModified.UnixNano(),
				})
			}
			cancel()
		} else {
			index.ScanLocalDir(repoDir, idx, false)
		}
	}
	return nil
}

func localPathFromS3Object(repoDir, prefix, objectKey string) (string, bool) {
	if prefix == "" || !strings.HasPrefix(objectKey, prefix) {
		return "", false
	}
	relative, ok := utils.SanitizePath(strings.TrimPrefix(objectKey, prefix))
	if !ok || relative == "" {
		return "", false
	}
	localPath := filepath.Join(repoDir, filepath.FromSlash(relative))
	if !utils.IsSubPath(repoDir, localPath) {
		return "", false
	}
	return localPath, true
}

func SaveAndUploadChecksum(state *core.AppState, basePath string, ext string, hash string) error {
	checksumPath := basePath + ext
	if IsS3Enabled(basePath) {
		if err := UploadChecksumS3(checksumPath, hash); err != nil {
			return err
		}
	} else {
		err := os.WriteFile(checksumPath, []byte(hash), 0644)
		if err != nil {
			return err
		}
	}
	state.Inner.FileIndex.InsertFile(checksumPath, index.FileInfo{
		Size:    int64(len(hash)),
		ModTime: time.Now().UnixNano(),
	})
	return nil
}

func UploadChecksumS3(path string, hash string) error {
	s3Key := filepath.ToSlash(path)
	s3Key = strings.TrimPrefix(s3Key, "./")
	s3Key = strings.TrimPrefix(s3Key, "/")
	return UploadStreamToS3(s3Key, strings.NewReader(hash), int64(len(hash)), "text/plain")
}
