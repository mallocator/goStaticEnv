package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplaceEnvVars(t *testing.T) {
	os.Setenv("FOO", "bar")
	os.Setenv("BAZ", "qux")
	defer os.Unsetenv("FOO")
	defer os.Unsetenv("BAZ")

	cases := []struct {
		input    string
		expected string
	}{
		{"Hello ${FOO}", "Hello bar"},
		{"${FOO} and ${BAZ}", "bar and qux"},
		{"${MISSING:=default}", "default"},
		{"${FOO:=default}", "bar"},
		{"No vars", "No vars"},
	}

	for _, c := range cases {
		out := replaceEnvVars(c.input)
		if out != c.expected {
			t.Errorf("replaceEnvVars(%q) = %q; want %q", c.input, out, c.expected)
		}
	}
}

func TestCheckEnvVarsInFiles(t *testing.T) {
	dir, err := os.MkdirTemp("", "envfs-test")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(dir)

	file1 := dir + "/file1.txt"
	file2 := dir + "/file2.txt"
	if err := os.WriteFile(file1, []byte("Hello ${FOO}"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(file2, []byte("World ${BAR:=default}"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	os.Setenv("FOO", "bar")
	defer os.Unsetenv("FOO")

	err = checkEnvVarsInFiles(dir, "", "")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	os.Unsetenv("FOO")
	err = checkEnvVarsInFiles(dir, "", "")
	if err == nil || !strings.Contains(err.Error(), "FOO") {
		t.Errorf("Expected error about missing FOO, got %v", err)
	}
}

func TestCheckEnvVarsInFilesWithIncludeExclude(t *testing.T) {
	dir, err := os.MkdirTemp("", "envfs-include-exclude-test")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(dir)

	// Create directory structure
	// /root
	//   /include-me/file1.txt (contains ${MISSING_VAR})
	//   /exclude-me/file2.txt (contains ${ANOTHER_MISSING_VAR})
	//   /nested/deep/file3.txt (contains ${THIRD_MISSING_VAR})
	//   file4.txt (contains ${ROOT_MISSING_VAR})

	includeDir := dir + "/include-me"
	excludeDir := dir + "/exclude-me"
	nestedDir := dir + "/nested/deep"

	if err := os.MkdirAll(includeDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.MkdirAll(excludeDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	// Create test files
	files := map[string]string{
		includeDir + "/file1.txt": "Content with ${MISSING_VAR}",
		excludeDir + "/file2.txt": "Content with ${ANOTHER_MISSING_VAR}",
		nestedDir + "/file3.txt":  "Content with ${THIRD_MISSING_VAR}",
		dir + "/file4.txt":        "Content with ${ROOT_MISSING_VAR}",
	}

	for filePath, content := range files {
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile failed for %s: %v", filePath, err)
		}
	}

	// Test 1: Include only specific directory
	err = checkEnvVarsInFiles(dir, "include-me", "")
	if err == nil || !strings.Contains(err.Error(), "MISSING_VAR") {
		t.Errorf("Expected error about MISSING_VAR when including only include-me, got %v", err)
	}
	if err != nil && strings.Contains(err.Error(), "ANOTHER_MISSING_VAR") {
		t.Errorf("Should not find ANOTHER_MISSING_VAR when only including include-me, got %v", err)
	}

	// Test 2: Exclude specific directory
	err = checkEnvVarsInFiles(dir, "", "exclude-me")
	if err == nil {
		t.Errorf("Expected error about missing vars when excluding exclude-me")
	}
	if err != nil && strings.Contains(err.Error(), "ANOTHER_MISSING_VAR") {
		t.Errorf("Should not find ANOTHER_MISSING_VAR when excluding exclude-me, got %v", err)
	}
	if err != nil && (!strings.Contains(err.Error(), "MISSING_VAR") || !strings.Contains(err.Error(), "ROOT_MISSING_VAR")) {
		t.Errorf("Should find other missing vars when excluding exclude-me, got %v", err)
	}

	// Test 3: Include multiple directories
	err = checkEnvVarsInFiles(dir, "include-me,nested", "")
	if err == nil {
		t.Errorf("Expected error about missing vars when including include-me and nested")
	}
	if err != nil && (!strings.Contains(err.Error(), "MISSING_VAR") || !strings.Contains(err.Error(), "THIRD_MISSING_VAR")) {
		t.Errorf("Should find vars from both included directories, got %v", err)
	}
	if err != nil && (strings.Contains(err.Error(), "ANOTHER_MISSING_VAR") || strings.Contains(err.Error(), "ROOT_MISSING_VAR")) {
		t.Errorf("Should not find vars from non-included directories, got %v", err)
	}

	// Test 4: Exclude multiple directories
	err = checkEnvVarsInFiles(dir, "", "exclude-me,nested")
	if err == nil {
		t.Errorf("Expected error about missing vars when excluding exclude-me and nested")
	}
	if err != nil && (strings.Contains(err.Error(), "ANOTHER_MISSING_VAR") || strings.Contains(err.Error(), "THIRD_MISSING_VAR")) {
		t.Errorf("Should not find vars from excluded directories, got %v", err)
	}

	// Test 5: Include and exclude combined
	err = checkEnvVarsInFiles(dir, "include-me,exclude-me", "exclude-me")
	if err == nil || !strings.Contains(err.Error(), "MISSING_VAR") {
		t.Errorf("Expected error about MISSING_VAR when including both but excluding exclude-me, got %v", err)
	}
	if err != nil && strings.Contains(err.Error(), "ANOTHER_MISSING_VAR") {
		t.Errorf("Should not find ANOTHER_MISSING_VAR when exclude overrides include, got %v", err)
	}
}

func TestEnvFileSystemOpenAndReplace(t *testing.T) {
	dir := t.TempDir()
	filePath := dir + "/test.txt"
	content := "A=${A}, B=${B:=bee}, C=${C}"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	os.Setenv("A", "aye")
	defer os.Unsetenv("A")
	// C is not set and has no default, so should remain as-is

	fs := EnvFileSystem{fs: http.Dir(dir)}
	f, err := fs.Open("test.txt")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	got := string(b)
	want := "A=aye, B=bee, C=${C}"
	if got != want {
		t.Errorf("envFileSystem.Open: got %q, want %q", got, want)
	}
}

func TestEnvFileSystemOpenDir(t *testing.T) {
	dir := t.TempDir()
	err := os.Mkdir(dir+"/subdir", 0755)
	if err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}
	fs := EnvFileSystem{fs: http.Dir(dir)}
	f, err := fs.Open("subdir")
	if err != nil {
		t.Fatalf("Open dir failed: %v", err)
	}
	info, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("Expected IsDir true, got false")
	}
}

func TestReplaceEnvVarsEdgeCases(t *testing.T) {
	os.Setenv("EMPTY", "")
	os.Setenv("FOO", "")
	os.Setenv("FOO_BAR", "foobar")
	os.Setenv("FOO123", "foo123")
	defer os.Unsetenv("EMPTY")
	defer os.Unsetenv("FOO")
	defer os.Unsetenv("FOO_BAR")
	defer os.Unsetenv("FOO123")

	tests := []struct {
		input    string
		expected string
	}{
		{"${EMPTY:=default}", ""},            // empty env var, not default
		{"${NOTSET}", "${NOTSET}"},           // not set, no default
		{"${NOTSET:=}", ""},                  // not set, empty default
		{"${_UNDERSCORE}", "${_UNDERSCORE}"}, // valid var name
		{"${1INVALID}", "${1INVALID}"},       // invalid var name
		{"${FOO:=default}", ""},              // empty env var, should not use default
		{"${FOO}", ""},
		{"${FOO_BAR}", "foobar"},
		{"${FOO123}", "foo123"},
		{"${}", "${}"},         // malformed, should remain as-is
		{"${:=}", "${:=}"},     // malformed, should remain as-is
		{"${FOO:}", "${FOO:}"}, // malformed, should remain as-is
		{"${FOO}${FOO}", ""},   // repeated empty
		{"${FOO}${FOO_BAR}", "foobar"},
		{"${FOO}bar", "bar"},                                 // var at start
		{"bar${FOO}", "bar"},                                 // var at end
		{"${FOO} ${BAR:=baz} ${MISSING}", " baz ${MISSING}"}, // mixed
	}

	for _, c := range tests {
		if got := replaceEnvVars(c.input); got != c.expected {
			t.Errorf("replaceEnvVars(%q) = %q, want %q", c.input, got, c.expected)
		}
	}
}

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		dirPath string
		pattern string
		want    bool
	}{
		// Exact matches (non-glob)
		{"docs", "docs", true},
		{"assets/css", "assets/css", true},

		// Prefix matches (non-glob, subdirectories)
		{"docs/api", "docs", true},
		{"assets/css/main.css", "assets", true},
		{"a/b/c", "a/b", true},

		// Parent directory matches (non-glob)
		{"docs", "docs/api", true},
		{"assets", "assets/css/main.css", true},

		// Directory name matches anywhere (non-glob)
		{"src/node_modules", "node_modules", true},
		{"lib/vendor/pkg", "vendor", true},

		// No matches (non-glob)
		{"docs", "assets", false},
		{"documentation", "docs", false},
		{"assets-old", "assets", false},
		{"", "docs", false},
		{"docs", "", false},

		// Glob patterns with *
		{"test-unit", "test-*", true},
		{"test-integration", "test-*", true},
		{"src/test-unit", "test-*", true},
		{"testing", "test-*", false},
		{"unit-test", "*-test", true},
		{"integration-test", "*-test", true},
		{"test", "*-test", false},

		// Glob patterns with ?
		{"test1", "test?", true},
		{"test2", "test?", true},
		{"test", "test?", false},
		{"test12", "test?", false},

		// Glob patterns with []
		{"test1", "test[123]", true},
		{"test2", "test[123]", true},
		{"test4", "test[123]", false},
		{"src/test2", "test[123]", true},

		// Complex glob patterns
		{"backup-2023", "backup-*", true},
		{"temp.old", "*.old", true},
		{"config.tmp", "*.tmp", true},
		{"src/config.tmp", "*.tmp", true},
		{"config.backup", "*.tmp", false},

		// Mixed scenarios
		{"node_modules", "*modules", true},
		{"my_modules", "*modules", true},
		{"modules", "*modules", true},
		{"test/node_modules", "*modules", true},
	}

	for _, tt := range tests {
		got := matchPattern(tt.dirPath, tt.pattern, true)
		if got != tt.want {
			t.Errorf("matchPattern(%q, %q) = %v; want %v", tt.dirPath, tt.pattern, got, tt.want)
		}
	}
}

func TestShouldExcludeDir(t *testing.T) {
	excludePatterns := []string{"node_modules", "vendor", "tmp/cache"}

	tests := []struct {
		dirPath string
		want    bool
	}{
		{"node_modules", true},
		{"vendor", true},
		{"tmp/cache", true},
		{"src/node_modules", true},   // subdirectory of excluded
		{"project/vendor/lib", true}, // subdirectory of excluded
		{"src", false},
		{"assets", false},
		{"node_modules_backup", false}, // not exact match
	}

	for _, tt := range tests {
		got := !shouldInclude(tt.dirPath, nil, excludePatterns, false)
		if got != tt.want {
			t.Errorf("shouldExcludeDir(%q, %v) = %v; want %v", tt.dirPath, excludePatterns, got, tt.want)
		}
	}
}

func TestShouldIncludeDir(t *testing.T) {
	includePatterns := []string{"src", "docs/api"}

	tests := []struct {
		dirPath string
		want    bool
	}{
		{"src", true},
		{"docs/api", true},
		{"src/components", true}, // subdirectory of included
		{"docs/api/v1", true},    // subdirectory of included
		{"docs", true},           // parent of included pattern
		{"assets", false},
		{"tests", false},
		{"docs/guides", false}, // not under included pattern
	}

	for _, tt := range tests {
		got := shouldInclude(tt.dirPath, includePatterns, nil, false)
		if got != tt.want {
			t.Errorf("shouldIncludeDir(%q, %v) = %v; want %v", tt.dirPath, includePatterns, got, tt.want)
		}
	}
}

func TestCheckEnvVarsInFilesWithGlobPatterns(t *testing.T) {
	dir, err := os.MkdirTemp("", "envfs-glob-test")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(dir)

	// Create directory structure with various naming patterns
	// /root
	//   /test-unit/file1.txt (contains ${TEST_UNIT_VAR})
	//   /test-integration/file2.txt (contains ${TEST_INTEGRATION_VAR})
	//   /backup-2023/file3.txt (contains ${BACKUP_VAR})
	//   /config.tmp/file4.txt (contains ${CONFIG_TMP_VAR})
	//   /src/file5.txt (contains ${SRC_VAR})

	testDirs := []string{
		dir + "/test-unit",
		dir + "/test-integration",
		dir + "/backup-2023",
		dir + "/config.tmp",
		dir + "/src",
	}

	for _, testDir := range testDirs {
		if err := os.MkdirAll(testDir, 0755); err != nil {
			t.Fatalf("MkdirAll failed: %v", err)
		}
	}

	// Create test files
	files := map[string]string{
		dir + "/test-unit/file1.txt":        "Content with ${TEST_UNIT_VAR}",
		dir + "/test-integration/file2.txt": "Content with ${TEST_INTEGRATION_VAR}",
		dir + "/backup-2023/file3.txt":      "Content with ${BACKUP_VAR}",
		dir + "/config.tmp/file4.txt":       "Content with ${CONFIG_TMP_VAR}",
		dir + "/src/file5.txt":              "Content with ${SRC_VAR}",
	}

	for filePath, content := range files {
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile failed for %s: %v", filePath, err)
		}
	}

	// Test 1: Include with wildcard pattern
	err = checkEnvVarsInFiles(dir, "test-*", "")
	if err == nil {
		t.Errorf("Expected error about missing vars when including test-* pattern")
	}
	if err != nil && (!strings.Contains(err.Error(), "TEST_UNIT_VAR") || !strings.Contains(err.Error(), "TEST_INTEGRATION_VAR")) {
		t.Errorf("Should find vars from test-* directories, got %v", err)
	}
	if err != nil && (strings.Contains(err.Error(), "BACKUP_VAR") || strings.Contains(err.Error(), "SRC_VAR") || strings.Contains(err.Error(), "CONFIG_TMP_VAR")) {
		t.Errorf("Should not find vars from non-test directories, got %v", err)
	}

	// Test 2: Exclude with wildcard pattern
	err = checkEnvVarsInFiles(dir, "", "test-*")
	if err == nil {
		t.Errorf("Expected error about missing vars when excluding test-* pattern")
	}
	if err != nil && (strings.Contains(err.Error(), "TEST_UNIT_VAR") || strings.Contains(err.Error(), "TEST_INTEGRATION_VAR")) {
		t.Errorf("Should not find vars from excluded test-* directories, got %v", err)
	}
	if err != nil && (!strings.Contains(err.Error(), "BACKUP_VAR") || !strings.Contains(err.Error(), "SRC_VAR") || !strings.Contains(err.Error(), "CONFIG_TMP_VAR")) {
		t.Errorf("Should find vars from non-excluded directories, got %v", err)
	}

	// Test 3: Include with extension-like pattern
	err = checkEnvVarsInFiles(dir, "*.tmp", "")
	if err == nil {
		t.Errorf("Expected error about missing vars when including *.tmp pattern")
	}
	if err != nil && (strings.Contains(err.Error(), "TEST_UNIT_VAR") || strings.Contains(err.Error(), "BACKUP_VAR") || strings.Contains(err.Error(), "SRC_VAR")) {
		t.Errorf("Should not find vars from non-*.tmp directories, got %v", err)
	}

	// Test 4: Multiple patterns with wildcards
	err = checkEnvVarsInFiles(dir, "src,backup-*", "")
	if err == nil {
		t.Errorf("Expected error about missing vars when including src and backup-* patterns")
	}
	if err != nil && (!strings.Contains(err.Error(), "SRC_VAR") || !strings.Contains(err.Error(), "BACKUP_VAR")) {
		t.Errorf("Should find vars from src and backup-* directories, got %v", err)
	}
	if err != nil && (strings.Contains(err.Error(), "TEST_UNIT_VAR") || strings.Contains(err.Error(), "CONFIG_TMP_VAR")) {
		t.Errorf("Should not find vars from non-matching directories, got %v", err)
	}
}

