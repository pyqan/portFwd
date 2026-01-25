package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/pyqan/portFwd/internal/config"
	"github.com/pyqan/portFwd/internal/k8s"
	"github.com/pyqan/portFwd/internal/logger"
	"github.com/pyqan/portFwd/internal/portforward"
)

// Daemon manages port-forward connections in background
type Daemon struct {
	k8sClient *k8s.Client
	manager   *portforward.Manager
	server    *Server
	startTime time.Time
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewDaemon creates a new daemon instance
func NewDaemon() (*Daemon, error) {
	// Initialize K8s client
	k8sClient, err := k8s.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Create port-forward manager
	manager := portforward.NewManager(k8sClient.GetClientset(), k8sClient.GetRestConfig())

	ctx, cancel := context.WithCancel(context.Background())

	d := &Daemon{
		k8sClient: k8sClient,
		manager:   manager,
		startTime: time.Now(),
		ctx:       ctx,
		cancel:    cancel,
	}

	// Create IPC server with daemon as handler
	d.server = NewServer(manager, d)

	return d, nil
}

// Run starts the daemon
func (d *Daemon) Run() error {
	logger.Info("daemon", "Starting daemon...")

	// Ignore SIGHUP so we don't die when parent terminal closes
	signal.Ignore(syscall.SIGHUP)

	// Write PID file
	if err := d.writePIDFile(); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}
	defer d.removePIDFile()

	// Restore previous connections
	if err := d.restoreConnections(); err != nil {
		logger.Warn("daemon", "Failed to restore connections: %v", err)
	}

	// Start IPC server
	if err := d.server.Start(); err != nil {
		return fmt.Errorf("failed to start IPC server: %w", err)
	}
	defer d.server.Stop()

	logger.Info("daemon", "Daemon started (PID: %d)", os.Getpid())

	// Handle signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	select {
	case sig := <-sigChan:
		logger.Info("daemon", "Received signal: %v", sig)
	case <-d.ctx.Done():
		logger.Info("daemon", "Shutdown requested")
	}

	// Graceful shutdown
	d.shutdown()
	return nil
}

// HandleCommand implements CommandHandler interface
func (d *Daemon) HandleCommand(req *Request) *Response {
	switch req.Command {
	case CmdAdd:
		return d.handleAdd(req.Payload)
	case CmdRemove:
		return d.handleRemove(req.Payload)
	case CmdList:
		return d.handleList()
	case CmdStop:
		return d.handleStop(req.Payload)
	case CmdStatus:
		return d.handleStatus()
	case CmdShutdown:
		return d.handleShutdown()
	default:
		return NewErrorResponse(fmt.Sprintf("unknown command: %s", req.Command))
	}
}

func (d *Daemon) handleAdd(payload json.RawMessage) *Response {
	var p AddPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return NewErrorResponse(fmt.Sprintf("invalid payload: %v", err))
	}

	logger.Debug("daemon", "Adding port-forward: %s/%s/%s %d->%d",
		p.Namespace, p.ResourceType, p.ResourceName, p.LocalPort, p.RemotePort)

	// Determine resource type
	resType := portforward.ResourcePod
	if p.ResourceType == "service" || p.ResourceType == "svc" {
		resType = portforward.ResourceService
	}

	// Start port-forward
	ctx, cancel := context.WithTimeout(d.ctx, 30*time.Second)
	defer cancel()

	var conn *portforward.Connection
	var err error

	if resType == portforward.ResourceService {
		conn, err = d.manager.StartPortForwardToService(ctx, p.Namespace, p.ResourceName, p.LocalPort, p.RemotePort)
	} else {
		conn, err = d.manager.StartPortForwardToPod(ctx, p.Namespace, p.ResourceName, p.LocalPort, p.RemotePort)
	}

	if err != nil {
		logger.Error("daemon", "Failed to start port-forward: %v", err)
		return NewErrorResponse(fmt.Sprintf("failed to start port-forward: %v", err))
	}

	// Save state
	d.saveState()

	info := ConnectionToInfo(conn)
	return NewSuccessResponse(fmt.Sprintf("Port-forward started: localhost:%d -> %s:%d",
		p.LocalPort, p.ResourceName, p.RemotePort), info)
}

