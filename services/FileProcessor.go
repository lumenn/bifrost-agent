package services

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/alexmullins/zip"
)

// DownloadFile downloads a file from URL to the specified filepath
func DownloadFile(url string, filepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// UnzipFile extracts a ZIP file to the specified directory, optionally using a password
func UnzipFile(zipPath, destDir string, password *string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer reader.Close()

	for _, file := range reader.File {
		if password != nil && file.IsEncrypted() {
			file.SetPassword(*password)
		}

		path := filepath.Join(destDir, file.Name)

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(path, os.ModePerm); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
			return fmt.Errorf("failed to create parent directory: %w", err)
		}

		dstFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return fmt.Errorf("failed to create destination file: %w", err)
		}

		srcFile, err := file.Open()
		if err != nil {
			dstFile.Close()
			return fmt.Errorf("failed to open zip file entry: %w", err)
		}

		_, err = io.Copy(dstFile, srcFile)
		dstFile.Close()
		srcFile.Close()

		if err != nil {
			return fmt.Errorf("failed to extract file: %w", err)
		}
	}
	return nil
}

// ListFiles returns a list of non-directory files in the specified directory
func ListFiles(dir string, excludeExt ...string) ([]string, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var filePaths []string
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		ext := filepath.Ext(file.Name())
		exclude := false
		for _, excludedExt := range excludeExt {
			if ext == excludedExt {
				exclude = true
				break
			}
		}

		if !exclude {
			filePaths = append(filePaths, filepath.Join(dir, file.Name()))
		}
	}

	return filePaths, nil
}

// FileExists checks if a file exists at the given path
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
