package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/pyqan/portFwd/internal/config"
	"github.com/pyqan/portFwd/internal/daemon"
	"github.com/pyqan/portFwd/internal/k8s"
	"github.com/pyqan/portFwd/internal/logger"
	"github.com/pyqan/portFwd/internal/portforward"
	"github.com/pyqan/portFwd/internal/ui"
)

var (
	version = "1.0.0"

	// Global flags
	namespace  string
	configPath string
	debugMode  bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "portfwd",
		Short: "Kubernetes Port Forward Manager",
		Long: `PortFwd is a powerful TUI application for managing Kubernetes port-forward connections.

Features:
  • Interactive namespace/pod/service selection
  • Multiple simultaneous port-forwards
  • Save and load profiles
  • Beautiful terminal UI`,
		RunE: runInteractive,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "Kubernetes namespace")
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "Config file path")
	rootCmd.PersistentFlags().BoolVarP(&debugMode, "debug", "d", false, "Enable debug logging to ~/.config/portfwd/debug.log")

	// Add subcommands
	rootCmd.AddCommand(
		newForwardCmd(),
		newListCmd(),
		newProfileCmd(),
		newVersionCmd(),
		newDaemonCmd(),
		newAddCmd(),
		newRemoveCmd(),
		newStatusCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// runInteractive starts the TUI application
func runInteractive(cmd *cobra.Command, args []string) error {
	// Initialize debug logger
	if err := logger.Init(debugMode); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize debug logger: %v\n", err)
	}
	defer logger.Close()

	if debugMode {
		logger.Info("main", "PortFwd started in debug mode")
		logger.Debug("main", "Log file: %s", logger.GetLogPath())
	}

	k8sClient, err := k8s.NewClient()
	if err != nil {
		logger.Error("main", "Failed to create Kubernetes client: %v", err)
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}
	logger.Debug("main", "Kubernetes client initialized")

	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Error("main", "Failed to load config: %v", err)
		return fmt.Errorf("failed to load config: %w", err)
	}
	logger.Debug("main", "Config loaded")

	pfManager := portforward.NewManager(k8sClient.GetClientset(), k8sClient.GetRestConfig())
	logger.Debug("main", "Port-forward manager created")

	// Cleanup on exit
	defer func() {
		logger.Debug("main", "Stopping all connections...")
		pfManager.StopAll()
		logger.Info("main", "PortFwd shutdown complete")
	}()

	return ui.Run(k8sClient, pfManager, cfg, debugMode)
}

// newForwardCmd creates the forward command
func newForwardCmd() *cobra.Command {
	var (
		pod        string
		service    string
		localPort  int
		remotePort int
	)

	cmd := &cobra.Command{
		Use:   "forward",
		Short: "Start a port-forward",
		Long:  "Start a port-forward to a pod or service",
		Example: `  # Forward local port 8080 to pod's port 80
  portfwd forward -n default -p my-pod -l 8080 -r 80

  # Forward using same port numbers
  portfwd forward -n default -p my-pod -l 3000 -r 3000`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if namespace == "" {
				return fmt.Errorf("namespace is required (-n)")
			}
			if pod == "" && service == "" {
				return fmt.Errorf("either pod (-p) or service (-s) is required")
			}
			if localPort == 0 {
				return fmt.Errorf("local port is required (-l)")
			}
			if remotePort == 0 {
				remotePort = localPort
			}

			k8sClient, err := k8s.NewClient()
			if err != nil {
				return fmt.Errorf("failed to create Kubernetes client: %w", err)
			}

			target := pod
			if service != "" {
				target = service
			}

			pfManager := portforward.NewManager(k8sClient.GetClientset(), k8sClient.GetRestConfig())

			// Handle signals for graceful shutdown
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

			go func() {
				<-sigChan
				fmt.Println("\nShutting down...")
				pfManager.StopAll()
				cancel()
			}()

			fmt.Printf("Starting port-forward: localhost:%d -> %s/%s:%d\n", localPort, namespace, target, remotePort)

			conn, err := pfManager.StartPortForward(ctx, namespace, target, localPort, remotePort)
			if err != nil {
				return fmt.Errorf("failed to start port-forward: %w", err)
			}

			fmt.Printf("✓ Port forward active: localhost:%d\n", localPort)
			fmt.Println("Press Ctrl+C to stop")

			// Wait for context cancellation
			<-ctx.Done()

			info := conn.GetConnectionInfo()
			fmt.Printf("\nPort forward stopped after %s\n", info.Duration)

			return nil
		},
	}

	cmd.Flags().StringVarP(&pod, "pod", "p", "", "Pod name")
	cmd.Flags().StringVarP(&service, "service", "s", "", "Service name")
	cmd.Flags().IntVarP(&localPort, "local", "l", 0, "Local port")
	cmd.Flags().IntVarP(&remotePort, "remote", "r", 0, "Remote port (defaults to local port)")

	return cmd
}