func (d *Daemon) handleRemove(payload json.RawMessage) *Response {
	var p RemovePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return NewErrorResponse(fmt.Sprintf("invalid payload: %v", err))
	}

	logger.Debug("daemon", "Removing connection: %s", p.ID)

	if err := d.manager.DeleteConnection(p.ID); err != nil {
		return NewErrorResponse(fmt.Sprintf("failed to remove: %v", err))
	}

	d.saveState()
	return NewSuccessResponse(fmt.Sprintf("Connection removed: %s", p.ID), nil)
}

func (d *Daemon) handleStop(payload json.RawMessage) *Response {
	var p RemovePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return NewErrorResponse(fmt.Sprintf("invalid payload: %v", err))
	}

	logger.Debug("daemon", "Stopping connection: %s", p.ID)

	if err := d.manager.StopPortForward(p.ID); err != nil {
		return NewErrorResponse(fmt.Sprintf("failed to stop: %v", err))
	}

	d.saveState()
	return NewSuccessResponse(fmt.Sprintf("Connection stopped: %s", p.ID), nil)
}

func (d *Daemon) handleList() *Response {
	connections := d.manager.GetConnections()
	infos := make([]ConnectionInfo, 0, len(connections))
	for _, conn := range connections {
		infos = append(infos, ConnectionToInfo(conn))
	}

	return NewSuccessResponse("", infos)
}

func (d *Daemon) handleStatus() *Response {
	connections := d.manager.GetConnections()
	infos := make([]ConnectionInfo, 0, len(connections))
	for _, conn := range connections {
		infos = append(infos, ConnectionToInfo(conn))
	}

	status := StatusInfo{
		Running:     true,
		PID:         os.Getpid(),
		Uptime:      formatDuration(time.Since(d.startTime)),
		Connections: infos,
	}

	return NewSuccessResponse("Daemon is running", status)
}

func (d *Daemon) handleShutdown() *Response {
	logger.Info("daemon", "Shutdown command received")
	
	// Trigger shutdown in background
	go func() {
		time.Sleep(100 * time.Millisecond)
		d.cancel()
	}()

	return NewSuccessResponse("Daemon shutting down", nil)
}

func (d *Daemon) shutdown() {
	logger.Info("daemon", "Shutting down...")

	// Save state before stopping
	d.saveState()

	// Stop all connections
	d.manager.StopAll()

	logger.Info("daemon", "Daemon stopped")
}

func (d *Daemon) writePIDFile() error {
	pidPath := GetPIDPath()
	
	// Ensure directory exists
	if err := os.MkdirAll(GetConfigDir(), 0755); err != nil {
		return err
	}

	return os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
}

func (d *Daemon) removePIDFile() {
	os.Remove(GetPIDPath())
}

func (d *Daemon) saveState() {
	state, err := config.LoadState()
	if err != nil {
		logger.Warn("daemon", "Failed to load state: %v", err)
		state = &config.SessionState{}
	}

	// Clear and rebuild
	state.Connections = nil

	for _, conn := range d.manager.GetAllConnectionsForSave() {
		state.Connections = append(state.Connections, config.SavedConnection{
			Namespace:    conn.Namespace,
			ResourceType: conn.ResourceType,
			ResourceName: conn.ResourceName,
			LocalPort:    conn.LocalPort,
			RemotePort:   conn.RemotePort,
			WasActive:    conn.WasActive,
		})
	}

	if err := state.Save(); err != nil {
		logger.Error("daemon", "Failed to save state: %v", err)
	}
}

