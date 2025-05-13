package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
)

type envFileSystem struct {
	fs http.FileSystem
}

type envFile struct {
	*bytes.Reader
	file         http.File
	info         os.FileInfo
	replacedSize int64
}

type envFileInfo struct {
	os.FileInfo
	size int64
}

func (e envFileInfo) Size() int64 {
	return e.size
}

func (f *envFile) Close() error {
	return f.file.Close()
}

func (f *envFile) Stat() (os.FileInfo, error) {
	return envFileInfo{FileInfo: f.info, size: f.replacedSize}, nil
}

func (f *envFile) Readdir(count int) ([]os.FileInfo, error) {
	return f.file.Readdir(count)
}

var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:=([^}]*))?}`)

func replaceEnvVars(content string) string {
	return envVarPattern.ReplaceAllStringFunc(content, func(match string) string {
		groups := envVarPattern.FindStringSubmatch(match)
		name := groups[1]
		def := groups[3]
		val, ok := os.LookupEnv(name)
		if !ok {
			if def != "" || groups[2] == ":=" {
				return def
			}
			return match
		}
		return val
	})
}

func (e envFileSystem) Open(name string) (http.File, error) {
	file, err := e.fs.Open(name)
	if err != nil {
		return nil, err
	}
	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if stat.IsDir() {
		return file, nil
	}
	data, err := io.ReadAll(file)
	if err != nil {
		file.Close()
		return nil, err
	}
	newContent := replaceEnvVars(string(data))
	replacedSize := int64(len(newContent))
	return &envFile{Reader: bytes.NewReader([]byte(newContent)), file: file, info: stat, replacedSize: replacedSize}, nil
}

func checkEnvVarsInFiles(root string) error {
	missing := map[string]struct{}{}
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		matches := envVarPattern.FindAllStringSubmatch(string(data), -1)
		for _, match := range matches {
			name := match[1]
			def := match[3]
			if _, ok := os.LookupEnv(name); !ok {
				if def == "" && match[2] != ":=" {
					missing[name] = struct{}{}
				}
			}
		}
		return nil
	})
	if len(missing) > 0 {
		return fmt.Errorf("missing environment variables: %v", keys(missing))
	}
	return nil
}

func keys(m map[string]struct{}) []string {
	var k []string
	for key := range m {
		k = append(k, key)
	}
	return k
}
