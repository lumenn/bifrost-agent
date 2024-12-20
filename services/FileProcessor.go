package services

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

// Add this function to check if unzip is available
func init() {
	_, err := exec.LookPath("unzip")
	if err != nil {
		log.Fatal("unzip command not found. Please install unzip to use this application.")
	}
}

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
	log.Printf("[INFO] Unzipping %s to %s", zipPath, destDir)

	// Ensure destination directory exists
	if err := os.MkdirAll(destDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	args := []string{"-o", zipPath, "-d", destDir}
	if password != nil {
		args = append([]string{"-P", *password}, args...)
	}

	cmd := exec.Command("unzip", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[ERROR] Unzip command failed: %s", string(output))
		return fmt.Errorf("failed to unzip file: %w", err)
	}

	log.Printf("[INFO] Successfully unzipped %s", zipPath)
	return nil
}

// ListFiles returns a list of non-directory files in the specified directory
func ListFiles(dir string, excludeExt ...string) ([]string, error) {
	log.Printf("[DEBUG] Listing files in directory: %s", dir)

	files, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("[ERROR] Failed to read directory %s: %v", dir, err)
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
