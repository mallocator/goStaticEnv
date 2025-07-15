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

func checkEnvVarsInFiles(root string, includeDirs string, excludeDirs string) error {
	missing := map[string]struct{}{}

	var includePatterns []string
	var excludePatterns []string

	if includeDirs != "" {
		includePatterns = strings.Split(strings.TrimSpace(includeDirs), ",")
		for i := range includePatterns {
			includePatterns[i] = strings.TrimSpace(includePatterns[i])
		}
	}

	if excludeDirs != "" {
		excludePatterns = strings.Split(strings.TrimSpace(excludeDirs), ",")
		for i := range excludePatterns {
			excludePatterns[i] = strings.TrimSpace(excludePatterns[i])
		}
	}

	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}

		// Normalize path separators for consistent matching
		relPath = filepath.ToSlash(relPath)

		// Skip directories but check if we should process files in them
		if info.IsDir() {
			// Skip the root directory itself
			if relPath == "." {
				return nil
			}

			// Check exclude patterns first
			if shouldExcludeDir(relPath, excludePatterns) {
				return filepath.SkipDir
			}

			// If include patterns are specified, check if this directory should be included
			if len(includePatterns) > 0 && !shouldIncludeDir(relPath, includePatterns) {
				return filepath.SkipDir
			}

			return nil
		}

		// Process files - but first check if this file should be processed based on include/exclude patterns
		// Get the directory containing this file
		fileDir := filepath.Dir(relPath)

		// If include patterns are specified, check if this file's directory is included
		if len(includePatterns) > 0 {
			if fileDir == "." {
				// File is in root directory - only process if root is explicitly included
				rootIncluded := false
				for _, pattern := range includePatterns {
					if pattern == "." || pattern == "" || pattern == "/" {
						rootIncluded = true
						break
					}
				}
				if !rootIncluded {
					return nil
				}
			} else {
				// File is in a subdirectory - check if that directory should be included
				if !shouldIncludeDir(fileDir, includePatterns) {
					return nil
				}
			}
		}

		// Check if this file's directory should be excluded
		if len(excludePatterns) > 0 && fileDir != "." {
			if shouldExcludeDir(fileDir, excludePatterns) {
				return nil
			}
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

// shouldExcludeDir checks if a directory path matches any exclude patterns
func shouldExcludeDir(dirPath string, excludePatterns []string) bool {
	for _, pattern := range excludePatterns {
		if matchesPattern(dirPath, pattern) {
			return true
		}
	}
	return false
}

// shouldIncludeDir checks if a directory path matches any include patterns
func shouldIncludeDir(dirPath string, includePatterns []string) bool {
	for _, pattern := range includePatterns {
		if matchesPatternForInclude(dirPath, pattern) {
			return true
		}
	}
	return false
}

// matchesPattern checks if a directory path matches a pattern
// Supports glob patterns using filepath.Match() and intuitive prefix matching
func matchesPattern(dirPath string, pattern string) bool {
	// Normalize pattern
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))

	// Check if pattern contains glob characters
	hasGlobChars := strings.ContainsAny(pattern, "*?[]")

	if hasGlobChars {
		// Use filepath.Match for glob patterns
		// Check exact match first
		if matched, _ := filepath.Match(pattern, dirPath); matched {
			return true
		}

		// For path-based patterns like "docs/v*", check if the directory path matches
		if strings.Contains(pattern, "/") {
			// This is a path-based pattern, match the full path
			if matched, _ := filepath.Match(pattern, dirPath); matched {
				return true
			}
			// For include logic: check if this is a parent directory that should be traversed
			// But only if we're checking includes, not excludes
			// We'll handle this differently in shouldIncludeDir vs shouldExcludeDir
		} else {
			// For simple patterns like "test-*", check individual directory components
			pathParts := strings.Split(dirPath, "/")
			for _, part := range pathParts {
				if matched, _ := filepath.Match(pattern, part); matched {
					return true
				}
			}

			// Also check if any parent directory matches
			for i := range pathParts {
				parentPath := strings.Join(pathParts[:i+1], "/")
				if matched, _ := filepath.Match(pattern, parentPath); matched {
					return true
				}
			}
		}
	} else {
		// For non-glob patterns, use intuitive prefix/exact matching

		// Exact match
		if dirPath == pattern {
			return true
		}

		// Check if dirPath is a subdirectory of pattern (prefix match)
		if strings.HasPrefix(dirPath, pattern+"/") {
			return true
		}

		// Check if pattern is a subdirectory of dirPath (for include logic)
		if strings.HasPrefix(pattern, dirPath+"/") {
			return true
		}

		// Check if any part of the path matches the pattern (for excluding specific directory names anywhere)
		pathParts := strings.Split(dirPath, "/")
		for _, part := range pathParts {
			if part == pattern {
				return true
			}
		}
	}

	return false
}

// matchesPatternForInclude handles include logic with parent directory traversal
func matchesPatternForInclude(dirPath string, pattern string) bool {
	// First check if it matches normally
	if matchesPattern(dirPath, pattern) {
		return true
	}

	// For path-based include patterns, also allow parent directories to be traversed
	if strings.ContainsAny(pattern, "*?[]") && strings.Contains(pattern, "/") {
		// If this is a parent directory of the pattern, include it so we can traverse into it
		if strings.HasPrefix(pattern, dirPath+"/") {
			return true
		}
	}

	return false
}

func keys(m map[string]struct{}) []string {
	var k []string
	for key := range m {
		k = append(k, key)
	}
	return k
}