// newListCmd creates the list command
func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List resources",
		Long:    "List Kubernetes resources (namespaces, pods, services)",
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "namespaces",
			Short: "List namespaces",
			Aliases: []string{"ns"},
			RunE: func(cmd *cobra.Command, args []string) error {
				k8sClient, err := k8s.NewClient()
				if err != nil {
					return err
				}

				namespaces, err := k8sClient.GetNamespaces(context.Background())
				if err != nil {
					return err
				}

				fmt.Println("NAMESPACES:")
				for _, ns := range namespaces {
					fmt.Printf("  %s\n", ns)
				}
				return nil
			},
		},
		&cobra.Command{
			Use:   "pods",
			Short: "List pods in namespace",
			RunE: func(cmd *cobra.Command, args []string) error {
				if namespace == "" {
					namespace = "default"
				}

				k8sClient, err := k8s.NewClient()
				if err != nil {
					return err
				}

				pods, err := k8sClient.GetPods(context.Background(), namespace)
				if err != nil {
					return err
				}

				fmt.Printf("PODS in %s:\n", namespace)
				for _, pod := range pods {
					ports := formatPorts(pod.Ports)
					fmt.Printf("  %-40s %-10s %s\n", pod.Name, pod.Status, ports)
				}
				return nil
			},
		},
		&cobra.Command{
			Use:   "services",
			Short: "List services in namespace",
			Aliases: []string{"svc"},
			RunE: func(cmd *cobra.Command, args []string) error {
				if namespace == "" {
					namespace = "default"
				}

				k8sClient, err := k8s.NewClient()
				if err != nil {
					return err
				}

				services, err := k8sClient.GetServices(context.Background(), namespace)
				if err != nil {
					return err
				}

				fmt.Printf("SERVICES in %s:\n", namespace)
				for _, svc := range services {
					ports := formatServicePorts(svc.Ports)
					fmt.Printf("  %-40s %-12s %s\n", svc.Name, svc.Type, ports)
				}
				return nil
			},
		},
	)

	return cmd
}

// newProfileCmd creates the profile command
func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage port-forward profiles",
		Long:  "Save, load, and manage port-forward configurations",
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List saved profiles",
			Aliases: []string{"ls"},
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load(configPath)
				if err != nil {
					return err
				}

				profiles := cfg.ListProfiles()
				if len(profiles) == 0 {
					fmt.Println("No profiles saved")
					return nil
				}

				fmt.Println("PROFILES:")
				for _, name := range profiles {
					profile, _ := cfg.GetProfile(name)
					fmt.Printf("  %-20s (%d forwards)\n", name, len(profile.Forwards))
					if profile.Description != "" {
						fmt.Printf("    %s\n", profile.Description)
					}
				}
				return nil
			},
		},
		&cobra.Command{
			Use:   "show [name]",
			Short: "Show profile details",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load(configPath)
				if err != nil {
					return err
				}

				profile, err := cfg.GetProfile(args[0])
				if err != nil {
					return err
				}

				fmt.Printf("Profile: %s\n", profile.Name)
				if profile.Description != "" {
					fmt.Printf("Description: %s\n", profile.Description)
				}
				fmt.Println("\nForwards:")
				for _, fwd := range profile.Forwards {
					target := fwd.Pod
					if fwd.Service != "" {
						target = fmt.Sprintf("svc/%s", fwd.Service)
					}
					fmt.Printf("  %s/%s  localhost:%d -> %d\n", fwd.Namespace, target, fwd.LocalPort, fwd.RemotePort)
				}
				return nil
			},
		},
		&cobra.Command{
			Use:   "start [name]",
			Short: "Start all forwards in a profile",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load(configPath)
				if err != nil {
					return err
				}

				profile, err := cfg.GetProfile(args[0])
				if err != nil {
					return err
				}

				k8sClient, err := k8s.NewClient()
				if err != nil {
					return err
				}

				pfManager := portforward.NewManager(k8sClient.GetClientset(), k8sClient.GetRestConfig())

				// Handle signals for graceful shutdown
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				sigChan := make(chan os.Signal, 1)
				signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

				go func() {
					<-sigChan
					fmt.Println("\nShutting down...")
					pfManager.StopAll()
					cancel()
				}()

				fmt.Printf("Starting profile: %s\n", profile.Name)

				for _, fwd := range profile.Forwards {
					target := fwd.Pod
					if fwd.Service != "" {
						target = fwd.Service
					}

					_, err := pfManager.StartPortForward(ctx, fwd.Namespace, target, fwd.LocalPort, fwd.RemotePort)
					if err != nil {
						fmt.Printf("✗ Failed: %s/%s - %v\n", fwd.Namespace, target, err)
						continue
					}
					fmt.Printf("✓ localhost:%d -> %s/%s:%d\n", fwd.LocalPort, fwd.Namespace, target, fwd.RemotePort)
				}

				fmt.Println("\nPress Ctrl+C to stop all forwards")
				<-ctx.Done()

				return nil
			},
		},
		&cobra.Command{
			Use:   "delete [name]",
			Short: "Delete a profile",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load(configPath)
				if err != nil {
					return err
				}

				if err := cfg.DeleteProfile(args[0]); err != nil {
					return err
				}

				if err := cfg.Save(configPath); err != nil {
					return err
				}

				fmt.Printf("Profile '%s' deleted\n", args[0])
				return nil
			},
		},
	)

	return cmd
}

