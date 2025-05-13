package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type HeaderConfigArray struct {
	Configs []HeaderConfig `json:"configs"`
}

type HeaderConfig struct {
	Path          string            `json:"path"`
	FileExtension string            `json:"fileExtension"`
	Headers       []HeaderDefiniton `json:"headers"`
}

type HeaderDefiniton struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

var headerConfigs HeaderConfigArray

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func initHeaderConfig(headerConfigPath string) bool {
	headerConfigValid := false
	if fileExists(headerConfigPath) {
		jsonFile, err := os.Open(headerConfigPath)
		if err == nil {
			byteValue, _ := io.ReadAll(jsonFile)
			json.Unmarshal(byteValue, &headerConfigs)
			if len(headerConfigs.Configs) > 0 {
				headerConfigValid = true
			}
			jsonFile.Close()
		}
	}
	return headerConfigValid
}

func customHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqFileExtension := filepath.Ext(r.URL.Path)
		for i := 0; i < len(headerConfigs.Configs); i++ {
			configEntry := headerConfigs.Configs[i]
			fileMatch := configEntry.FileExtension == "*" || reqFileExtension == "."+configEntry.FileExtension
			pathMatch := configEntry.Path == "*" || strings.HasPrefix(r.URL.Path, configEntry.Path)
			if fileMatch && pathMatch {
				for j := 0; j < len(configEntry.Headers); j++ {
					headerEntry := configEntry.Headers[j]
					w.Header().Set(headerEntry.Key, headerEntry.Value)
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
