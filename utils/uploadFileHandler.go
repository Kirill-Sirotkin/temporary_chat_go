package utils

import (
	"errors"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

func UploadFile(file *multipart.FileHeader) (string, error) {
	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	if file.Size > 2000000 {
		return "", errors.New("file size exceeds 2MB")
	}

	fileName := strings.TrimSuffix(file.Filename, filepath.Ext(file.Filename))
	id := uuid.New()
	fileSaveDir := filepath.Join("uploads", fileName+id.String()+filepath.Ext(file.Filename))

	dst, err := os.Create(fileSaveDir)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err = io.Copy(dst, src); err != nil {
		return "", err
	}

	return fileSaveDir, nil
}

func GetOriginalFilePath(file string) string {
	fileName := strings.TrimSuffix(file, filepath.Ext(file))
	originalName := fileName[:len(fileName)-36]

	return originalName + filepath.Ext(file)
}