// newVersionCmd creates the version command
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("PortFwd version %s\n", version)
		},
	}
}

// Helper functions
func formatPorts(ports []k8s.ContainerPort) string {
	if len(ports) == 0 {
		return "-"
	}
	var parts []string
	for _, p := range ports {
		parts = append(parts, strconv.Itoa(int(p.ContainerPort)))
	}
	return strings.Join(parts, ",")
}

func formatServicePorts(ports []k8s.ServicePort) string {
	if len(ports) == 0 {
		return "-"
	}
	var parts []string
	for _, p := range ports {
		parts = append(parts, fmt.Sprintf("%d->%s", p.Port, p.TargetPort))
	}
	return strings.Join(parts, ",")
}

// newDaemonCmd creates the daemon command
func newDaemonCmd() *cobra.Command {
	var foreground bool

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Manage background daemon",
		Long:  "Start, stop, and manage the PortFwd background daemon",
	}

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the daemon",
		Long:  "Start the PortFwd daemon to manage port-forwards in background",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize logger for daemon
			if err := logger.Init(debugMode || foreground); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to initialize logger: %v\n", err)
			}
			defer logger.Close()

			return daemon.StartDaemon(foreground)
		},
	}
	startCmd.Flags().BoolVarP(&foreground, "foreground", "f", false, "Run in foreground (don't daemonize)")

	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return daemon.StopDaemon()
		},
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show daemon status",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !daemon.IsDaemonRunning() {
				fmt.Println("Daemon is not running")
				return nil
			}

			client := daemon.NewClient()
			if err := client.Connect(); err != nil {
				return err
			}
			defer client.Close()

			resp, err := client.Status()
			if err != nil {
				return err
			}

			if !resp.Success {
				return fmt.Errorf(resp.Error)
			}

			var status daemon.StatusInfo
			if err := json.Unmarshal(resp.Data, &status); err != nil {
				return err
			}

			fmt.Printf("Daemon Status: Running\n")
			fmt.Printf("PID: %d\n", status.PID)
			fmt.Printf("Uptime: %s\n", status.Uptime)
			fmt.Printf("Active Connections: %d\n", len(status.Connections))

			if len(status.Connections) > 0 {
				fmt.Println("\nConnections:")
				for _, conn := range status.Connections {
					status := "●"
					if conn.Status == "stopped" {
						status = "○"
					} else if conn.Status == "error" {
						status = "✗"
					}
					fmt.Printf("  %s %s/%s/%s  localhost:%d -> %d  [%s]\n",
						status, conn.Namespace, conn.ResourceType, conn.ResourceName,
						conn.LocalPort, conn.RemotePort, conn.Duration)
				}
			}

			return nil
		},
	}

	cmd.AddCommand(startCmd, stopCmd, statusCmd)
	return cmd
}

