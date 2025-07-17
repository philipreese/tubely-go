package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (cfg apiConfig) ensureAssetsDir() error {
	if _, err := os.Stat(cfg.assetsRoot); os.IsNotExist(err) {
		return os.Mkdir(cfg.assetsRoot, 0755)
	}
	return nil
}

func getAssetPath(mediaType string) string {
	randomBytes := make([]byte, 32)
	rand.Read(randomBytes)
	fileName := base64.RawURLEncoding.EncodeToString(randomBytes)
	fileExt := mediaTypeToExt(mediaType)
	return fmt.Sprintf("%s%s", fileName, fileExt)
}

func (cfg apiConfig) getAssetDiskPath(assetPath string) string {
	return filepath.Join(cfg.assetsRoot, assetPath)
}

func (cfg apiConfig) getAssetURL(assetPath string) string {
	return fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, assetPath)
}

func (cfg apiConfig) getObjectURL(key string) string {
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, key)
}

func mediaTypeToExt(mediaType string) string {
	result := strings.Split(mediaType, "/")
	if len(result) != 2 {
		return ".bin"
	}

	return "." + result[1]
}
