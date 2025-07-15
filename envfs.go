package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type EnvFileSystem struct {
	fs http.FileSystem
}

type EnvFile struct {
	*bytes.Reader
	file         http.File
	info         os.FileInfo
	replacedSize int64
}

type EnvFileInfo struct {
	os.FileInfo
	size int64
}

func (e EnvFileInfo) Size() int64 {
	return e.size
}

func (f *EnvFile) Close() error {
	if f.file == nil {
		return nil
	}
	return f.file.Close()
}

func (f *EnvFile) Stat() (os.FileInfo, error) {
	return EnvFileInfo{FileInfo: f.info, size: f.replacedSize}, nil
}

func (f *EnvFile) Readdir(count int) ([]os.FileInfo, error) {
	if f.file == nil {
		return nil, fmt.Errorf("file is closed")
	}
	return f.file.Readdir(count)
}

var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:=([^}]*))?}`)

var fileExtensions = map[string]bool{
	"html": true, "js": true, "css": true, "json": true, "txt": true,
	"md": true, "xml": true, "yml": true, "yaml": true, "log": true, "bak": true,
}

func replaceEnvVars(content string) string {
	return envVarPattern.ReplaceAllStringFunc(content, func(match string) string {
		groups := envVarPattern.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}

		varName := groups[1]
		defaultValue := ""
		hasDefault := len(groups) > 3 && groups[3] != ""

		if hasDefault {
			defaultValue = groups[3]
		}

		if value, exists := os.LookupEnv(varName); exists {
			return value
		}

		if hasDefault || (len(groups) > 2 && groups[2] == ":=") {
			return defaultValue
		}

		return match
	})
}

func (e EnvFileSystem) Open(name string) (http.File, error) {
	file, err := e.fs.Open(name)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", name, err)
	}

	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat file %s: %w", name, err)
	}

	if stat.IsDir() {
		return file, nil
	}

	data, err := io.ReadAll(file)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to read file %s: %w", name, err)
	}

	processedContent := replaceEnvVars(string(data))

	return &EnvFile{
		Reader:       bytes.NewReader([]byte(processedContent)),
		file:         file,
		info:         stat,
		replacedSize: int64(len(processedContent)),
	}, nil
}

func parsePatterns(patterns string) []string {
	if patterns == "" {
		return nil
	}

	parts := strings.Split(strings.TrimSpace(patterns), ",")
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func matchPattern(path, pattern string, isInclude bool) bool {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))

	if path == pattern {
		return true
	}

	if strings.ContainsAny(pattern, "*?[]") {
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}

		// For path-based patterns like docs/v*, check directory matching
		if strings.Contains(pattern, "/") {
			dir := filepath.Dir(path)
			if matched, _ := filepath.Match(pattern, dir); matched {
				return true
			}

			if isInclude {
				patternDir := filepath.Dir(pattern)
				if patternDir != "." && (path == patternDir || strings.HasPrefix(path, patternDir+"/") || strings.HasPrefix(patternDir, path+"/")) {
					return true
				}
			}
		} else {
			pathParts := strings.Split(path, "/")
			for _, part := range pathParts {
				if matched, _ := filepath.Match(pattern, part); matched {
					return true
				}
			}
		}
	} else {
		if strings.HasPrefix(path, pattern+"/") {
			return true
		}

		if isInclude && strings.HasPrefix(pattern, path+"/") {
			return true
		}

		pathParts := strings.Split(path, "/")
		for _, part := range pathParts {
			if part == pattern {
				return true
			}
		}
	}

	return false
}

func shouldInclude(path string, includePatterns, excludePatterns []string, isFile bool) bool {
	for _, pattern := range excludePatterns {
		if matchPattern(path, pattern, false) {
			return false
		}
	}

	if len(includePatterns) == 0 {
		return true
	}

	for _, pattern := range includePatterns {
		if isFile && isFilePattern(pattern) {
			if matchesFilePattern(path, pattern) {
				return true
			}
		} else {
			if matchPattern(path, pattern, true) {
				return true
			}
		}
	}

	return false
}

func isFilePattern(pattern string) bool {
	if strings.HasPrefix(pattern, "*.") && !strings.Contains(pattern, "/") {
		ext := pattern[2:]
		return fileExtensions[ext]
	}
	return strings.Contains(pattern, "/") && strings.Contains(pattern, "*") && strings.Contains(pattern, ".")
}

func matchesFilePattern(filePath, pattern string) bool {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))

	if strings.HasPrefix(pattern, "*.") && !strings.Contains(pattern, "/") {
		ext := pattern[2:]
		if fileExtensions[ext] {
			filename := filepath.Base(filePath)
			matched, _ := filepath.Match(pattern, filename)
			return matched
		}
	}

	if strings.Contains(pattern, "/") && strings.Contains(pattern, ".") {
		matched, _ := filepath.Match(pattern, filePath)
		return matched
	}

	dir := filepath.Dir(filePath)
	return matchPattern(dir, pattern, true)
}

func hasFilePatterns(patterns []string) bool {
	for _, pattern := range patterns {
		if isFilePattern(pattern) {
			return true
		}
	}
	return false
}

func checkEnvVarsInFiles(root, includeDirs, excludeDirs string) error {
	if root == "" {
		return fmt.Errorf("root path cannot be empty")
	}

	includePatterns := parsePatterns(includeDirs)
	excludePatterns := parsePatterns(excludeDirs)
	hasFilePats := hasFilePatterns(includePatterns)
	missing := make(map[string]struct{})

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		relPath = filepath.ToSlash(relPath)

		if info.IsDir() {
			if relPath == "." {
				return nil
			}

			if hasFilePats {
				if !shouldInclude(relPath, nil, excludePatterns, false) {
					return filepath.SkipDir
				}
				return nil
			}

			if !shouldInclude(relPath, includePatterns, excludePatterns, false) {
				return filepath.SkipDir
			}
			return nil
		}

		if !shouldInclude(relPath, includePatterns, excludePatterns, true) {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		matches := envVarPattern.FindAllStringSubmatch(string(data), -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}

			varName := match[1]
			defaultValue := ""
			hasDefault := len(match) > 3 && match[3] != ""

			if hasDefault {
				defaultValue = match[3]
			}

			if _, exists := os.LookupEnv(varName); !exists {
				if defaultValue == "" && (len(match) <= 2 || match[2] != ":=") {
					missing[varName] = struct{}{}
				}
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("error walking directory tree: %w", err)
	}

	if len(missing) > 0 {
		keys := make([]string, 0, len(missing))
		for key := range missing {
			keys = append(keys, key)
		}
		return fmt.Errorf("missing environment variables: %v", keys)
	}

	return nil
}
