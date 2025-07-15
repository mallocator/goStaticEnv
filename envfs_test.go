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

	fs := envFileSystem{fs: http.Dir(dir)}
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
	fs := envFileSystem{fs: http.Dir(dir)}
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
		got := matchesPattern(tt.dirPath, tt.pattern)
		if got != tt.want {
			t.Errorf("matchesPattern(%q, %q) = %v; want %v", tt.dirPath, tt.pattern, got, tt.want)
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
		got := shouldExcludeDir(tt.dirPath, excludePatterns)
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
		got := shouldIncludeDir(tt.dirPath, includePatterns)
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
	if err == nil || !strings.Contains(err.Error(), "CONFIG_TMP_VAR") {
		t.Errorf("Expected error about CONFIG_TMP_VAR when including *.tmp pattern, got %v", err)
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
			name:          "Glob include: *.tmp",
			includeDirs:   "*.tmp",
			excludeDirs:   "",
			shouldFind:    []string{"CONFIG_TMP_VAR"},
			shouldNotFind: []string{"SRC_VAR", "PUBLIC_VAR", "TEST_UNIT_VAR"},
		},
		{
			name:          "Glob include: cache-?",
			includeDirs:   "cache-?",
			excludeDirs:   "",
			shouldFind:    []string{"CACHE_1_VAR", "CACHE_2_VAR"},
			shouldNotFind: []string{"SRC_VAR", "CONFIG_TMP_VAR", "TEST_UNIT_VAR"},
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
			if tt.name == "Glob include: *.tmp" || tt.name == "Complex path: docs/v*" || tt.name == "Mixed: src,docs/v* exclude docs/v1-*" {
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
