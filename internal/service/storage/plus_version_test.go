package storage

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/index"
	"renop/internal/utils"
)

func TestPlusVersionUploadAndDownload(t *testing.T) {
	storagePath := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	InitS3(&cfg)

	state := core.NewAppState()
	state.Inner.Config.Store(&cfg)
	state.Inner.FileIndex = index.NewFileIndex()
	repo := cfg.Maven.Repositories["releases"]
	repo.AllowRedeployment = true

	app := fiber.New(fiber.Config{StreamRequestBody: true, UnescapePath: false})
	app.Put("/:repo_name/*", func(c fiber.Ctx) error {
		path, ok := utils.SanitizePath(c.Params("*"))
		if !ok {
			return c.SendStatus(fiber.StatusBadRequest)
		}
		localPath := filepath.Join(storagePath, c.Params("repo_name"), path)
		return HandlePut(c, state, repo, localPath)
	})
	app.Get("/:repo_name/*", func(c fiber.Ctx) error {
		return HandleGet(c, state, repo, storagePath)
	})

	want := []byte("plus-version-content")
	urls := []string{
		"/releases/com/example/mod/1.2.0+26.1/mod-1.2.0+26.1.jar",
		"/releases/com/example/mod/1.2.0%2B26.1/mod-1.2.0%2B26.1.jar",
	}

	req := httptest.NewRequest(http.MethodPut, urls[0], bytes.NewReader(want))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("PUT bare+ returned %d", resp.StatusCode)
	}

	onDisk := filepath.Join(storagePath, "releases", "com", "example", "mod", "1.2.0+26.1", "mod-1.2.0+26.1.jar")
	got, err := os.ReadFile(onDisk)
	if err != nil {
		t.Fatalf("file on disk: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("disk content mismatch")
	}

	wrong := filepath.Join(storagePath, "releases", "com", "example", "mod", "1.2.0 26.1", "mod-1.2.0 26.1.jar")
	if _, err := os.Stat(wrong); err == nil {
		t.Fatal("artifact was incorrectly stored with '+' converted to space")
	}

	for _, u := range urls {
		getResp, err := app.Test(httptest.NewRequest(http.MethodGet, u, nil))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(getResp.Body)
		getResp.Body.Close()
		if getResp.StatusCode != fiber.StatusOK || !bytes.Equal(body, want) {
			t.Fatalf("GET %s -> status %d body %q", u, getResp.StatusCode, body)
		}
	}
}