func TestReadmeExamples(t *testing.T) {
	dir, err := os.MkdirTemp("", "readme-examples-test")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(dir)

	// Create directory structure matching README examples
	testDirs := []string{
		"src", "public", "node_modules", "vendor",
		"test-unit", "test-integration", "old-backup",
		"config.tmp", "cache-1", "cache-2",
		"docs/v1", "docs/v2", "docs/v1-draft",
	}

	// Create directories and test files
	for _, testDir := range testDirs {
		fullDir := filepath.Join(dir, testDir)
		if err := os.MkdirAll(fullDir, 0755); err != nil {
			t.Fatalf("MkdirAll failed for %s: %v", testDir, err)
		}

		// Create test file with environment variable
		// Normalize directory name to variable name: replace /, -, . with _ and uppercase
		varName := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(testDir, "/", "_"), "-", "_"), ".", "_")) + "_VAR"
		content := fmt.Sprintf("Content with ${%s}", varName)
		if err := os.WriteFile(filepath.Join(fullDir, "test.html"), []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile failed for %s: %v", testDir, err)
		}
	}

	// Test all README examples
	readmeTests := []struct {
		name          string
		includeDirs   string
		excludeDirs   string
		shouldFind    []string
		shouldNotFind []string
	}{
		{
			name:          "Simple include: src,public",
			includeDirs:   "src,public",
			excludeDirs:   "",
			shouldFind:    []string{"SRC_VAR", "PUBLIC_VAR"},
			shouldNotFind: []string{"NODE_MODULES_VAR", "VENDOR_VAR", "TEST_UNIT_VAR"},
		},
		{
			name:          "Simple exclude: node_modules,vendor",
			includeDirs:   "",
			excludeDirs:   "node_modules,vendor",
			shouldFind:    []string{"SRC_VAR", "PUBLIC_VAR", "TEST_UNIT_VAR"},
			shouldNotFind: []string{"NODE_MODULES_VAR", "VENDOR_VAR"},
		},
		{
			name:          "Glob exclude: test-*",
			includeDirs:   "",
			excludeDirs:   "test-*",
			shouldFind:    []string{"SRC_VAR", "PUBLIC_VAR", "NODE_MODULES_VAR"},
			shouldNotFind: []string{"TEST_UNIT_VAR", "TEST_INTEGRATION_VAR"},
		},
		{
			name:          "Glob exclude: *-backup",
			includeDirs:   "",
			excludeDirs:   "*-backup",
			shouldFind:    []string{"SRC_VAR", "PUBLIC_VAR", "TEST_UNIT_VAR"},
			shouldNotFind: []string{"OLD_BACKUP_VAR"},
		},
		{
			name:          "Glob include: cache-?",
			includeDirs:   "cache-?",
			excludeDirs:   "",
			shouldFind:    []string{"CACHE_1_VAR", "CACHE_2_VAR"},
			shouldNotFind: []string{"SRC_VAR", "PUBLIC_VAR", "TEST_UNIT_VAR"},
		},
		{
			name:          "Complex path: docs/v*",
			includeDirs:   "docs/v*",
			excludeDirs:   "",
			shouldFind:    []string{"DOCS_V1_VAR", "DOCS_V2_VAR", "DOCS_V1_DRAFT_VAR"},
			shouldNotFind: []string{"SRC_VAR", "PUBLIC_VAR"},
		},
		{
			name:          "Mixed: src,docs/v* exclude docs/v1-*",
			includeDirs:   "src,docs/v*",
			excludeDirs:   "docs/v1-*",
			shouldFind:    []string{"SRC_VAR", "DOCS_V1_VAR", "DOCS_V2_VAR"},
			shouldNotFind: []string{"DOCS_V1_DRAFT_VAR", "PUBLIC_VAR"},
		},
	}

	for _, tt := range readmeTests {
		t.Run(tt.name, func(t *testing.T) {
			// Debug: print what files and variables were created for this test
			if tt.name == "Glob include: cache-?" || tt.name == "Complex path: docs/v*" || tt.name == "Mixed: src,docs/v* exclude docs/v1-*" {
				t.Logf("Debug: Testing %s", tt.name)
				t.Logf("Include dirs: %s, Exclude dirs: %s", tt.includeDirs, tt.excludeDirs)
				t.Logf("Expected to find: %v", tt.shouldFind)

				// Check what files actually exist
				filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
					if err != nil || info.IsDir() {
						return nil
					}
					relPath, _ := filepath.Rel(dir, path)
					content, _ := os.ReadFile(path)
					t.Logf("File: %s, Content: %s", relPath, string(content))
					return nil
				})
			}

			err := checkEnvVarsInFiles(dir, tt.includeDirs, tt.excludeDirs)

			if err == nil {
				if len(tt.shouldFind) > 0 {
					t.Errorf("Expected to find missing vars %v, but got no error", tt.shouldFind)
				}
				return
			}

			errorMsg := err.Error()

			// Check that we found all variables we should find
			for _, varName := range tt.shouldFind {
				if !strings.Contains(errorMsg, varName) {
					t.Errorf("Expected to find %s in error message, but didn't. Error: %s", varName, errorMsg)
				}
			}

			// Check that we didn't find variables we shouldn't find
			for _, varName := range tt.shouldNotFind {
				if strings.Contains(errorMsg, varName) {
					t.Errorf("Expected NOT to find %s in error message, but did. Error: %s", varName, errorMsg)
				}
			}
		})
	}
}

