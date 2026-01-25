# ⚡ PortFwd - Kubernetes Port Forward Manager

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.21+-00ADD8?style=for-the-badge&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/Kubernetes-1.25+-326CE5?style=for-the-badge&logo=kubernetes" alt="Kubernetes">
  <img src="https://img.shields.io/badge/License-MIT-green?style=for-the-badge" alt="License">
</p>

A powerful TUI (Terminal User Interface) application for managing Kubernetes port-forward connections with style. Supports both interactive mode and background daemon.

## ✨ Features

- 🎨 **Beautiful TUI** - Cyberpunk-styled terminal interface
- 🔌 **Multiple Connections** - Manage many port-forwards simultaneously
- 🔄 **Auto-reconnect** - Automatic reconnection of dropped connections
- 💾 **Session Persistence** - Connections are saved and restored on restart
- 🖥️ **Background Daemon** - Run port-forwards as a background service
- 📋 **Profile Support** - Save and quickly restore port-forward configurations
- 🔍 **Debug Mode** - Detailed logging for troubleshooting
- ⚡ **Fast & Lightweight** - Single binary, no dependencies

## 📦 Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/pyqan/portFwd.git
cd portFwd

# Build
go build -o portfwd .

# Install to PATH (optional)
sudo mv portfwd /usr/local/bin/
```

## 🚀 Quick Start

### Interactive Mode (TUI)

Simply run `portfwd` to start the interactive terminal interface:

```bash
portfwd
```

### Background Daemon Mode

Run port-forwards as a background service:

```bash
# Start daemon
portfwd daemon start

# Check status
portfwd status

# Add port-forward to running daemon
portfwd add -n longhorn-system -s longhorn-frontend -l 8080 -r 80

# Stop daemon
portfwd daemon stop
```

### Command Line (One-shot)

```bash
# Forward local port 8080 to service port 80
portfwd forward -n default -s my-service -l 8080 -r 80

# Forward to pod
portfwd forward -n default -p my-pod -l 3000 -r 3000

# List resources
portfwd list pods -n kube-system
portfwd list services -n default
portfwd list namespaces
```

## 🎮 TUI Controls

### Connections View

| Key | Action |
|-----|--------|
| `↑/↓` or `j/k` | Navigate connections |
| `Enter` | Toggle: stop active / reconnect stopped |
| `n` | New port-forward |
| `d` | Stop selected connection |
| `r` | Reconnect selected |
| `x` or `Delete` | Delete connection from list |
| `l` | View connection logs |
| `?` | Show help |
| `q` | Quit |

### Selection Views (Namespace/Pod/Service)

| Key | Action |
|-----|--------|
| `↑/↓` or `j/k` | Navigate |
| `Enter` | Select |
| `p` | Quick select Pods |
| `s` | Quick select Services |
| `Esc` | Go back |

### Port Input

| Key | Action |
|-----|--------|
| `Tab` | Switch between local/remote port |
| `Enter` | Start port-forward |
| `Esc` | Cancel |

### Debug Mode (when enabled with `--debug`)

| Key | Action |
|-----|--------|
| `g` | View debug logs |
| `↑/↓` | Scroll logs |
| `PgUp/PgDn` | Fast scroll |
| `Home/End` | Jump to start/end |

## 🖥️ Daemon Mode

The daemon runs port-forwards in the background, surviving terminal closes.

### Commands

```bash
# Start daemon (background)
portfwd daemon start

# Start daemon (foreground, for debugging)
portfwd daemon start --foreground

# Stop daemon
portfwd daemon stop

# Show status
portfwd daemon status
# or simply:
portfwd status

# Add connection to running daemon
portfwd add -n <namespace> -s <service> -l <local-port> -r <remote-port>
portfwd add -n <namespace> -p <pod> -l <local-port> -r <remote-port>

# Remove connection
portfwd remove "<connection-id>"
```

### Files

| Path | Description |
|------|-------------|
| `~/.config/portfwd/state.yaml` | Saved connections (session persistence) |
| `~/.config/portfwd/portfwd.sock` | Unix socket for IPC |
| `~/.config/portfwd/portfwd.pid` | Daemon PID file |
| `~/.config/portfwd/daemon.log` | Daemon output log |
| `~/.config/portfwd/debug.log` | Debug log (when `--debug` enabled) |

## 🔧 CLI Reference

### Global Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--namespace` | `-n` | Kubernetes namespace |
| `--config` | `-c` | Config file path |
| `--debug` | `-d` | Enable debug logging |

### Commands

#### `portfwd`

Start the interactive TUI application.

