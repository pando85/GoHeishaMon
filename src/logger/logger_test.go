package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	buf := &bytes.Buffer{}
	l := New(LevelInfo, buf)
	if l == nil {
		t.Error("New() returned nil")
	}
	if l.level != LevelInfo {
		t.Errorf("expected level %d, got %d", LevelInfo, l.level)
	}
}

func TestSetLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected Level
	}{
		{"debug", LevelDebug},
		{"info", LevelInfo},
		{"error", LevelError},
		{"unknown", LevelInfo},
		{"", LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			SetLevel(LevelError)
			SetLevelString(tt.input)
			if std.level != tt.expected {
				t.Errorf("SetLevelString(%q) = %d, expected %d", tt.input, std.level, tt.expected)
			}
		})
	}
}

func TestInfo(t *testing.T) {
	buf := &bytes.Buffer{}
	std = New(LevelInfo, buf)

	Info("test message")
	if !strings.Contains(buf.String(), "INFO:") {
		t.Errorf("expected INFO prefix, got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "test message") {
		t.Errorf("expected message, got %q", buf.String())
	}
}

func TestInfoFilteredAtErrorLevel(t *testing.T) {
	buf := &bytes.Buffer{}
	std = New(LevelError, buf)

	Info("test message")
	if buf.String() != "" {
		t.Errorf("expected no output at error level, got %q", buf.String())
	}
}

func TestError(t *testing.T) {
	buf := &bytes.Buffer{}
	std = New(LevelError, buf)

	Error("error message")
	if !strings.Contains(buf.String(), "ERROR:") {
		t.Errorf("expected ERROR prefix, got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "error message") {
		t.Errorf("expected message, got %q", buf.String())
	}
}

func TestDebug(t *testing.T) {
	buf := &bytes.Buffer{}
	std = New(LevelDebug, buf)

	Debug("debug message")
	if !strings.Contains(buf.String(), "DEBUG:") {
		t.Errorf("expected DEBUG prefix, got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "debug message") {
		t.Errorf("expected message, got %q", buf.String())
	}
}

func TestDebugFilteredAtInfoLevel(t *testing.T) {
	buf := &bytes.Buffer{}
	std = New(LevelInfo, buf)

	Debug("debug message")
	if buf.String() != "" {
		t.Errorf("expected no output at info level, got %q", buf.String())
	}
}

func TestDebugHex(t *testing.T) {
	buf := &bytes.Buffer{}
	std = New(LevelDebug, buf)

	DebugHex("prefix", []byte{0x01, 0x02, 0x03})
	output := buf.String()
	if !strings.Contains(output, "DEBUG:") {
		t.Errorf("expected DEBUG prefix, got %q", output)
	}
	if !strings.Contains(output, "prefix") {
		t.Errorf("expected prefix in output, got %q", output)
	}
	if !strings.Contains(output, "01 02 03") {
		t.Errorf("expected hex data in output, got %q", output)
	}
}

func TestDebugHexFilteredAtInfoLevel(t *testing.T) {
	buf := &bytes.Buffer{}
	std = New(LevelInfo, buf)

	DebugHex("prefix", []byte{0x01, 0x02, 0x03})
	if buf.String() != "" {
		t.Errorf("expected no output at info level, got %q", buf.String())
	}
}

func TestFormatting(t *testing.T) {
	buf := &bytes.Buffer{}
	std = New(LevelInfo, buf)

	Info("value: %d", 42)
	if !strings.Contains(buf.String(), "value: 42") {
		t.Errorf("expected formatted message, got %q", buf.String())
	}
}

func TestMultipleArguments(t *testing.T) {
	buf := &bytes.Buffer{}
	std = New(LevelInfo, buf)

	Info("values: %d, %s, %v", 1, "test", true)
	if !strings.Contains(buf.String(), "values: 1, test, true") {
		t.Errorf("expected formatted message, got %q", buf.String())
	}
}