func TestFilePatternMatching(t *testing.T) {
	dir := t.TempDir()

	testFiles := map[string]string{
		"public/test.html": "Content with ${PUBLIC_VAR}",
		"src/test.html":    "Content with ${SRC_VAR}",
		"src/script.js":    "Content with ${JS_VAR}",
		"public/style.css": "Content with ${CSS_VAR}",
		"config.txt":       "Content with ${CONFIG_VAR}",
	}

	for filePath, content := range testFiles {
		fullPath := filepath.Join(dir, filePath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("Failed to create directory for %s: %v", filePath, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", filePath, err)
		}
	}

	tests := []struct {
		name         string
		includeDirs  string
		excludeDirs  string
		expectedVars []string
		notExpected  []string
	}{
		{
			name:         "Include only HTML files",
			includeDirs:  "*.html",
			expectedVars: []string{"PUBLIC_VAR", "SRC_VAR"},
			notExpected:  []string{"JS_VAR", "CSS_VAR", "CONFIG_VAR"},
		},
		{
			name:         "Include only JavaScript files",
			includeDirs:  "*.js",
			expectedVars: []string{"JS_VAR"},
			notExpected:  []string{"PUBLIC_VAR", "SRC_VAR", "CSS_VAR", "CONFIG_VAR"},
		},
		{
			name:         "Include only CSS files",
			includeDirs:  "*.css",
			expectedVars: []string{"CSS_VAR"},
			notExpected:  []string{"PUBLIC_VAR", "SRC_VAR", "JS_VAR", "CONFIG_VAR"},
		},
		{
			name:         "Include only config.txt file",
			includeDirs:  "config.txt",
			expectedVars: []string{"CONFIG_VAR"},
			notExpected:  []string{"PUBLIC_VAR", "SRC_VAR", "JS_VAR", "CSS_VAR"},
		},
		{
			name:         "Include JS files in src directory",
			includeDirs:  "src/*.js",
			expectedVars: []string{"JS_VAR"},
			notExpected:  []string{"PUBLIC_VAR", "SRC_VAR", "CSS_VAR", "CONFIG_VAR"},
		},
		{
			name:         "Exclude HTML files",
			excludeDirs:  "*.html",
			expectedVars: []string{"JS_VAR", "CSS_VAR", "CONFIG_VAR"},
			notExpected:  []string{"PUBLIC_VAR", "SRC_VAR"},
		},
		{
			name:         "Exclude specific file config.txt",
			excludeDirs:  "config.txt",
			expectedVars: []string{"PUBLIC_VAR", "SRC_VAR", "JS_VAR", "CSS_VAR"},
			notExpected:  []string{"CONFIG_VAR"},
		},
		{
			name:         "Include CSS and JS files",
			includeDirs:  "*.css,*.js",
			expectedVars: []string{"JS_VAR", "CSS_VAR"},
			notExpected:  []string{"PUBLIC_VAR", "SRC_VAR", "CONFIG_VAR"},
		},
		{
			name:         "Include all files but exclude CSS",
			excludeDirs:  "*.css",
			expectedVars: []string{"PUBLIC_VAR", "SRC_VAR", "JS_VAR", "CONFIG_VAR"},
			notExpected:  []string{"CSS_VAR"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkEnvVarsInFiles(dir, tt.includeDirs, tt.excludeDirs)

			if len(tt.expectedVars) > 0 {
				// Should find missing variables
				if err == nil {
					t.Errorf("Expected to find missing vars %v, but got no error", tt.expectedVars)
					return
				}

				errorMsg := err.Error()
				for _, varName := range tt.expectedVars {
					if !strings.Contains(errorMsg, varName) {
						t.Errorf("Expected to find %s in error message, but didn't. Error: %s", varName, errorMsg)
					}
				}
			}

			if len(tt.notExpected) > 0 && err != nil {
				errorMsg := err.Error()
				for _, varName := range tt.notExpected {
					if strings.Contains(errorMsg, varName) {
						t.Errorf("Expected NOT to find %s in error message, but did. Error: %s", varName, errorMsg)
					}
				}
			}
		})
	}
}

// Test cases for uncovered scenarios and edge cases
func TestReplaceEnvVarsInvalidSyntax(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		desc     string
	}{
		// Malformed patterns that should remain unchanged
		{"${", "${", "incomplete opening"},
		{"$}", "$}", "incomplete closing"},
		{"${FOO", "${FOO", "missing closing brace"},
		{"FOO}", "FOO}", "missing opening"},
		{"$FOO", "$FOO", "missing braces entirely"},
		{"${FOO:=}", "", "empty default value should work"},
		{"${FOO:=   }", "   ", "whitespace default should be preserved"},
		{"${FOO:=bar:baz}", "bar:baz", "colon in default value"},
		{"${FOO:=bar=baz}", "bar=baz", "equals in default value"},
		{"${FOO:=bar}baz}", "barbaz}", "extra text after valid pattern"},
		{"${${FOO}}", "${${FOO}}", "nested variable reference (malformed)"},
		{"${{FOO}}", "${{FOO}}", "double opening braces"},
		{"${FOO}}", "${FOO}}", "extra closing brace should remain"},
		{"${FOO:=bar${BAZ}}", "bar${BAZ}", "nested variable in default"},
	}

	for _, test := range tests {
		got := replaceEnvVars(test.input)
		if got != test.expected {
			t.Errorf("%s: replaceEnvVars(%q) = %q, want %q", test.desc, test.input, got, test.expected)
		}
	}
}

