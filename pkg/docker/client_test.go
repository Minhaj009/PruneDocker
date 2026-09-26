package docker

import (
	"context"
	"os"
	"runtime"
	"testing"

	"github.com/docker/docker/api/types"
)

func TestResolveDefaultHost(t *testing.T) {
	// 1. Explicit host provided
	host := ResolveDefaultHost("tcp://127.0.0.1:2375")
	if host != "tcp://127.0.0.1:2375" {
		t.Fatalf("expected tcp://127.0.0.1:2375, got %s", host)
	}

	// 2. DOCKER_HOST env var set
	os.Setenv("DOCKER_HOST", "unix:///tmp/custom.sock")
	defer os.Unsetenv("DOCKER_HOST")
	hostEnv := ResolveDefaultHost("")
	if hostEnv != "unix:///tmp/custom.sock" {
		t.Fatalf("expected unix:///tmp/custom.sock, got %s", hostEnv)
	}

	// 3. System default fallback
	os.Unsetenv("DOCKER_HOST")
	hostDefault := ResolveDefaultHost("")
	if runtime.GOOS == "windows" {
		if hostDefault != "npipe:////./pipe/docker_engine" {
			t.Fatalf("expected windows named pipe default, got %s", hostDefault)
		}
	} else {
		if hostDefault != "unix:///var/run/docker.sock" {
			t.Fatalf("expected unix socket default, got %s", hostDefault)
		}
	}
}

func TestCleanID(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"sha256:71923058869b1234567890abcdef", "71923058869b"},
		{"71923058869b1234567890abcdef", "71923058869b"},
		{"shortid", "shortid"},
		{"", ""},
	}

	for _, c := range cases {
		result := CleanID(c.input)
		if result != c.expected {
			t.Errorf("CleanID(%q) = %q; want %q", c.input, result, c.expected)
		}
	}
}

func TestMockClientPing(t *testing.T) {
	mock := NewMockDockerClient()
	ping, err := mock.Ping(context.Background())
	if err != nil {
		t.Fatalf("unexpected ping error: %v", err)
	}
	if ping.APIVersion != "1.45" {
		t.Errorf("expected APIVersion 1.45, got %s", ping.APIVersion)
	}

	mock.PingFunc = func(ctx context.Context) (types.Ping, error) {
		return types.Ping{}, os.ErrDeadlineExceeded
	}
	_, err = mock.Ping(context.Background())
	if err != os.ErrDeadlineExceeded {
		t.Fatalf("expected ErrDeadlineExceeded, got %v", err)
	}
}
