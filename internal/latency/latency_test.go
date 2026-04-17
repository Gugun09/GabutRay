package latency

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"gabutray/internal/profile"
)

func TestCheckSuccess(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	_, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}

	result := Check(profile.Profile{
		Name:     "local",
		Protocol: profile.ProtocolTrojan,
		Address:  "127.0.0.1",
		Port:     port,
	}, time.Second)

	if result.Status != StatusOK {
		t.Fatalf("status = %s, want %s, error = %s", result.Status, StatusOK, result.Error)
	}
	if result.Duration <= 0 {
		t.Fatalf("duration should be positive: %s", result.Duration)
	}
}

func TestStatusFromTimeoutError(t *testing.T) {
	err := &net.DNSError{Err: "timeout", IsTimeout: true}
	if got := statusFromError(err); got != StatusTimeout {
		t.Fatalf("status = %s, want %s", got, StatusTimeout)
	}
	if got := statusFromError(errors.New("connection refused")); got != StatusFailed {
		t.Fatalf("status = %s, want %s", got, StatusFailed)
	}
}

func TestFormatResults(t *testing.T) {
	out := FormatResults([]Result{
		{
			Profile:  profile.Profile{Name: "main", Protocol: profile.ProtocolVLESS},
			Address:  "example.com:443",
			Status:   StatusOK,
			Duration: 48 * time.Millisecond,
		},
		{
			Profile: profile.Profile{Name: "backup", Protocol: profile.ProtocolTrojan},
			Address: "backup.example.com:443",
			Status:  StatusTimeout,
		},
	})
	for _, want := range []string{"main", "vless", "example.com:443", "48 ms", "backup", "timeout"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}