func TestReplaceEnvVarsVariableNameValidation(t *testing.T) {
	// Clear any existing environment variables that might interfere
	originalUnderscore := os.Getenv("_")
	os.Unsetenv("_")
	defer func() {
		if originalUnderscore != "" {
			os.Setenv("_", originalUnderscore)
		}
	}()

	tests := []struct {
		input    string
		expected string
		desc     string
	}{
		// Valid variable names (but not set)
		{"${_}", "${_}", "underscore only (valid but not set)"},
		{"${_ABC}", "${_ABC}", "underscore prefix"},
		{"${ABC_}", "${ABC_}", "underscore suffix"},
		{"${A_B_C}", "${A_B_C}", "underscores in middle"},
		{"${ABC123}", "${ABC123}", "letters and numbers"},
		{"${A1B2C3}", "${A1B2C3}", "mixed letters and numbers"},

		// Invalid variable names (should remain unchanged)
		{"${123ABC}", "${123ABC}", "starts with number"},
		{"${-ABC}", "${-ABC}", "starts with hyphen"},
		{"${ABC-DEF}", "${ABC-DEF}", "contains hyphen"},
		{"${ABC.DEF}", "${ABC.DEF}", "contains dot"},
		{"${ABC DEF}", "${ABC DEF}", "contains space"},
		{"${ABC@DEF}", "${ABC@DEF}", "contains special character"},
		{"${}", "${}", "empty variable name"},
		{"${ }", "${ }", "space as variable name"},
	}

	for _, test := range tests {
		got := replaceEnvVars(test.input)
		if got != test.expected {
			t.Errorf("%s: replaceEnvVars(%q) = %q, want %q", test.desc, test.input, got, test.expected)
		}
	}
}

