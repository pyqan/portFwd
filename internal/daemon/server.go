package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/pyqan/portFwd/internal/logger"
	"github.com/pyqan/portFwd/internal/portforward"
)

// Server handles IPC communication via Unix socket
type Server struct {
	socketPath string
	listener   net.Listener
	manager    *portforward.Manager
	handler    CommandHandler
	mu         sync.Mutex
	clients    map[net.Conn]struct{}
	ctx        context.Context
	cancel     context.CancelFunc
}

// CommandHandler processes commands and returns responses
type CommandHandler interface {
	HandleCommand(req *Request) *Response
}

// NewServer creates a new IPC server
func NewServer(manager *portforward.Manager, handler CommandHandler) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		socketPath: GetSocketPath(),
		manager:    manager,
		handler:    handler,
		clients:    make(map[net.Conn]struct{}),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Start starts the IPC server
func (s *Server) Start() error {
	// Ensure config directory exists
	if err := os.MkdirAll(GetConfigDir(), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Remove existing socket if present
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		logger.Warn("daemon", "Failed to remove existing socket: %v", err)
	}

	// Create Unix socket listener
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to create socket: %w", err)
	}
	s.listener = listener

	// Set socket permissions
	if err := os.Chmod(s.socketPath, 0600); err != nil {
		logger.Warn("daemon", "Failed to set socket permissions: %v", err)
	}

	logger.Info("daemon", "IPC server started on %s", s.socketPath)

	// Accept connections in goroutine
	go s.acceptLoop()

	return nil
}

// Stop stops the IPC server
func (s *Server) Stop() {
	logger.Debug("daemon", "Stopping IPC server...")
	s.cancel()

	if s.listener != nil {
		s.listener.Close()
	}

	// Close all client connections
	s.mu.Lock()
	for conn := range s.clients {
		conn.Close()
	}
	s.clients = make(map[net.Conn]struct{})
	s.mu.Unlock()

	// Remove socket file
	os.Remove(s.socketPath)
	logger.Info("daemon", "IPC server stopped")
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return
			default:
				logger.Error("daemon", "Accept error: %v", err)
				continue
			}
		}

		s.mu.Lock()
		s.clients[conn] = struct{}{}
		s.mu.Unlock()

		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer func() {
		conn.Close()
		s.mu.Lock()
		delete(s.clients, conn)
		s.mu.Unlock()
	}()

	logger.Debug("daemon", "New client connection")

	reader := bufio.NewReader(conn)

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		// Read request line (JSON terminated by newline)
		line, err := reader.ReadBytes('\n')
		if err != nil {
			logger.Debug("daemon", "Client disconnected: %v", err)
			return
		}

		// Parse request
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			logger.Error("daemon", "Invalid request: %v", err)
			s.sendResponse(conn, NewErrorResponse("invalid request format"))
			continue
		}

		logger.Debug("daemon", "Received command: %s", req.Command)

		// Handle command
		resp := s.handler.HandleCommand(&req)

		// Send response
		if err := s.sendResponse(conn, resp); err != nil {
			logger.Error("daemon", "Failed to send response: %v", err)
			return
		}
	}
}

func (s *Server) sendResponse(conn net.Conn, resp *Response) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = conn.Write(data)
	return err
}
