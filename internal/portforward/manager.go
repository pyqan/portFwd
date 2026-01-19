package portforward

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// For backward compatibility
// Deprecated: use StartPortForwardToPod or StartPortForwardToService
func (m *Manager) StartPortForward(ctx context.Context, namespace, podName string, localPort, remotePort int) (*Connection, error) {
	return m.StartPortForwardToPod(ctx, namespace, podName, localPort, remotePort)
}

// Status represents the status of a port-forward connection
type Status string

const (
	StatusActive       Status = "active"
	StatusStopped      Status = "stopped"
	StatusError        Status = "error"
	StatusStarting     Status = "starting"
	StatusReconnecting Status = "reconnecting"
)

// ResourceType for port-forward target
type ResourceType string

const (
	ResourcePod     ResourceType = "pod"
	ResourceService ResourceType = "service"
)

// Connection represents a single port-forward connection
type Connection struct {
	ID             string
	Namespace      string
	ResourceType   ResourceType
	ResourceName   string // pod name or service name
	LocalPort      int
	RemotePort     int
	Status         Status
	Error          string
	StartedAt      time.Time
	StoppedAt      time.Time
	Logs           []string
	ReconnectCount int
	AutoReconnect  bool

	stopChan   chan struct{}
	readyChan  chan struct{}
	stopOnce   sync.Once
	cancelFunc context.CancelFunc
	manager    *Manager
	mu         sync.RWMutex
}

// Manager manages multiple port-forward connections
type Manager struct {
	connections map[string]*Connection
	clientset   *kubernetes.Clientset
	restConfig  *rest.Config
	mu          sync.RWMutex
	onChange    func()
}

// NewManager creates a new port-forward manager
func NewManager(clientset *kubernetes.Clientset, restConfig *rest.Config) *Manager {
	return &Manager{
		connections: make(map[string]*Connection),
		clientset:   clientset,
		restConfig:  restConfig,
	}
}

// SetOnChange sets a callback function that is called when connections change
func (m *Manager) SetOnChange(fn func()) {
	m.onChange = fn
}

func (m *Manager) notifyChange() {
	if m.onChange != nil {
		m.onChange()
	}
}

// AddLog adds a log entry to connection
func (c *Connection) AddLog(msg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	timestamp := time.Now().Format("15:04:05")
	c.Logs = append(c.Logs, fmt.Sprintf("[%s] %s", timestamp, msg))
	if len(c.Logs) > 100 {
		c.Logs = c.Logs[len(c.Logs)-100:]
	}
}

// GetLogs returns connection logs
func (c *Connection) GetLogs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]string, len(c.Logs))
	copy(result, c.Logs)
	return result
}

// StartPortForwardToPod starts a port-forward to a pod
func (m *Manager) StartPortForwardToPod(ctx context.Context, namespace, podName string, localPort, remotePort int) (*Connection, error) {
	return m.startPortForward(ctx, namespace, ResourcePod, podName, localPort, remotePort)
}

// StartPortForwardToService starts a port-forward to a service
func (m *Manager) StartPortForwardToService(ctx context.Context, namespace, serviceName string, localPort, remotePort int) (*Connection, error) {
	return m.startPortForward(ctx, namespace, ResourceService, serviceName, localPort, remotePort)
}

