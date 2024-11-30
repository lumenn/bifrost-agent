package services

import (
	"archive/zip"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type FileProcessor struct {
	downloadURL string
	workDir     string
}

func NewFileProcessor(downloadURL, workDir string) *FileProcessor {
	return &FileProcessor{
		downloadURL: downloadURL,
		workDir:     workDir,
	}
}

func (fp *FileProcessor) ProcessFiles() ([]string, error) {
	// Download and extract files if needed
	zipPath := filepath.Join(fp.workDir, "files.zip")
	if !fileExists(zipPath) {
		if err := fp.downloadFile(zipPath); err != nil {
			return nil, err
		}
	}

	if err := fp.unzipFile(zipPath); err != nil {
		return nil, err
	}

	// Get list of files
	files, err := os.ReadDir(fp.workDir)
	if err != nil {
		return nil, err
	}

	var filePaths []string
	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) == ".zip" {
			continue
		}
		filePaths = append(filePaths, filepath.Join(fp.workDir, file.Name()))
	}

	return filePaths, nil
}

func (fp *FileProcessor) downloadFile(filepath string) error {
	resp, err := http.Get(fp.downloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func (fp *FileProcessor) unzipFile(zipPath string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		path := filepath.Join(fp.workDir, file.Name)

		if file.FileInfo().IsDir() {
			os.MkdirAll(path, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
			return err
		}

		dstFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return err
		}

		srcFile, err := file.Open()
		if err != nil {
			dstFile.Close()
			return err
		}

		_, err = io.Copy(dstFile, srcFile)
		dstFile.Close()
		srcFile.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
