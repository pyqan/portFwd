package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/pyqan/portFwd/internal/config"
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

// Unused but keep for potential future use
var _ = time.Now
