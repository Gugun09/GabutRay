package latency

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"gabutray/internal/profile"
)

type Status string

const (
	StatusOK      Status = "ok"
	StatusTimeout Status = "timeout"
	StatusFailed  Status = "failed"
)

type Result struct {
	Profile  profile.Profile
	Address  string
	Duration time.Duration
	Status   Status
	Error    string
}

func Check(item profile.Profile, timeout time.Duration) Result {
	result := Result{
		Profile: item,
		Address: net.JoinHostPort(item.Address, fmt.Sprintf("%d", item.Port)),
		Status:  StatusFailed,
	}
	start := time.Now()
	conn, err := net.DialTimeout("tcp", result.Address, timeout)
	result.Duration = time.Since(start)
	if err != nil {
		result.Status = statusFromError(err)
		result.Error = err.Error()
		return result
	}
	_ = conn.Close()
	result.Status = StatusOK
	return result
}

func CheckAll(items []profile.Profile, timeout time.Duration) []Result {
	results := make([]Result, 0, len(items))
	for _, item := range items {
		results = append(results, Check(item, timeout))
	}
	return results
}

func FormatResults(results []Result) string {
	if len(results) == 0 {
		return "no profiles imported"
	}
	var b strings.Builder
	for _, result := range results {
		fmt.Fprintf(&b, "%-14s %-8s %-28s %s\n",
			result.Profile.Name,
			result.Profile.Protocol,
			result.Address,
			statusText(result),
		)
	}
	return strings.TrimRight(b.String(), "\n")
}

func statusText(result Result) string {
	switch result.Status {
	case StatusOK:
		ms := result.Duration.Round(time.Millisecond)
		if ms <= 0 {
			ms = time.Millisecond
		}
		return fmt.Sprintf("%d ms", ms.Milliseconds())
	case StatusTimeout:
		return string(StatusTimeout)
	default:
		return string(StatusFailed)
	}
}

func statusFromError(err error) Status {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return StatusTimeout
	}
	return StatusFailed
}