// startPortForward starts a new port-forward connection
func (m *Manager) startPortForward(ctx context.Context, namespace string, resourceType ResourceType, resourceName string, localPort, remotePort int) (*Connection, error) {
	prefix := "pod"
	if resourceType == ResourceService {
		prefix = "svc"
	}
	id := fmt.Sprintf("%s/%s/%s:%d->%d", namespace, prefix, resourceName, localPort, remotePort)

	m.mu.Lock()
	if existing, ok := m.connections[id]; ok {
		existing.mu.RLock()
		status := existing.Status
		existing.mu.RUnlock()
		if status == StatusActive || status == StatusStarting {
			m.mu.Unlock()
			return nil, fmt.Errorf("port-forward already active for %s", id)
		}
		// Cancel existing connection if any
		if existing.cancelFunc != nil {
			existing.cancelFunc()
		}
		delete(m.connections, id)
	}

	// Create cancellable context for this connection
	connCtx, cancelFunc := context.WithCancel(ctx)

	conn := &Connection{
		ID:            id,
		Namespace:     namespace,
		ResourceType:  resourceType,
		ResourceName:  resourceName,
		LocalPort:     localPort,
		RemotePort:    remotePort,
		Status:        StatusStarting,
		StartedAt:     time.Now(),
		Logs:          make([]string, 0),
		AutoReconnect: true,
		manager:       m,
		stopChan:      make(chan struct{}),
		readyChan:     make(chan struct{}),
		cancelFunc:    cancelFunc,
	}

	conn.AddLog("Starting port-forward...")
	conn.AddLog(fmt.Sprintf("Target: %s/%s/%s", namespace, prefix, resourceName))
	conn.AddLog(fmt.Sprintf("Ports: localhost:%d -> %d", localPort, remotePort))

	m.connections[id] = conn
	m.mu.Unlock()
	m.notifyChange()

	// Start port-forward in goroutine with cancellable context
	errChan := make(chan error, 1)
	go func() {
		errChan <- m.runPortForward(connCtx, conn)
	}()

	// Wait for ready or error
	select {
	case <-conn.readyChan:
		conn.AddLog("✓ Port-forward ready!")
		return conn, nil
	case err := <-errChan:
		return nil, err
	case <-time.After(30 * time.Second):
		conn.AddLog("✗ Timeout")
		m.StopPortForward(id)
		return nil, fmt.Errorf("timeout waiting for port-forward")
	case <-ctx.Done():
		m.StopPortForward(id)
		return nil, ctx.Err()
	}
}