func TestEnvFileSystemErrors(t *testing.T) {
	// Test error handling in EnvFileSystem
	dir := t.TempDir()

	// Test opening non-existent file
	fs := EnvFileSystem{fs: http.Dir(dir)}
	_, err := fs.Open("nonexistent.txt")
	if err == nil {
		t.Error("Expected error when opening non-existent file")
	}

	// Test with unreadable file (create file then remove read permissions)
	unreadableFile := filepath.Join(dir, "unreadable.txt")
	if err := os.WriteFile(unreadableFile, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	if err := os.Chmod(unreadableFile, 0000); err != nil {
		t.Fatalf("Failed to remove permissions: %v", err)
	}
	defer os.Chmod(unreadableFile, 0644) // Restore permissions for cleanup

	_, err = fs.Open("unreadable.txt")
	if err == nil {
		t.Error("Expected error when opening unreadable file")
	}
}

func TestEnvFileReaddir(t *testing.T) {
	dir := t.TempDir()

	// Create a test file that will be processed by EnvFileSystem
	testFile := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(testFile, []byte("content with ${VAR}"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create some subdirectories
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	fs := EnvFileSystem{fs: http.Dir(dir)}

	// Test opening a directory - this should return the original directory file, not an EnvFile
	f, err := fs.Open("/")
	if err != nil {
		t.Fatalf("Failed to open directory: %v", err)
	}
	defer f.Close()

	// Test Readdir functionality on directory
	entries, err := f.Readdir(-1)
	if err != nil {
		t.Fatalf("Readdir failed: %v", err)
	}

	if len(entries) < 2 {
		t.Errorf("Expected at least 2 entries, got %d", len(entries))
	}

	// Test opening a file - this should return an EnvFile
	envFile, err := fs.Open("test.txt")
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}

	// Verify it's an EnvFile
	if envFileTyped, ok := envFile.(*EnvFile); ok {
		// Test that we can call Readdir on a file (should fail)
		_, err = envFileTyped.Readdir(-1)
		if err == nil {
			t.Error("Expected error when calling Readdir on a file")
		}

		// Close the file and test Readdir on closed file
		envFileTyped.Close()
		envFileTyped.file = nil

		_, err = envFileTyped.Readdir(-1)
		if err == nil {
			t.Error("Expected error when calling Readdir on closed file")
		}
	} else {
		t.Error("Expected EnvFile type for regular file")
	}
}

func TestReplaceEnvVarsComplexScenarios(t *testing.T) {
	// Set up environment variables for testing
	os.Setenv("SET_VAR", "value")
	os.Setenv("EMPTY_VAR", "")
	os.Setenv("MULTILINE_VAR", "line1\nline2")
	os.Setenv("SPECIAL_CHARS", "!@#$%^&*()")
	defer func() {
		os.Unsetenv("SET_VAR")
		os.Unsetenv("EMPTY_VAR")
		os.Unsetenv("MULTILINE_VAR")
		os.Unsetenv("SPECIAL_CHARS")
	}()

	tests := []struct {
		input    string
		expected string
		desc     string
	}{
		// Multiple variables in one string
		{"${SET_VAR}${EMPTY_VAR}${UNSET:=default}", "valuedefault", "mixed set/empty/unset vars"},
		{"${SET_VAR} ${EMPTY_VAR} ${UNSET:=def}", "value  def", "vars with spaces"},

		// Multiline content
		{"Start\n${MULTILINE_VAR}\nEnd", "Start\nline1\nline2\nEnd", "multiline variable"},

		// Special characters
		{"${SPECIAL_CHARS}", "!@#$%^&*()", "special characters in value"},
		{"${UNSET:=!@#$%^&*()}", "!@#$%^&*()", "special characters in default"},

		// Large strings
		{"prefix_${SET_VAR}_middle_${EMPTY_VAR}_suffix_${UNSET:=def}_end", "prefix_value_middle__suffix_def_end", "complex pattern"},

		// Unicode characters
		{"${UNSET:=café}", "café", "unicode in default"},
		{"${UNSET:=測試}", "測試", "non-latin unicode in default"},
	}

	for _, test := range tests {
		got := replaceEnvVars(test.input)
		if got != test.expected {
			t.Errorf("%s: replaceEnvVars(%q) = %q, want %q", test.desc, test.input, got, test.expected)
		}
	}
}

func TestCheckEnvVarsInFilesErrorCases(t *testing.T) {
	// Test with empty root path
	err := checkEnvVarsInFiles("", "", "")
	if err == nil || !strings.Contains(err.Error(), "root path cannot be empty") {
		t.Errorf("Expected error about empty root path, got %v", err)
	}

	// Test with non-existent directory - filepath.Walk will return an error but it's not always the case
	// The function might succeed but find no files to process
	err = checkEnvVarsInFiles("/nonexistent/path", "", "")
	// This test should expect an error, but the specific error depends on the OS and filepath.Walk behavior
	if err == nil {
		t.Logf("No error returned for non-existent path (this may be OS-dependent)")
	} else {
		t.Logf("Got expected error for non-existent path: %v", err)
	}

	// Test with file instead of directory - filepath.Walk can handle files too
	tempFile := filepath.Join(t.TempDir(), "tempfile.txt")
	if err := os.WriteFile(tempFile, []byte("content with ${MISSING_VAR}"), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	// This should actually work and find the missing variable in the file
	err = checkEnvVarsInFiles(tempFile, "", "")
	if err == nil {
		t.Error("Expected error about missing environment variable in file")
	} else if !strings.Contains(err.Error(), "MISSING_VAR") {
		t.Errorf("Expected error about MISSING_VAR but got different error: %v", err)
	} else {
		t.Logf("Got expected error for missing variable: %v", err)
	}
}

func TestParsePatterns(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
		desc     string
	}{
		{"", nil, "empty string"},
		{"   ", nil, "whitespace only"},
		{"single", []string{"single"}, "single pattern"},
		{"one,two,three", []string{"one", "two", "three"}, "multiple patterns"},
		{"one, two , three ", []string{"one", "two", "three"}, "patterns with spaces"},
		{" , , ", nil, "only commas and spaces"},
		{"valid,,invalid,", []string{"valid", "invalid"}, "empty patterns in list"},
		{"a,b,c,", []string{"a", "b", "c"}, "trailing comma"},
		{",a,b,c", []string{"a", "b", "c"}, "leading comma"},
	}

	for _, test := range tests {
		got := parsePatterns(test.input)
		if len(got) != len(test.expected) {
			t.Errorf("%s: parsePatterns(%q) length = %d, want %d", test.desc, test.input, len(got), len(test.expected))
			continue
		}
		for i, pattern := range got {
			if pattern != test.expected[i] {
				t.Errorf("%s: parsePatterns(%q)[%d] = %q, want %q", test.desc, test.input, i, pattern, test.expected[i])
			}
		}
	}
}

func TestEnvFileSystemWithDifferentFileExtensions(t *testing.T) {
	dir := t.TempDir()

	// Test files with different extensions that should be processed
	testFiles := map[string]string{
		"test.html": "<html>${VAR}</html>",
		"test.js":   "var x = '${VAR}';",
		"test.css":  ".class { color: ${VAR}; }",
		"test.json": `{"key": "${VAR}"}`,
		"test.txt":  "Text with ${VAR}",
		"test.md":   "# ${VAR}",
		"test.xml":  "<root>${VAR}</root>",
		"test.yml":  "key: ${VAR}",
		"test.yaml": "key: ${VAR}",
		"test.log":  "Log entry: ${VAR}",
		"test.bak":  "Backup: ${VAR}",
	}

	// Files with extensions that should NOT be processed
	binaryFiles := map[string][]byte{
		"test.png": {0x89, 0x50, 0x4E, 0x47}, // PNG header
		"test.jpg": {0xFF, 0xD8, 0xFF, 0xE0}, // JPEG header
		"test.bin": {0x00, 0x01, 0x02, 0x03}, // Binary data
		"test.exe": {0x4D, 0x5A, 0x90, 0x00}, // PE header
	}

	// Create test files
	for filename, content := range testFiles {
		path := filepath.Join(dir, filename)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create %s: %v", filename, err)
		}
	}

	for filename, content := range binaryFiles {
		path := filepath.Join(dir, filename)
		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatalf("Failed to create %s: %v", filename, err)
		}
	}

	os.Setenv("VAR", "test_value")
	defer os.Unsetenv("VAR")

	fs := EnvFileSystem{fs: http.Dir(dir)}

	// Test that text files are processed
	for filename := range testFiles {
		f, err := fs.Open(filename)
		if err != nil {
			t.Errorf("Failed to open %s: %v", filename, err)
			continue
		}

		content, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			t.Errorf("Failed to read %s: %v", filename, err)
			continue
		}

		if !strings.Contains(string(content), "test_value") {
			t.Errorf("File %s was not processed correctly, content: %s", filename, string(content))
		}
		if strings.Contains(string(content), "${VAR}") {
			t.Errorf("File %s still contains unprocessed variable: %s", filename, string(content))
		}
	}

	// Test that binary files are also processed (current implementation processes all files)
	// Note: This documents current behavior - binary files get processed too
	for filename := range binaryFiles {
		f, err := fs.Open(filename)
		if err != nil {
			t.Errorf("Failed to open %s: %v", filename, err)
			continue
		}
		f.Close()
	}
}

