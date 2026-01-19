# ⚡ PortFwd - Kubernetes Port Forward Manager

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.21+-00ADD8?style=for-the-badge&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/Kubernetes-1.25+-326CE5?style=for-the-badge&logo=kubernetes" alt="Kubernetes">
  <img src="https://img.shields.io/badge/License-MIT-green?style=for-the-badge" alt="License">
</p>

A powerful TUI (Terminal User Interface) application for managing Kubernetes port-forward connections with style.

## ✨ Features

- 🎨 **Beautiful TUI** - Cyberpunk-styled terminal interface
- 🔌 **Multiple Connections** - Manage many port-forwards simultaneously
- 📁 **Profile Support** - Save and quickly restore port-forward configurations
- 🔍 **Interactive Selection** - Navigate namespaces, pods, and services with ease
- ⚡ **Fast & Lightweight** - Single binary, no dependencies
- 🔄 **Auto-reconnect** - Easy reconnection of dropped connections

## 📦 Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/alexsashin/portfwd.git
cd portfwd

# Build
go build -o portfwd .

# Install to PATH (optional)
sudo mv portfwd /usr/local/bin/
```

### Go Install

```bash
go install github.com/alexsashin/portfwd@latest
```

## 🚀 Quick Start

### Interactive Mode (TUI)

Simply run `portfwd` to start the interactive terminal interface:

```bash
portfwd
```

### Command Line

```bash
# Forward local port 8080 to pod's port 80
portfwd forward -n default -p my-pod -l 8080 -r 80

# Forward using same port numbers
portfwd forward -n default -p nginx-pod -l 3000 -r 3000

# List pods in a namespace
portfwd list pods -n kube-system

# List namespaces
portfwd list namespaces
```

## 🎮 TUI Controls

### Main View (Connections)

| Key | Action |
|-----|--------|
| `↑/↓` or `j/k` | Navigate |
| `n` | New port-forward |
| `d` | Disconnect selected |
| `D` | Disconnect all |
| `r` | Reconnect |
| `q` | Quit |

### Selection Views

| Key | Action |
|-----|--------|
| `↑/↓` | Navigate |
| `Enter` | Select |
| `Tab` | Switch Pods/Services |
| `Esc` | Go back |
| `/` | Search |

### Port Input

| Key | Action |
|-----|--------|
| `Tab` | Next field |
| `Enter` | Confirm |
| `Esc` | Cancel |

## 📋 Profiles

Profiles allow you to save and quickly restore port-forward configurations.

### Configuration File

Profiles are stored in `~/.config/portfwd/config.yaml`:

```yaml
profiles:
  - name: development
    description: Local development setup
    forwards:
      - namespace: default
        pod: api-server-abc123
        localPort: 8080
        remotePort: 8080
      - namespace: default
        pod: postgres-xyz789
        localPort: 5432
        remotePort: 5432
      - namespace: monitoring
        service: grafana
        localPort: 3000
        remotePort: 3000

  - name: debugging
    description: Debug services
    forwards:
      - namespace: kube-system
        pod: coredns-abc123
        localPort: 9153
        remotePort: 9153
```

### Profile Commands

```bash
# List all profiles
portfwd profile list

# Show profile details
portfwd profile show development

# Start all forwards in a profile
portfwd profile start development

# Delete a profile
portfwd profile delete old-profile
```

## 🔧 CLI Reference

### Global Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--namespace` | `-n` | Kubernetes namespace |
| `--config` | `-c` | Config file path |

### Commands

#### `portfwd`

Start the interactive TUI application.

#### `portfwd forward`

Start a port-forward from command line.

```bash
portfwd forward -n <namespace> -p <pod> -l <local-port> [-r <remote-port>]
portfwd forward -n <namespace> -s <service> -l <local-port> [-r <remote-port>]
```

| Flag | Short | Description |
|------|-------|-------------|
| `--pod` | `-p` | Pod name |
| `--service` | `-s` | Service name |
| `--local` | `-l` | Local port |
| `--remote` | `-r` | Remote port (defaults to local) |

#### `portfwd list`

List Kubernetes resources.

```bash
portfwd list namespaces    # or: portfwd list ns
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

## 🏗️ Architecture

```
portfwd/
├── main.go                     # CLI entry point
├── go.mod                      # Go module
├── internal/
│   ├── k8s/
│   │   └── client.go           # Kubernetes API client
│   ├── portforward/
│   │   └── manager.go          # Port-forward connection manager
│   ├── config/
│   │   └── config.go           # Configuration & profiles
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

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## 📄 License

MIT License - see [LICENSE](LICENSE) for details.

---

<p align="center">
  Made with ❤️ for the Kubernetes community
</p>