// newAddCmd creates the add command for daemon
func newAddCmd() *cobra.Command {
	var (
		service    string
		pod        string
		localPort  int
		remotePort int
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add port-forward to running daemon",
		Long:  "Add a new port-forward to the running daemon",
		Example: `  # Add service port-forward
  portfwd add -n longhorn-system -s longhorn-frontend -l 8080 -r 80

  # Add pod port-forward
  portfwd add -n default -p my-pod -l 3000 -r 3000`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !daemon.IsDaemonRunning() {
				return fmt.Errorf("daemon is not running. Start it with: portfwd daemon start")
			}

			if namespace == "" {
				return fmt.Errorf("namespace is required (-n)")
			}
			if pod == "" && service == "" {
				return fmt.Errorf("either pod (-p) or service (-s) is required")
			}
			if localPort == 0 {
				return fmt.Errorf("local port is required (-l)")
			}
			if remotePort == 0 {
				remotePort = localPort
			}

			client := daemon.NewClient()
			if err := client.Connect(); err != nil {
				return err
			}
			defer client.Close()

			resourceType := "pod"
			resourceName := pod
			if service != "" {
				resourceType = "service"
				resourceName = service
			}

			resp, err := client.Add(namespace, resourceType, resourceName, localPort, remotePort)
			if err != nil {
				return err
			}

			if !resp.Success {
				return fmt.Errorf(resp.Error)
			}

			fmt.Println(resp.Message)
			return nil
		},
	}

	cmd.Flags().StringVarP(&service, "service", "s", "", "Service name")
	cmd.Flags().StringVarP(&pod, "pod", "p", "", "Pod name")
	cmd.Flags().IntVarP(&localPort, "local", "l", 0, "Local port")
	cmd.Flags().IntVarP(&remotePort, "remote", "r", 0, "Remote port (defaults to local)")

	return cmd
}

// newRemoveCmd creates the remove command for daemon
func newRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove [id]",
		Aliases: []string{"rm"},
		Short:   "Remove port-forward from daemon",
		Long:    "Remove a port-forward connection from the running daemon",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !daemon.IsDaemonRunning() {
				return fmt.Errorf("daemon is not running")
			}

			client := daemon.NewClient()
			if err := client.Connect(); err != nil {
				return err
			}
			defer client.Close()

			resp, err := client.Remove(args[0])
			if err != nil {
				return err
			}

			if !resp.Success {
				return fmt.Errorf(resp.Error)
			}

			fmt.Println(resp.Message)
			return nil
		},
	}

	return cmd
}

// newStatusCmd creates standalone status command (alias for daemon status)
func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show daemon and connections status",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !daemon.IsDaemonRunning() {
				fmt.Println("Daemon: Not running")
				fmt.Println("\nTo start daemon: portfwd daemon start")
				fmt.Println("To use TUI mode: portfwd")
				return nil
			}

			client := daemon.NewClient()
			if err := client.Connect(); err != nil {
				return err
			}
			defer client.Close()

			resp, err := client.Status()
			if err != nil {
				return err
			}

			var status daemon.StatusInfo
			if err := json.Unmarshal(resp.Data, &status); err != nil {
				return err
			}

			fmt.Printf("Daemon: Running (PID %d, uptime %s)\n", status.PID, status.Uptime)
			
			if len(status.Connections) == 0 {
				fmt.Println("\nNo active connections")
				fmt.Println("Add with: portfwd add -n <namespace> -s <service> -l <local-port> -r <remote-port>")
				return nil
			}

			fmt.Printf("\nConnections (%d):\n", len(status.Connections))
			fmt.Println("  ID                                                          LOCAL    REMOTE  STATUS    UPTIME")
			fmt.Println("  " + strings.Repeat("-", 90))
			
			for _, conn := range status.Connections {
				statusIcon := "●"
				switch conn.Status {
				case "stopped":
					statusIcon = "○"
				case "error":
					statusIcon = "✗"
				case "starting":
					statusIcon = "◐"
				}
				
				id := conn.ID
				if len(id) > 55 {
					id = id[:52] + "..."
				}
				
				fmt.Printf("  %-55s  %5d -> %-5d  %s %-8s %s\n",
					id, conn.LocalPort, conn.RemotePort, statusIcon, conn.Status, conn.Duration)
			}

			return nil
		},
	}
}

// Unused but keep for potential future use
var _ = time.Now