func (d *Daemon) restoreConnections() error {
	state, err := config.LoadState()
	if err != nil {
		return err
	}

	if len(state.Connections) == 0 {
		logger.Debug("daemon", "No connections to restore")
		return nil
	}

	logger.Debug("daemon", "Restoring %d connections", len(state.Connections))

	restored := 0
	failed := 0

	for _, saved := range state.Connections {
		resType := portforward.ResourcePod
		if saved.ResourceType == "service" {
			resType = portforward.ResourceService
		}

		if !saved.WasActive {
			// Add as stopped connection (for tracking)
			d.manager.AddStoppedConnection(saved.Namespace, resType, saved.ResourceName, saved.LocalPort, saved.RemotePort)
			logger.Debug("daemon", "Added stopped connection: %s/%s/%s",
				saved.Namespace, saved.ResourceType, saved.ResourceName)
			continue
		}

		// Try to start active connections
		logger.Debug("daemon", "Restoring: %s/%s/%s %d->%d",
			saved.Namespace, saved.ResourceType, saved.ResourceName, saved.LocalPort, saved.RemotePort)

		ctx, cancel := context.WithTimeout(d.ctx, 30*time.Second)

		var err error
		if resType == portforward.ResourceService {
			_, err = d.manager.StartPortForwardToService(ctx, saved.Namespace, saved.ResourceName, saved.LocalPort, saved.RemotePort)
		} else {
			_, err = d.manager.StartPortForwardToPod(ctx, saved.Namespace, saved.ResourceName, saved.LocalPort, saved.RemotePort)
		}
		cancel()

		if err != nil {
			logger.Warn("daemon", "Failed to restore connection %s/%s/%s: %v",
				saved.Namespace, saved.ResourceType, saved.ResourceName, err)
			// Add as stopped connection so user can see it and retry
			d.manager.AddStoppedConnection(saved.Namespace, resType, saved.ResourceName, saved.LocalPort, saved.RemotePort)
			failed++
		} else {
			restored++
		}
	}

	logger.Info("daemon", "Connection restore complete: %d restored, %d failed", restored, failed)
	return nil
}

// StartDaemon starts the daemon process
func StartDaemon(foreground bool) error {
	// Check if already running
	if IsDaemonRunning() {
		return fmt.Errorf("daemon is already running")
	}

	if foreground {
		// Run in foreground (useful for debugging)
		return runDaemonProcess()
	}

	// Fork and run in background
	return forkDaemon()
}

func runDaemonProcess() error {
	daemon, err := NewDaemon()
	if err != nil {
		return err
	}
	return daemon.Run()
}

func forkDaemon() error {
	// Get current executable
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable: %w", err)
	}

	// Ensure config directory exists
	if err := os.MkdirAll(GetConfigDir(), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Prepare log file for daemon output
	logFile, err := os.OpenFile(GetLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	// Use exec.Command for better process management
	cmd := exec.Command(executable, "daemon", "start", "--foreground")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	
	// Set process group to detach from controlling terminal
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	pid := cmd.Process.Pid
	fmt.Printf("Daemon started (PID: %d)\n", pid)
	fmt.Printf("Log file: %s\n", GetLogPath())
	
	// Wait a bit to check if daemon started successfully
	time.Sleep(1 * time.Second)
	
	// Check if process is still running
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		return fmt.Errorf("daemon failed to start (check log: %s)", GetLogPath())
	}

	return nil
}

// StopDaemon stops the running daemon
func StopDaemon() error {
	if !IsDaemonRunning() {
		return fmt.Errorf("daemon is not running")
	}

	client := NewClient()
	if err := client.Connect(); err != nil {
		// Try to kill by PID
		pid, err := GetDaemonPID()
		if err != nil {
			return fmt.Errorf("cannot determine daemon PID: %w", err)
		}
		
		process, err := os.FindProcess(pid)
		if err != nil {
			return fmt.Errorf("cannot find process: %w", err)
		}
		
		return process.Signal(syscall.SIGTERM)
	}
	defer client.Close()

	resp, err := client.Shutdown()
	if err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("shutdown failed: %s", resp.Error)
	}

	fmt.Println(resp.Message)
	return nil
}
