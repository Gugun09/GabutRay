package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"

	"gabutray/internal/config"
	"gabutray/internal/profile"
	"gabutray/internal/runtime"
)

type Request struct {
	Action  string                 `json:"action"`
	Profile *profile.Profile       `json:"profile,omitempty"`
	Options runtime.ConnectOptions `json:"options,omitempty"`
	Force   bool                   `json:"force,omitempty"`
	Lines   int                    `json:"lines,omitempty"`
}

type Response struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

func Run(socket string, paths config.Paths, out io.Writer) error {
	if os.Geteuid() != 0 {
		return errors.New("daemon must run as root; use sudo gabutray daemon or install the service")
	}
	if err := config.Ensure(paths); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(socket), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(socket); err == nil {
		if err := os.Remove(socket); err != nil {
			return fmt.Errorf("remove stale socket %s: %w", socket, err)
		}
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return fmt.Errorf("bind socket %s: %w", socket, err)
	}
	defer listener.Close()
	if err := os.Chmod(socket, 0o660); err != nil {
		return fmt.Errorf("chmod socket %s: %w", socket, err)
	}
	_ = exec.Command("chgrp", "sudo", socket).Run()

	fmt.Fprintf(out, "gabutray daemon listening on %s\n", socket)
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Fprintf(out, "daemon accept error: %v\n", err)
			continue
		}
		go func() {
			defer conn.Close()
			response := handle(conn, paths)
			_ = json.NewEncoder(conn).Encode(response)
		}()
	}
}

func RequestDaemon(socket string, request Request) (Response, error) {
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return Response{}, fmt.Errorf("cannot connect to daemon socket %s; run gabutray service install first or start sudo gabutray daemon: %w", socket, err)
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return Response{}, err
	}
	if unixConn, ok := conn.(*net.UnixConn); ok {
		_ = unixConn.CloseWrite()
	}
	var response Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return Response{}, fmt.Errorf("daemon returned invalid JSON: %w", err)
	}
	if !response.OK {
		return response, errors.New(response.Message)
	}
	return response, nil
}

func handle(conn net.Conn, paths config.Paths) Response {
	var request Request
	if err := json.NewDecoder(conn).Decode(&request); err != nil {
		return Response{OK: false, Message: "invalid daemon request: " + err.Error()}
	}
	switch request.Action {
	case "connect":
		if request.Profile == nil {
			return Response{OK: false, Message: "connect request missing profile"}
		}
		if request.Force {
			_ = runtime.Disconnect(paths, false, io.Discard)
		}
		if err := runtime.Connect(paths, *request.Profile, request.Options, io.Discard); err != nil {
			return Response{OK: false, Message: err.Error()}
		}
		status, _ := runtime.StatusText(paths)
		return Response{OK: true, Message: "connected: " + request.Profile.Name, Data: status}
	case "disconnect":
		if err := runtime.Disconnect(paths, false, io.Discard); err != nil {
			return Response{OK: false, Message: err.Error()}
		}
		return Response{OK: true, Message: "disconnected"}
	case "status":
		text, err := runtime.StatusText(paths)
		if err != nil {
			return Response{OK: false, Message: err.Error()}
		}
		return Response{OK: true, Message: text}
	case "logs":
		lines := request.Lines
		if lines <= 0 {
			lines = 80
		}
		text, err := runtime.TailLogs(paths.LogDir, lines)
		if err != nil {
			return Response{OK: false, Message: err.Error()}
		}
		return Response{OK: true, Message: text}
	default:
		return Response{OK: false, Message: "unknown daemon action: " + request.Action}
	}
}
