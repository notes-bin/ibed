package storage

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

type Storage struct {
	uploadDir string
}

func NewStorage(uploadDir string) (*Storage, error) {
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return nil, err
	}
	return &Storage{uploadDir: uploadDir}, nil
}

func (s *Storage) SaveFile(file io.Reader, path string) error {
	out, err := os.Create(path)
	if err != nil {
		slog.Error("Failed to create file", "path", path, "error", err)
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, file)
	if err != nil {
		slog.Error("Failed to save file", "path", path, "error", err)
	}
	return err
}

func (s *Storage) DeleteFile(path string) error {
	return os.Remove(path)
}

func (s *Storage) GetFilePath(filename string) string {
	return filepath.Join(s.uploadDir, filename)
}