```bash
portfwd              # Normal mode
portfwd --debug      # With debug logging (press 'g' to view logs)
```

#### `portfwd daemon`

Manage background daemon.

```bash
portfwd daemon start              # Start in background
portfwd daemon start --foreground # Start in foreground
portfwd daemon stop               # Stop daemon
portfwd daemon status             # Show daemon status
```

#### `portfwd add`

Add port-forward to running daemon.

```bash
portfwd add -n <namespace> -s <service> -l <local-port> [-r <remote-port>]
portfwd add -n <namespace> -p <pod> -l <local-port> [-r <remote-port>]
```

| Flag | Short | Description |
|------|-------|-------------|
| `--namespace` | `-n` | Kubernetes namespace (required) |
| `--service` | `-s` | Service name |
| `--pod` | `-p` | Pod name |
| `--local` | `-l` | Local port (required) |
| `--remote` | `-r` | Remote port (defaults to local) |

#### `portfwd remove`

Remove port-forward from daemon.

```bash
portfwd remove "<connection-id>"
# Example: portfwd remove "default/svc/my-service:8080->80"
```

#### `portfwd status`

Show daemon and connections status.

```bash
portfwd status
```

#### `portfwd forward`

Start a one-shot port-forward (blocks until Ctrl+C).

```bash
portfwd forward -n <namespace> -p <pod> -l <local-port> [-r <remote-port>]
portfwd forward -n <namespace> -s <service> -l <local-port> [-r <remote-port>]
```

#### `portfwd list`

List Kubernetes resources.

```bash
portfwd list namespaces          # or: portfwd list ns
portfwd list pods -n <namespace>
portfwd list services -n <namespace>  # or: portfwd list svc
```

#### `portfwd profile`

Manage port-forward profiles.

```bash
portfwd profile list
portfwd profile show <name>
portfwd profile start <name>
portfwd profile delete <name>
```

#### `portfwd version`

Print version information.

## 📋 Profiles

Profiles allow you to save and quickly restore port-forward configurations.

### Configuration File

Profiles are stored in `~/.config/portfwd/profiles.yaml`:

```yaml
profiles:
  - name: development
    description: Local development setup
    forwards:
      - namespace: default
        service: api-server
        localPort: 8080
        remotePort: 8080
      - namespace: default
        service: postgres
        localPort: 5432
        remotePort: 5432
      - namespace: monitoring
        service: grafana
        localPort: 3000
        remotePort: 80
```

## 🏗️ Architecture

```
portfwd/
├── main.go                     # CLI entry point (Cobra commands)
├── go.mod                      # Go module
├── internal/
│   ├── config/
│   │   ├── config.go           # Configuration & profiles
│   │   └── state.go            # Session state persistence
│   ├── daemon/
│   │   ├── client.go           # IPC client for CLI
│   │   ├── daemon.go           # Background daemon logic
│   │   ├── protocol.go         # IPC protocol definitions
│   │   └── server.go           # Unix socket server
│   ├── k8s/
│   │   └── client.go           # Kubernetes API client
│   ├── logger/
│   │   └── logger.go           # Debug logging system
│   ├── portforward/
│   │   └── manager.go          # Port-forward connection manager
│   └── ui/
│       ├── app.go              # Bubble Tea application
│       ├── styles.go           # Lipgloss styles
│       └── views.go            # UI components
└── README.md
```

## 🛠️ Dependencies

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Style definitions
- [Bubbles](https://github.com/charmbracelet/bubbles) - TUI components
- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [client-go](https://github.com/kubernetes/client-go) - Kubernetes client

## 📝 Requirements

- Go 1.21+
- Access to a Kubernetes cluster
- Valid kubeconfig (`~/.kube/config` or `KUBECONFIG` env var)

## 🐛 Troubleshooting

### Enable Debug Mode

```bash
# TUI mode with debug
portfwd --debug
# Press 'g' to view debug logs in TUI

# Daemon with debug
portfwd --debug daemon start --foreground
# Check logs at ~/.config/portfwd/debug.log
```

### Common Issues

**Port already in use:**
- Check if another process uses the port: `lsof -i :<port>`
- Use a different local port

**Permission denied for port < 1024:**
- Use ports above 1024 (e.g., 8080 instead of 80)
- Or run with sudo (not recommended)

**Connection refused:**
- Check if the target pod/service is running
- Verify the remote port is correct
- Check pod logs for errors

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## 📄 License

MIT License - see [LICENSE](LICENSE) for details.

---

<p align="center">
  Made with ❤️ for the Kubernetes community
</p>