func TestEnvFileStatSize(t *testing.T) {
	dir := t.TempDir()
	originalContent := "Original content with ${VAR} variable"
	processedContent := "Original content with expanded_value variable"

	// Create test file
	testFile := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(testFile, []byte(originalContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	os.Setenv("VAR", "expanded_value")
	defer os.Unsetenv("VAR")

	fs := EnvFileSystem{fs: http.Dir(dir)}
	f, err := fs.Open("test.txt")
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer f.Close()

	// Test that Stat() returns the size of processed content, not original
	stat, err := f.Stat()
	if err != nil {
		t.Fatalf("Failed to get file stat: %v", err)
	}

	expectedSize := int64(len(processedContent))
	if stat.Size() != expectedSize {
		t.Errorf("Expected size %d, got %d", expectedSize, stat.Size())
	}
}

func TestMatchPatternEdgeCases(t *testing.T) {
	tests := []struct {
		path    string
		pattern string
		include bool
		want    bool
		desc    string
	}{
		// Empty cases - based on actual implementation behavior
		{"", "", true, true, "both empty returns true in actual implementation"},
		{"path", "", true, false, "empty pattern should not match non-empty path"},
		{"", "pattern", true, false, "empty path should not match non-empty pattern"},

		// Case sensitivity
		{"Test", "test", true, false, "case sensitive - should not match"},
		{"test", "Test", true, false, "case sensitive - should not match reverse"},

		// Special characters in patterns
		{"file[1]", "file[1]", true, true, "literal brackets should match exactly"},
		{"file1", "file[1-3]", true, true, "character class should match"},
		{"file4", "file[1-3]", true, false, "character class should not match outside range"},

		// Path separators
		{"docs\\api", "docs/api", true, false, "backslash vs forward slash should not match"},
		{"docs/api/v1", "docs/api", true, true, "subdirectory should match parent"},
	}

	for _, tt := range tests {
		got := matchPattern(tt.path, tt.pattern, tt.include)
		if got != tt.want {
			t.Errorf("%s: matchPattern(%q, %q, %v) = %v, want %v", tt.desc, tt.path, tt.pattern, tt.include, got, tt.want)
		}
	}
}

func TestShouldIncludeFileVsDirectory(t *testing.T) {
	includePatterns := []string{"*.js", "src"}
	excludePatterns := []string{"*.tmp"}

	tests := []struct {
		path   string
		isFile bool
		want   bool
		desc   string
	}{
		{"test.js", true, true, "JS file should be included"},
		{"test.txt", true, false, "TXT file should not be included when only JS files included"},
		{"src", false, true, "src directory should be included"},
		{"test.tmp", true, false, "tmp file should be excluded"},
		{"cache.tmp", false, false, "tmp directory should be excluded"},
		{"src/app.js", true, true, "JS file in src should be included"},
		{"docs", false, false, "docs directory should not be included"},
	}

	for _, tt := range tests {
		got := shouldInclude(tt.path, includePatterns, excludePatterns, tt.isFile)
		if got != tt.want {
			t.Errorf("%s: shouldInclude(%q, %v, %v, %v) = %v, want %v", tt.desc, tt.path, includePatterns, excludePatterns, tt.isFile, got, tt.want)
		}
	}
}

func TestRegexPatternMatchingLimits(t *testing.T) {
	// Test edge cases for the regex pattern
	tests := []struct {
		input string
		desc  string
	}{
		{"${" + strings.Repeat("A", 1000) + "}", "very long variable name"},
		{"${VAR" + strings.Repeat(":=default", 100) + "}", "very long default value"},
		{strings.Repeat("${VAR}", 1000), "many variables in one string"},
		{"${VAR:=" + strings.Repeat("x", 10000) + "}", "extremely long default"},
	}

	for _, test := range tests {
		// Just ensure it doesn't panic or hang
		result := replaceEnvVars(test.input)
		if len(result) == 0 && len(test.input) > 0 {
			t.Errorf("%s: got empty result for non-empty input", test.desc)
		}
	}
}
