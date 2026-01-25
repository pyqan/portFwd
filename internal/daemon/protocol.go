package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pyqan/portFwd/internal/portforward"
)

// Socket and PID file paths
func GetSocketPath() string {
	return filepath.Join(GetConfigDir(), "portfwd.sock")
}

func GetPIDPath() string {
	return filepath.Join(GetConfigDir(), "portfwd.pid")
}

func GetLogPath() string {
	return filepath.Join(GetConfigDir(), "daemon.log")
}

func GetConfigDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = os.TempDir()
	}
	return filepath.Join(configDir, "portfwd")
}

// Command types
type CommandType string

const (
	CmdAdd      CommandType = "add"
	CmdRemove   CommandType = "remove"
	CmdList     CommandType = "list"
	CmdStop     CommandType = "stop"
	CmdStatus   CommandType = "status"
	CmdShutdown CommandType = "shutdown"
)

// Request represents a command from CLI to daemon
type Request struct {
	Command CommandType     `json:"command"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// AddPayload for add command
type AddPayload struct {
	Namespace    string `json:"namespace"`
	ResourceType string `json:"resource_type"` // "pod" or "service"
	ResourceName string `json:"resource_name"`
	LocalPort    int    `json:"local_port"`
	RemotePort   int    `json:"remote_port"`
}

// RemovePayload for remove command
type RemovePayload struct {
	ID string `json:"id"`
}

// Response from daemon to CLI
type Response struct {
	Success bool            `json:"success"`
	Message string          `json:"message,omitempty"`
	Error   string          `json:"error,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// ConnectionInfo for list response
type ConnectionInfo struct {
	ID           string `json:"id"`
	Namespace    string `json:"namespace"`
	ResourceType string `json:"resource_type"`
	ResourceName string `json:"resource_name"`
	LocalPort    int    `json:"local_port"`
	RemotePort   int    `json:"remote_port"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
	Duration     string `json:"duration"`
}

// StatusInfo for status response
type StatusInfo struct {
	Running     bool             `json:"running"`
	PID         int              `json:"pid"`
	Uptime      string           `json:"uptime"`
	Connections []ConnectionInfo `json:"connections"`
}

// Helper functions for creating requests/responses

func NewRequest(cmd CommandType, payload interface{}) (*Request, error) {
	req := &Request{Command: cmd}
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err)
		}
		req.Payload = data
	}
	return req, nil
}

func NewSuccessResponse(message string, data interface{}) *Response {
	resp := &Response{
		Success: true,
		Message: message,
	}
	if data != nil {
		jsonData, _ := json.Marshal(data)
		resp.Data = jsonData
	}
	return resp
}

func NewErrorResponse(err string) *Response {
	return &Response{
		Success: false,
		Error:   err,
	}
}

// Convert portforward.Connection to ConnectionInfo
func ConnectionToInfo(conn *portforward.Connection) ConnectionInfo {
	info := conn.GetConnectionInfo()
	resType := "pod"
	if info.ResourceType == portforward.ResourceService {
		resType = "service"
	}
	return ConnectionInfo{
		ID:           info.ID,
		Namespace:    info.Namespace,
		ResourceType: resType,
		ResourceName: info.ResourceName,
		LocalPort:    info.LocalPort,
		RemotePort:   info.RemotePort,
		Status:       string(info.Status),
		Error:        info.Error,
		Duration:     formatDuration(info.Duration),
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
