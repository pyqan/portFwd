package daemon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Client communicates with the daemon via Unix socket
type Client struct {
	socketPath string
	conn       net.Conn
}

// NewClient creates a new daemon client
func NewClient() *Client {
	return &Client{
		socketPath: GetSocketPath(),
	}
}

// Connect establishes connection to daemon
func (c *Client) Connect() error {
	conn, err := net.DialTimeout("unix", c.socketPath, 5*time.Second)
	if err != nil {
		return fmt.Errorf("cannot connect to daemon (is it running?): %w", err)
	}
	c.conn = conn
	return nil
}

// Close closes the connection
func (c *Client) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// Send sends a request and returns the response
func (c *Client) Send(req *Request) (*Response, error) {
	if c.conn == nil {
		return nil, fmt.Errorf("not connected")
	}

	// Send request
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	data = append(data, '\n')

	if _, err := c.conn.Write(data); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Read response
	reader := bufio.NewReader(c.conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, nil
}

// IsDaemonRunning checks if daemon is running
func IsDaemonRunning() bool {
	// Primary check: try to connect to socket
	// This is the most reliable method
	client := NewClient()
	if err := client.Connect(); err != nil {
		// Socket not available, clean up stale files
		cleanupStaleFiles()
		return false
	}
	client.Close()
	return true
}

// cleanupStaleFiles removes PID and socket files if daemon is not running
func cleanupStaleFiles() {
	pidPath := GetPIDPath()
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		os.Remove(pidPath)
		os.Remove(GetSocketPath())
		return
	}

	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		os.Remove(pidPath)
		os.Remove(GetSocketPath())
		return
	}

	// Try to signal process (signal 0 just checks if process exists)
	if err := process.Signal(syscall.Signal(0)); err != nil {
		// Process doesn't exist, clean up
		os.Remove(pidPath)
		os.Remove(GetSocketPath())
	}
}

// GetDaemonPID returns the daemon PID if running
func GetDaemonPID() (int, error) {
	pidPath := GetPIDPath()
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(string(data))
}

// Helper methods for common operations

// Add sends an add command
func (c *Client) Add(namespace, resourceType, resourceName string, localPort, remotePort int) (*Response, error) {
	payload := AddPayload{
		Namespace:    namespace,
		ResourceType: resourceType,
		ResourceName: resourceName,
		LocalPort:    localPort,
		RemotePort:   remotePort,
	}
	req, err := NewRequest(CmdAdd, payload)
	if err != nil {
		return nil, err
	}
	return c.Send(req)
}

// Remove sends a remove command
func (c *Client) Remove(id string) (*Response, error) {
	payload := RemovePayload{ID: id}
	req, err := NewRequest(CmdRemove, payload)
	if err != nil {
		return nil, err
	}
	return c.Send(req)
}

// List sends a list command
func (c *Client) List() (*Response, error) {
	req, err := NewRequest(CmdList, nil)
	if err != nil {
		return nil, err
	}
	return c.Send(req)
}

// Status sends a status command
func (c *Client) Status() (*Response, error) {
	req, err := NewRequest(CmdStatus, nil)
	if err != nil {
		return nil, err
	}
	return c.Send(req)
}

// Shutdown sends a shutdown command
func (c *Client) Shutdown() (*Response, error) {
	req, err := NewRequest(CmdShutdown, nil)
	if err != nil {
		return nil, err
	}
	return c.Send(req)
}

// Stop sends a stop command for a specific connection
func (c *Client) Stop(id string) (*Response, error) {
	payload := RemovePayload{ID: id}
	req, err := NewRequest(CmdStop, payload)
	if err != nil {
		return nil, err
	}
	return c.Send(req)
}
