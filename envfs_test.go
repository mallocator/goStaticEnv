package main

import (
	"io"
	"net/http"
	"os"
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

	err = checkEnvVarsInFiles(dir)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	os.Unsetenv("FOO")
	err = checkEnvVarsInFiles(dir)
	if err == nil || !strings.Contains(err.Error(), "FOO") {
		t.Errorf("Expected error about missing FOO, got %v", err)
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