// runPortForward runs the port-forward (like kubectl does)
func (m *Manager) runPortForward(ctx context.Context, conn *Connection) error {
	var podName string
	var targetPort int = conn.RemotePort

	if conn.ResourceType == ResourceService {
		// For service, we need to find a backing pod (like kubectl does)
		conn.AddLog("Finding pod for service...")
		
		svc, err := m.clientset.CoreV1().Services(conn.Namespace).Get(ctx, conn.ResourceName, metav1.GetOptions{})
		if err != nil {
			conn.AddLog(fmt.Sprintf("✗ Service not found: %v", err))
			conn.mu.Lock()
			conn.Status = StatusError
			conn.Error = err.Error()
			conn.mu.Unlock()
			m.notifyChange()
			return err
		}
		conn.AddLog(fmt.Sprintf("Service: %s", svc.Name))
		
		// Find pod using service selector first (we need it to resolve named ports)
		selector := svc.Spec.Selector
		if len(selector) == 0 {
			err := fmt.Errorf("service has no selector")
			conn.AddLog(fmt.Sprintf("✗ %v", err))
			conn.mu.Lock()
			conn.Status = StatusError
			conn.Error = err.Error()
			conn.mu.Unlock()
			m.notifyChange()
			return err
		}
		
		var labelSelector []string
		for k, v := range selector {
			labelSelector = append(labelSelector, fmt.Sprintf("%s=%s", k, v))
		}
		
		pods, err := m.clientset.CoreV1().Pods(conn.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: strings.Join(labelSelector, ","),
		})
		if err != nil || len(pods.Items) == 0 {
			err := fmt.Errorf("no pods found for service")
			conn.AddLog(fmt.Sprintf("✗ %v", err))
			conn.mu.Lock()
			conn.Status = StatusError
			conn.Error = err.Error()
			conn.mu.Unlock()
			m.notifyChange()
			return err
		}
		
		// Find first running pod
		var runningPod *corev1.Pod
		for i := range pods.Items {
			if pods.Items[i].Status.Phase == corev1.PodRunning {
				runningPod = &pods.Items[i]
				podName = runningPod.Name
				conn.AddLog(fmt.Sprintf("Using pod: %s", podName))
				break
			}
		}
		
		if podName == "" {
			err := fmt.Errorf("no running pods found for service")
			conn.AddLog(fmt.Sprintf("✗ %v", err))
			conn.mu.Lock()
			conn.Status = StatusError
			conn.Error = err.Error()
			conn.mu.Unlock()
			m.notifyChange()
			return err
		}
		
		// Resolve targetPort from service spec
		// TargetPort can be: number, named port, or empty (defaults to Port)
		for _, port := range svc.Spec.Ports {
			if int(port.Port) == conn.RemotePort {
				if port.TargetPort.IntValue() != 0 {
					// TargetPort is a number
					targetPort = port.TargetPort.IntValue()
				} else if port.TargetPort.String() != "" && port.TargetPort.String() != "0" {
					// TargetPort is a named port - resolve from pod spec
					namedPort := port.TargetPort.String()
					conn.AddLog(fmt.Sprintf("Resolving named port: %s", namedPort))
					for _, container := range runningPod.Spec.Containers {
						for _, cp := range container.Ports {
							if cp.Name == namedPort {
								targetPort = int(cp.ContainerPort)
								conn.AddLog(fmt.Sprintf("Resolved %s -> %d", namedPort, targetPort))
								break
							}
						}
					}
				}
				// If still not resolved, targetPort stays as conn.RemotePort
				conn.AddLog(fmt.Sprintf("Service port %d -> pod port %d", port.Port, targetPort))
				break
			}
		}
	} else {
		// Port-forward to pod directly
		conn.AddLog("Checking pod status...")
		pod, err := m.clientset.CoreV1().Pods(conn.Namespace).Get(ctx, conn.ResourceName, metav1.GetOptions{})
		if err != nil {
			conn.AddLog(fmt.Sprintf("✗ Pod not found: %v", err))
			conn.mu.Lock()
			conn.Status = StatusError
			conn.Error = err.Error()
			conn.mu.Unlock()
			m.notifyChange()
			return err
		}

		if pod.Status.Phase != corev1.PodRunning {
			err := fmt.Errorf("pod is not running: %s", pod.Status.Phase)
			conn.AddLog(fmt.Sprintf("✗ %v", err))
			conn.mu.Lock()
			conn.Status = StatusError
			conn.Error = err.Error()
			conn.mu.Unlock()
			m.notifyChange()
			return err
		}
		conn.AddLog(fmt.Sprintf("Pod status: %s", pod.Status.Phase))
		podName = conn.ResourceName
	}

	// Build request URL for pod port-forward
	req := m.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(conn.Namespace).
		Name(podName).
		SubResource("portforward")

	conn.AddLog(fmt.Sprintf("URL: %s", req.URL().String()))
	conn.AddLog(fmt.Sprintf("Forwarding: localhost:%d -> %s:%d", conn.LocalPort, podName, targetPort))

	// Create SPDY transport
	transport, upgrader, err := spdy.RoundTripperFor(m.restConfig)
	if err != nil {
		conn.AddLog(fmt.Sprintf("✗ Transport error: %v", err))
		conn.mu.Lock()
		conn.Status = StatusError
		conn.Error = err.Error()
		conn.mu.Unlock()
		m.notifyChange()
		return err
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, req.URL())

	// Port mapping - use targetPort (resolved from service if applicable)
	ports := []string{fmt.Sprintf("%d:%d", conn.LocalPort, targetPort)}
	conn.AddLog(fmt.Sprintf("Port mapping: %s", ports[0]))

	// Create log writers
	outWriter := &logWriter{conn: conn}
	errWriter := &logWriter{conn: conn}

	// Create port forwarder - bind to 127.0.0.1 only (like kubectl with --address)
	fw, err := portforward.NewOnAddresses(
		dialer,
		[]string{"127.0.0.1"},
		ports,
		conn.stopChan,
		conn.readyChan,
		outWriter,
		errWriter,
	)
	if err != nil {
		conn.AddLog(fmt.Sprintf("✗ Failed to create forwarder: %v", err))
		conn.mu.Lock()
		conn.Status = StatusError
		conn.Error = err.Error()
		conn.mu.Unlock()
		m.notifyChange()
		return err
	}

	conn.AddLog("Starting tunnel...")

	// Run port forwarding in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- fw.ForwardPorts()
	}()

	// Wait for ready or error
	select {
	case <-conn.readyChan:
		conn.AddLog("✓ Tunnel ready")
		conn.mu.Lock()
		conn.Status = StatusActive
		conn.mu.Unlock()
		m.notifyChange()
		
	case err := <-errChan:
		conn.AddLog(fmt.Sprintf("✗ Forward error: %v", err))
		conn.mu.Lock()
		conn.Status = StatusError
		conn.Error = err.Error()
		conn.StoppedAt = time.Now()
		conn.mu.Unlock()
		m.notifyChange()
		return err
	}

	// Wait for forward to complete, stop signal, or context cancellation
	select {
	case err = <-errChan:
		conn.mu.Lock()
		if conn.Status != StatusStopped {
			if err != nil {
				conn.Status = StatusError
				conn.Error = err.Error()
				conn.AddLog(fmt.Sprintf("✗ Forward error: %v", err))
			} else {
				conn.Status = StatusStopped
				conn.AddLog("Port-forward stopped")
			}
			conn.StoppedAt = time.Now()
		}
		conn.mu.Unlock()
		m.notifyChange()
		return err
		
	case <-ctx.Done():
		// Context cancelled - exit immediately
		conn.AddLog("Shutting down...")
		return nil
	}
}

// logWriter writes to connection logs
type logWriter struct {
	conn *Connection
	buf  bytes.Buffer
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	w.buf.Write(p)
	for {
		line, err := w.buf.ReadString('\n')
		if err == io.EOF {
			w.buf.WriteString(line)
			break
		}
		line = strings.TrimSpace(line)
		if line != "" {
			w.conn.AddLog(line)
		}
	}
	return len(p), nil
}

// StopPortForward stops a port-forward connection
func (m *Manager) StopPortForward(id string) error {
	m.mu.RLock()
	conn, ok := m.connections[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("connection not found: %s", id)
	}

	conn.mu.Lock()
	if conn.Status == StatusStopped {
		conn.mu.Unlock()
		return nil
	}
	conn.Status = StatusStopped
	conn.StoppedAt = time.Now()
	conn.mu.Unlock()

	// Cancel the context to stop any blocking operations
	if conn.cancelFunc != nil {
		conn.cancelFunc()
	}

	// Safely close stop channel using sync.Once to prevent panic on double close
	conn.stopOnce.Do(func() {
		close(conn.stopChan)
	})

	m.notifyChange()
	return nil
}

// StopAll stops all port-forward connections (for graceful shutdown)
func (m *Manager) StopAll() {
	// Disable onChange to prevent blocking on Bubble Tea's Send() during shutdown
	m.mu.Lock()
	m.onChange = nil
	m.mu.Unlock()

	m.mu.RLock()
	connections := make([]*Connection, 0, len(m.connections))
	for _, conn := range m.connections {
		connections = append(connections, conn)
	}
	m.mu.RUnlock()

	// Stop all connections without notifying
	for _, conn := range connections {
		conn.mu.Lock()
		if conn.Status != StatusStopped {
			conn.Status = StatusStopped
			conn.StoppedAt = time.Now()
		}
		conn.mu.Unlock()

		if conn.cancelFunc != nil {
			conn.cancelFunc()
		}

		conn.stopOnce.Do(func() {
			close(conn.stopChan)
		})
	}
}

// GetConnection returns a specific connection
func (m *Manager) GetConnection(id string) (*Connection, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	conn, ok := m.connections[id]
	return conn, ok
}

// GetConnections returns all connections
func (m *Manager) GetConnections() []*Connection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Connection, 0, len(m.connections))
	for _, conn := range m.connections {
		result = append(result, conn)
	}
	return result
}

// GetActiveConnections returns only active connections
func (m *Manager) GetActiveConnections() []*Connection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Connection, 0)
	for _, conn := range m.connections {
		conn.mu.RLock()
		if conn.Status == StatusActive {
			result = append(result, conn)
		}
		conn.mu.RUnlock()
	}
	return result
}

// RemoveConnection removes a stopped connection from the manager
func (m *Manager) RemoveConnection(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	conn, ok := m.connections[id]
	if !ok {
		return fmt.Errorf("connection not found: %s", id)
	}

	conn.mu.RLock()
	status := conn.Status
	conn.mu.RUnlock()

	if status == StatusActive || status == StatusStarting {
		return fmt.Errorf("cannot remove active connection")
	}

	delete(m.connections, id)
	m.notifyChange()
	return nil
}

// ConnectionInfo returns display info for a connection
type ConnectionInfo struct {
	ID           string
	Namespace    string
	ResourceType ResourceType
	ResourceName string
	LocalPort    int
	RemotePort   int
	Status       Status
	Error        string
	Duration     time.Duration
}

// GetConnectionInfo returns info about a connection
func (c *Connection) GetConnectionInfo() ConnectionInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var duration time.Duration
	if c.Status == StatusActive {
		duration = time.Since(c.StartedAt)
	} else if !c.StoppedAt.IsZero() {
		duration = c.StoppedAt.Sub(c.StartedAt)
	}

	return ConnectionInfo{
		ID:           c.ID,
		Namespace:    c.Namespace,
		ResourceType: c.ResourceType,
		ResourceName: c.ResourceName,
		LocalPort:    c.LocalPort,
		RemotePort:   c.RemotePort,
		Status:       c.Status,
		Error:        c.Error,
		Duration:     duration,
	}
}

// SavedConnectionInfo represents connection info for saving
type SavedConnectionInfo struct {
	Namespace    string
	ResourceType string
	ResourceName string
	LocalPort    int
	RemotePort   int
	WasActive    bool
}

// GetAllConnectionsForSave returns all connections info for saving to state
func (m *Manager) GetAllConnectionsForSave() []SavedConnectionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]SavedConnectionInfo, 0)
	for _, conn := range m.connections {
		conn.mu.RLock()
		result = append(result, SavedConnectionInfo{
			Namespace:    conn.Namespace,
			ResourceType: string(conn.ResourceType),
			ResourceName: conn.ResourceName,
			LocalPort:    conn.LocalPort,
			RemotePort:   conn.RemotePort,
			WasActive:    conn.Status == StatusActive,
		})
		conn.mu.RUnlock()
	}
	return result
}

// AddStoppedConnection adds a connection in stopped state (for restoring from state)
func (m *Manager) AddStoppedConnection(namespace string, resourceType ResourceType, resourceName string, localPort, remotePort int) {
	prefix := "pod"
	if resourceType == ResourceService {
		prefix = "svc"
	}
	id := fmt.Sprintf("%s/%s/%s:%d->%d", namespace, prefix, resourceName, localPort, remotePort)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Don't add if already exists
	if _, ok := m.connections[id]; ok {
		return
	}

	conn := &Connection{
		ID:            id,
		Namespace:     namespace,
		ResourceType:  resourceType,
		ResourceName:  resourceName,
		LocalPort:     localPort,
		RemotePort:    remotePort,
		Status:        StatusStopped,
		StartedAt:     time.Now(),
		StoppedAt:     time.Now(),
		Logs:          make([]string, 0),
		AutoReconnect: true,
		manager:       m,
		stopChan:      make(chan struct{}),
		readyChan:     make(chan struct{}),
	}

	conn.AddLog("Restored from previous session (stopped)")
	m.connections[id] = conn
}

// DeleteConnection completely removes a connection from manager
func (m *Manager) DeleteConnection(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	conn, ok := m.connections[id]
	if !ok {
		return fmt.Errorf("connection not found: %s", id)
	}

	// Stop if running
	conn.mu.Lock()
	if conn.Status == StatusActive || conn.Status == StatusStarting {
		conn.Status = StatusStopped
		conn.StoppedAt = time.Now()
	}
	conn.mu.Unlock()

	if conn.cancelFunc != nil {
		conn.cancelFunc()
	}

	conn.stopOnce.Do(func() {
		close(conn.stopChan)
	})

	delete(m.connections, id)
	return nil
}
