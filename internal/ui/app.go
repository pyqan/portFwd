package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/pyqan/portFwd/internal/config"
	"github.com/pyqan/portFwd/internal/k8s"
	"github.com/pyqan/portFwd/internal/logger"
	"github.com/pyqan/portFwd/internal/portforward"
)

// View represents the current view
type View int

const (
	ViewConnections View = iota
	ViewResourceType
	ViewNamespaces
	ViewPods
	ViewServices
	ViewPortInput
	ViewConnecting
	ViewConfirm
	ViewLogs
	ViewHelp
	ViewDebug
)

// ResourceType represents the type of resource to forward
type ResourceType int

const (
	ResourceTypePod ResourceType = iota
	ResourceTypeService
)

// Model is the main application model
type Model struct {
	// Kubernetes client
	k8sClient *k8s.Client

	// Port forward manager
	pfManager *portforward.Manager

	// Config
	config *config.Config

	// Current view
	view     View
	prevView View

	// Selection state
	namespaces           []string
	pods                 []k8s.PodInfo
	services             []k8s.ServiceInfo
	selectedNamespace    int
	selectedPod          int
	selectedService      int
	selectedConn         int
	selectedResourceType ResourceType

	// Current namespace
	currentNamespace string

	// Port input
	localPortInput  textinput.Model
	remotePortInput textinput.Model
	focusedInput    int

	// Selected target for port forward
	targetPod     string
	targetService string
	
	// Current connecting connection (for log display)
	connectingConnID string

	// Confirm dialog
	confirmTitle   string
	confirmMessage string
	confirmAction  func() tea.Cmd

	// UI state
	width   int
	height  int
	err     error
	message string
	loading bool

	// Context
	k8sContext string

	// Search/filter
	searchMode  bool
	searchQuery string

	// Global log messages (last N events)
	globalLogs    []string
	maxGlobalLogs int
	
	// Viewing logs for specific connection
	viewingLogsConnID string
	
	// Debug mode
	debugMode       bool
	debugScrollOffset int
	
	// Session restoration state
	restoring        bool
	restoringCurrent int
	restoringTotal   int
}

// Messages
type (
	errMsg             struct{ err error }
	namespacesMsg      []string
	podsMsg            []k8s.PodInfo
	servicesMsg        []k8s.ServiceInfo
	portForwardStarted struct{ id string }
	portForwardStopped struct{ id string }
	portForwardFailed  struct{ err error }
	connectionsUpdated struct{}
	contextMsg         string
	tickMsg            time.Time
	
	// Session restoration messages
	restorationStarted  struct{ total int }
	restorationProgress struct{ current, total int }
	restorationComplete struct{}
)

// tickCmd returns a command that sends tick messages for UI updates
func tickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// NewModel creates a new UI model
func NewModel(k8sClient *k8s.Client, pfManager *portforward.Manager, cfg *config.Config) Model {
	localInput := textinput.New()
	localInput.Placeholder = "8080"
	localInput.CharLimit = 5
	localInput.Width = 10
	localInput.Cursor.Style = CursorStyle
	localInput.TextStyle = InputStyle
	localInput.PlaceholderStyle = PlaceholderStyle

	remoteInput := textinput.New()
	remoteInput.Placeholder = "80"
	remoteInput.CharLimit = 5
	remoteInput.Width = 10
	remoteInput.Cursor.Style = CursorStyle
	remoteInput.TextStyle = InputStyle
	remoteInput.PlaceholderStyle = PlaceholderStyle

	return Model{
		k8sClient:       k8sClient,
		pfManager:       pfManager,
		config:          cfg,
		view:            ViewConnections,
		localPortInput:  localInput,
		remotePortInput: remoteInput,
		width:           80,
		height:          24,
		globalLogs:      make([]string, 0),
		maxGlobalLogs:   5,
	}
}

// addLog adds a message to the global log
func (m *Model) addLog(msg string) {
	timestamp := time.Now().Format("15:04:05")
	m.globalLogs = append(m.globalLogs, fmt.Sprintf("[%s] %s", timestamp, msg))
	if len(m.globalLogs) > m.maxGlobalLogs {
		m.globalLogs = m.globalLogs[len(m.globalLogs)-m.maxGlobalLogs:]
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.loadContext(),
		m.loadNamespaces(),
	)
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Allow quit even during restoration
		switch msg.String() {
		case "ctrl+c", "q":
			// Save state BEFORE stopping connections
			saveSessionState(m.pfManager)
			// Then stop all connections
			m.pfManager.StopAll()
			return m, tea.Quit
		}
		
		// Block all other keys while restoring session
		if m.restoring {
			return m, nil
		}
		
		// Global keys (not during restoration)
		switch msg.String() {
		case "?":
			if m.view != ViewHelp {
				m.prevView = m.view
				m.view = ViewHelp
				return m, nil
			}
		case "g":
			// Debug logs view (only in debug mode)
			if m.debugMode && m.view != ViewDebug {
				m.prevView = m.view
				m.view = ViewDebug
				m.debugScrollOffset = 0
				return m, nil
			}
		case "esc":
			return m.handleEsc()
		}

		// View-specific keys
		switch m.view {
		case ViewConnections:
			return m.updateConnections(msg)
		case ViewResourceType:
			return m.updateResourceType(msg)
		case ViewNamespaces:
			return m.updateNamespaces(msg)
		case ViewPods:
			return m.updatePods(msg)
		case ViewServices:
			return m.updateServices(msg)
		case ViewPortInput:
			return m.updatePortInput(msg)
		case ViewConnecting:
			return m.updateConnecting(msg)
		case ViewConfirm:
			return m.updateConfirm(msg)
		case ViewLogs:
			return m.updateLogs(msg)
		case ViewHelp:
			return m.updateHelp(msg)
		case ViewDebug:
			return m.updateDebug(msg)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case errMsg:
		m.err = msg.err
		m.loading = false

	case contextMsg:
		m.k8sContext = string(msg)

	case namespacesMsg:
		m.namespaces = msg
		m.loading = false

	case podsMsg:
		m.pods = msg
		m.loading = false

	case servicesMsg:
		m.services = msg
		m.loading = false

	case portForwardStarted:
		m.message = fmt.Sprintf("Port forward started: %s", msg.id)
		m.view = ViewConnections
		m.connectingConnID = ""

	case portForwardStopped:
		m.message = fmt.Sprintf("Port forward stopped: %s", msg.id)

	case portForwardFailed:
		m.err = msg.err
		m.view = ViewConnections
		m.connectingConnID = ""

	case connectionsUpdated:
		// Refresh view

	case tickMsg:
		// Continue ticking while connecting or restoring
		if m.view == ViewConnecting || m.restoring {
			return m, tickCmd()
		}
	
	// Session restoration messages
	case restorationStarted:
		m.restoring = true
		m.restoringCurrent = 0
		m.restoringTotal = msg.total
		m.loading = true
		return m, tickCmd()
	
	case restorationProgress:
		m.restoringCurrent = msg.current
		m.restoringTotal = msg.total
	
	case restorationComplete:
		m.restoring = false
		m.restoringCurrent = 0
		m.restoringTotal = 0
		m.loading = false
	}

	return m, nil
}

// View renders the UI
func (m Model) View() string {
	var b strings.Builder

	// Header
	b.WriteString(RenderHeader(m.k8sContext, m.currentNamespace, m.width))
	b.WriteString("\n")

	// Main content
	contentHeight := m.height - 8 // Account for header, help, messages
	content := m.renderContent(contentHeight)
	b.WriteString(content)
	b.WriteString("\n")

	// Messages
	if m.err != nil {
		b.WriteString(RenderError(m.err.Error(), m.width))
		b.WriteString("\n")
	} else if m.message != "" {
		b.WriteString(RenderSuccess(m.message, m.width))
		b.WriteString("\n")
	} else if m.restoring {
		msg := fmt.Sprintf("Restoring connections... %d/%d", m.restoringCurrent, m.restoringTotal)
		b.WriteString(RenderLoading(msg))
		b.WriteString("\n")
	} else if m.loading {
		b.WriteString(RenderLoading("Loading..."))
		b.WriteString("\n")
	}

	// Help (show limited help during restoration)
	if m.restoring {
		b.WriteString(RenderHelp("restoring"))
	} else {
		b.WriteString(RenderHelp(m.viewName()))
	}

	return b.String()
}

func (m Model) renderContent(height int) string {
	switch m.view {
	case ViewConnections:
		connections := m.pfManager.GetConnections()
		return RenderConnectionList(connections, m.selectedConn, m.width-4, height)

	case ViewResourceType:
		return RenderResourceTypeMenu(int(m.selectedResourceType), m.width-4)

	case ViewNamespaces:
		return RenderNamespaceList(m.namespaces, m.selectedNamespace, m.width-4, height)

	case ViewPods:
		return RenderPodList(m.pods, m.selectedPod, m.width-4, height)

	case ViewServices:
		return RenderServiceList(m.services, m.selectedService, m.width-4, height)

	case ViewPortInput:
		return RenderPortInput(
			m.localPortInput,
			m.remotePortInput,
			m.width-4,
		)

	case ViewConnecting:
		var logs []string
		title := "Connecting..."
		if m.connectingConnID != "" {
			if conn, ok := m.pfManager.GetConnection(m.connectingConnID); ok {
				logs = conn.GetLogs()
				info := conn.GetConnectionInfo()
				resType := "pod"
				if info.ResourceType == portforward.ResourceService {
					resType = "svc"
				}
				title = fmt.Sprintf("Connecting to %s/%s/%s", info.Namespace, resType, info.ResourceName)
			}
		}
		return RenderLogWindow(logs, title, m.width-4, height-4)

	case ViewConfirm:
		return RenderConfirmDialog(m.confirmTitle, m.confirmMessage, m.width/2)

	case ViewLogs:
		var logs []string
		title := "Connection Logs"
		if m.viewingLogsConnID != "" {
			if conn, ok := m.pfManager.GetConnection(m.viewingLogsConnID); ok {
				logs = conn.GetLogs()
				info := conn.GetConnectionInfo()
				resType := "pod"
				if info.ResourceType == portforward.ResourceService {
					resType = "svc"
				}
				title = fmt.Sprintf("Logs: %s/%s/%s", info.Namespace, resType, info.ResourceName)
			}
		}
		return RenderLogWindow(logs, title, m.width-4, height-2)

	case ViewHelp:
		return RenderHelpScreen(m.width-4, height, m.debugMode)

	case ViewDebug:
		return RenderDebugLogs(m.width-4, height, m.debugScrollOffset)

	default:
		return ""
	}
}

func (m Model) viewName() string {
	switch m.view {
	case ViewConnections:
		return "connections"
	case ViewResourceType:
		return "resource_type"
	case ViewNamespaces:
		return "namespace"
	case ViewPods:
		return "pod"
	case ViewServices:
		return "service"
	case ViewPortInput:
		return "port_input"
	case ViewConnecting:
		return "connecting"
	case ViewConfirm:
		return "confirm"
	case ViewLogs:
		return "logs"
	case ViewHelp:
		return "help"
	case ViewDebug:
		return "debug"
	default:
		return ""
	}
}

func (m Model) handleEsc() (tea.Model, tea.Cmd) {
	switch m.view {
	case ViewResourceType:
		m.view = ViewConnections
	case ViewNamespaces:
		m.view = ViewResourceType
	case ViewPods, ViewServices:
		m.view = ViewNamespaces
	case ViewPortInput:
		m.view = m.prevView
	case ViewConfirm:
		m.view = m.prevView
	case ViewLogs:
		m.viewingLogsConnID = ""
		m.view = ViewConnections
	case ViewHelp:
		m.view = m.prevView
	case ViewDebug:
		m.view = m.prevView
	}
	m.err = nil
	m.message = ""
	return m, nil
}

// Connection view handlers
func (m Model) updateConnections(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	connections := m.pfManager.GetConnections()

	switch msg.String() {
	case "up", "k":
		if m.selectedConn > 0 {
			m.selectedConn--
		}
		return m, nil
	case "down", "j":
		if m.selectedConn < len(connections)-1 {
			m.selectedConn++
		}
		return m, nil
	case "enter":
		// Toggle connection: active -> stop, stopped/error -> reconnect
		if len(connections) > 0 && m.selectedConn < len(connections) {
			conn := connections[m.selectedConn]
			info := conn.GetConnectionInfo()
			if info.Status == portforward.StatusActive {
				// Stop active connection
				return m, m.stopPortForward(info.ID)
			} else if info.Status == portforward.StatusStopped || info.Status == portforward.StatusError {
				// Reconnect stopped/error connection
				m.view = ViewConnecting
				if info.ResourceType == portforward.ResourceService {
					m.connectingConnID = info.ID
					return m, tea.Batch(
						m.startPortForwardToServiceAsync(info.Namespace, info.ResourceName, info.LocalPort, info.RemotePort),
						tickCmd(),
					)
				}
				m.connectingConnID = info.ID
				return m, tea.Batch(
					m.startPortForwardToPodAsync(info.Namespace, info.ResourceName, info.LocalPort, info.RemotePort),
					tickCmd(),
				)
			}
		}
		return m, nil
	case "n":
		// New port forward - go to resource type selection
		m.view = ViewResourceType
		m.selectedResourceType = ResourceTypePod
		return m, nil
	case "d":
		// Disconnect selected
		if len(connections) > 0 && m.selectedConn < len(connections) {
			conn := connections[m.selectedConn]
			info := conn.GetConnectionInfo()
			return m, m.stopPortForward(info.ID)
		}
	case "D":
		// Disconnect all - show confirm
		if len(connections) > 0 {
			m.confirmTitle = "Disconnect All"
			m.confirmMessage = fmt.Sprintf("Stop all %d connections?", len(connections))
			m.confirmAction = func() tea.Cmd {
				m.pfManager.StopAll()
				return func() tea.Msg { return connectionsUpdated{} }
			}
			m.prevView = m.view
			m.view = ViewConfirm
		}
	case "r":
		// Reconnect selected
		if len(connections) > 0 && m.selectedConn < len(connections) {
			conn := connections[m.selectedConn]
			info := conn.GetConnectionInfo()
			if info.Status == portforward.StatusStopped || info.Status == portforward.StatusError {
				m.view = ViewConnecting
				if info.ResourceType == portforward.ResourceService {
					m.connectingConnID = info.ID
					return m, tea.Batch(
						m.startPortForwardToServiceAsync(info.Namespace, info.ResourceName, info.LocalPort, info.RemotePort),
						tickCmd(),
					)
				}
				m.connectingConnID = info.ID
				return m, tea.Batch(
					m.startPortForwardToPodAsync(info.Namespace, info.ResourceName, info.LocalPort, info.RemotePort),
					tickCmd(),
				)
			}
		}
	case "x", "delete", "backspace":
		// Delete selected connection completely
		if len(connections) > 0 && m.selectedConn < len(connections) {
			conn := connections[m.selectedConn]
			info := conn.GetConnectionInfo()
			m.pfManager.DeleteConnection(info.ID)
			// Adjust selection if needed
			if m.selectedConn >= len(connections)-1 && m.selectedConn > 0 {
				m.selectedConn--
			}
			return m, func() tea.Msg { return connectionsUpdated{} }
		}
	case "l":
		// View logs for selected connection
		if len(connections) > 0 && m.selectedConn < len(connections) {
			conn := connections[m.selectedConn]
			m.viewingLogsConnID = conn.GetConnectionInfo().ID
			m.view = ViewLogs
		}
	}
	return m, nil
}

// Resource type view handlers
func (m Model) updateResourceType(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.selectedResourceType > 0 {
			m.selectedResourceType--
		}
	case "down", "j":
		if m.selectedResourceType < ResourceTypeService {
			m.selectedResourceType++
		}
	case "enter":
		m.view = ViewNamespaces
		m.selectedNamespace = 0
		return m, m.loadNamespaces()
	case "p":
		// Quick select Pod
		m.selectedResourceType = ResourceTypePod
		m.view = ViewNamespaces
		m.selectedNamespace = 0
		return m, m.loadNamespaces()
	case "s":
		// Quick select Service
		m.selectedResourceType = ResourceTypeService
		m.view = ViewNamespaces
		m.selectedNamespace = 0
		return m, m.loadNamespaces()
	}
	return m, nil
}

// Namespace view handlers
func (m Model) updateNamespaces(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.selectedNamespace > 0 {
			m.selectedNamespace--
		}
	case "down", "j":
		if m.selectedNamespace < len(m.namespaces)-1 {
			m.selectedNamespace++
		}
	case "enter":
		if len(m.namespaces) > 0 {
			m.currentNamespace = m.namespaces[m.selectedNamespace]
			// Go to selected resource type
			if m.selectedResourceType == ResourceTypePod {
				m.view = ViewPods
				m.selectedPod = 0
				return m, m.loadPods()
			} else {
				m.view = ViewServices
				m.selectedService = 0
				return m, m.loadServices()
			}
		}
	}
	return m, nil
}

// Pod view handlers
func (m Model) updatePods(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.selectedPod > 0 {
			m.selectedPod--
		}
	case "down", "j":
		if m.selectedPod < len(m.pods)-1 {
			m.selectedPod++
		}
	case "enter":
		if len(m.pods) > 0 && m.selectedPod < len(m.pods) {
			pod := m.pods[m.selectedPod]
			m.targetPod = pod.Name
			m.targetService = ""

			// Pre-fill remote port if pod has ports
			if len(pod.Ports) > 0 {
				m.remotePortInput.SetValue(fmt.Sprintf("%d", pod.Ports[0].ContainerPort))
				m.localPortInput.SetValue(fmt.Sprintf("%d", pod.Ports[0].ContainerPort))
			} else {
				m.remotePortInput.SetValue("")
				m.localPortInput.SetValue("")
			}

			m.focusedInput = 0
			m.localPortInput.Focus()
			m.prevView = m.view
			m.view = ViewPortInput
		}
	}
	return m, nil
}

// Service view handlers
func (m Model) updateServices(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.selectedService > 0 {
			m.selectedService--
		}
	case "down", "j":
		if m.selectedService < len(m.services)-1 {
			m.selectedService++
		}
	case "enter":
		if len(m.services) > 0 && m.selectedService < len(m.services) {
			svc := m.services[m.selectedService]
			m.targetService = svc.Name
			m.targetPod = ""

			// Pre-fill remote port if service has ports
			if len(svc.Ports) > 0 {
				m.remotePortInput.SetValue(fmt.Sprintf("%d", svc.Ports[0].Port))
				m.localPortInput.SetValue(fmt.Sprintf("%d", svc.Ports[0].Port))
			} else {
				m.remotePortInput.SetValue("")
				m.localPortInput.SetValue("")
			}

			m.focusedInput = 0
			m.localPortInput.Focus()
			m.prevView = m.view
			m.view = ViewPortInput
		}
	}
	return m, nil
}

// Port input handlers
func (m Model) updatePortInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab", "down":
		m.focusedInput = (m.focusedInput + 1) % 2
		if m.focusedInput == 0 {
			m.localPortInput.Focus()
			m.remotePortInput.Blur()
		} else {
			m.remotePortInput.Focus()
			m.localPortInput.Blur()
		}
	case "shift+tab", "up":
		m.focusedInput = (m.focusedInput + 1) % 2
		if m.focusedInput == 0 {
			m.localPortInput.Focus()
			m.remotePortInput.Blur()
		} else {
			m.remotePortInput.Focus()
			m.localPortInput.Blur()
		}
	case "enter":
		localPort, err := strconv.Atoi(m.localPortInput.Value())
		if err != nil || localPort <= 0 || localPort > 65535 {
			m.err = fmt.Errorf("invalid local port")
			return m, nil
		}
		remotePort, err := strconv.Atoi(m.remotePortInput.Value())
		if err != nil || remotePort <= 0 || remotePort > 65535 {
			m.err = fmt.Errorf("invalid remote port")
			return m, nil
		}

		m.err = nil
		m.view = ViewConnecting
		
		if m.targetService != "" {
			// Port-forward to Service (like kubectl port-forward svc/...)
			m.connectingConnID = fmt.Sprintf("%s/svc/%s:%d->%d", m.currentNamespace, m.targetService, localPort, remotePort)
			return m, tea.Batch(
				m.startPortForwardToServiceAsync(m.currentNamespace, m.targetService, localPort, remotePort),
				tickCmd(),
			)
		} else if m.targetPod != "" {
			// Port-forward to Pod
			m.connectingConnID = fmt.Sprintf("%s/pod/%s:%d->%d", m.currentNamespace, m.targetPod, localPort, remotePort)
			return m, tea.Batch(
				m.startPortForwardToPodAsync(m.currentNamespace, m.targetPod, localPort, remotePort),
				tickCmd(),
			)
		} else {
			m.err = fmt.Errorf("no target specified")
			m.view = ViewPortInput
			return m, nil
		}

	default:
		// Handle text input
		var cmd tea.Cmd
		if m.focusedInput == 0 {
			m.localPortInput, cmd = m.localPortInput.Update(msg)
		} else {
			m.remotePortInput, cmd = m.remotePortInput.Update(msg)
		}
		return m, cmd
	}
	return m, nil
}

// Connecting view handlers
func (m Model) updateConnecting(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Cancel connection attempt
		if m.connectingConnID != "" {
			m.pfManager.StopPortForward(m.connectingConnID)
		}
		m.connectingConnID = ""
		m.view = ViewConnections
	}
	return m, nil
}

// Confirm dialog handlers
func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if m.confirmAction != nil {
			cmd := m.confirmAction()
			m.view = m.prevView
			m.confirmAction = nil
			return m, cmd
		}
	case "n", "N":
		m.view = m.prevView
		m.confirmAction = nil
	}
	return m, nil
}

// Logs view handlers
func (m Model) updateLogs(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "l":
		m.viewingLogsConnID = ""
		m.view = ViewConnections
	}
	return m, nil
}

// Help view handlers
func (m Model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "?":
		m.view = m.prevView
	}
	return m, nil
}

// Debug view handlers
func (m Model) updateDebug(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	entries := logger.GetEntries()
	maxScroll := len(entries) - 10
	if maxScroll < 0 {
		maxScroll = 0
	}

	switch msg.String() {
	case "esc", "g":
		m.view = m.prevView
	case "up", "k":
		if m.debugScrollOffset > 0 {
			m.debugScrollOffset--
		}
	case "down", "j":
		if m.debugScrollOffset < maxScroll {
			m.debugScrollOffset++
		}
	case "home":
		m.debugScrollOffset = 0
	case "end":
		m.debugScrollOffset = maxScroll
	case "pgup":
		m.debugScrollOffset -= 10
		if m.debugScrollOffset < 0 {
			m.debugScrollOffset = 0
		}
	case "pgdown":
		m.debugScrollOffset += 10
		if m.debugScrollOffset > maxScroll {
			m.debugScrollOffset = maxScroll
		}
	}
	return m, nil
}

// Commands
func (m Model) loadContext() tea.Cmd {
	return func() tea.Msg {
		ctx, err := m.k8sClient.GetCurrentContext()
		if err != nil {
			return contextMsg("")
		}
		return contextMsg(ctx)
	}
}

func (m Model) loadNamespaces() tea.Cmd {
	return func() tea.Msg {
		m.loading = true
		namespaces, err := m.k8sClient.GetNamespaces(context.Background())
		if err != nil {
			return errMsg{err}
		}
		return namespacesMsg(namespaces)
	}
}

func (m Model) loadPods() tea.Cmd {
	return func() tea.Msg {
		m.loading = true
		pods, err := m.k8sClient.GetRunningPods(context.Background(), m.currentNamespace)
		if err != nil {
			return errMsg{err}
		}
		return podsMsg(pods)
	}
}

func (m Model) loadServices() tea.Cmd {
	return func() tea.Msg {
		m.loading = true
		services, err := m.k8sClient.GetServices(context.Background(), m.currentNamespace)
		if err != nil {
			return errMsg{err}
		}
		return servicesMsg(services)
	}
}

func (m Model) startPortForwardToPodAsync(namespace, pod string, localPort, remotePort int) tea.Cmd {
	return func() tea.Msg {
		_, err := m.pfManager.StartPortForwardToPod(context.Background(), namespace, pod, localPort, remotePort)
		if err != nil {
			return portForwardFailed{err: err}
		}
		return portForwardStarted{id: fmt.Sprintf("%s/pod/%s:%d->%d", namespace, pod, localPort, remotePort)}
	}
}

func (m Model) startPortForwardToServiceAsync(namespace, svc string, localPort, remotePort int) tea.Cmd {
	return func() tea.Msg {
		_, err := m.pfManager.StartPortForwardToService(context.Background(), namespace, svc, localPort, remotePort)
		if err != nil {
			return portForwardFailed{err: err}
		}
		return portForwardStarted{id: fmt.Sprintf("%s/svc/%s:%d->%d", namespace, svc, localPort, remotePort)}
	}
}

func (m Model) stopPortForward(id string) tea.Cmd {
	return func() tea.Msg {
		err := m.pfManager.StopPortForward(id)
		if err != nil {
			return errMsg{err}
		}
		return portForwardStopped{id: id}
	}
}

// KeyMap defines key bindings
type KeyMap struct {
	Up       key.Binding
	Down     key.Binding
	Enter    key.Binding
	Esc      key.Binding
	Quit     key.Binding
	Help     key.Binding
	New      key.Binding
	Delete   key.Binding
	Reconnect key.Binding
}

// DefaultKeyMap returns the default key bindings
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Esc: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		New: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "new"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "delete"),
		),
		Reconnect: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "reconnect"),
		),
	}
}

// Run starts the TUI application
func Run(k8sClient *k8s.Client, pfManager *portforward.Manager, cfg *config.Config, debugMode bool) error {
	model := NewModel(k8sClient, pfManager, cfg)
	model.debugMode = debugMode
	p := tea.NewProgram(model, tea.WithAltScreen())

	// Set up onChange callback to refresh UI
	pfManager.SetOnChange(func() {
		p.Send(connectionsUpdated{})
	})

	// Load and restore previous session
	go restorePreviousSession(k8sClient, pfManager, p)

	_, err := p.Run()
	return err
}

// restorePreviousSession loads and restores connections from previous session
func restorePreviousSession(k8sClient *k8s.Client, pfManager *portforward.Manager, p *tea.Program) {
	state, err := config.LoadState()
	if err != nil || len(state.Connections) == 0 {
		return
	}

	total := len(state.Connections)
	
	// Signal restoration started
	p.Send(restorationStarted{total: total})
	
	ctx := context.Background()
	
	for i, saved := range state.Connections {
		// Update progress
		p.Send(restorationProgress{current: i + 1, total: total})
		
		resourceType := portforward.ResourcePod
		if saved.ResourceType == "service" {
			resourceType = portforward.ResourceService
		}
		
		if !saved.WasActive {
			// Restore as stopped - don't try to connect
			pfManager.AddStoppedConnection(saved.Namespace, resourceType, saved.ResourceName, saved.LocalPort, saved.RemotePort)
			continue
		}
		
		// Was active - check availability and try to connect
		available := false
		if saved.ResourceType == "service" {
			_, err := k8sClient.GetService(ctx, saved.Namespace, saved.ResourceName)
			available = err == nil
		} else {
			pod, err := k8sClient.GetPod(ctx, saved.Namespace, saved.ResourceName)
			available = err == nil && pod.Status == "Running"
		}
		
		if !available {
			// Resource not available - add as stopped
			pfManager.AddStoppedConnection(saved.Namespace, resourceType, saved.ResourceName, saved.LocalPort, saved.RemotePort)
			continue
		}
		
		// Try to restore active connection
		var restoreErr error
		if saved.ResourceType == "service" {
			_, restoreErr = pfManager.StartPortForwardToService(ctx, saved.Namespace, saved.ResourceName, saved.LocalPort, saved.RemotePort)
		} else {
			_, restoreErr = pfManager.StartPortForwardToPod(ctx, saved.Namespace, saved.ResourceName, saved.LocalPort, saved.RemotePort)
		}
		
		if restoreErr != nil {
			// Failed - add as stopped
			pfManager.AddStoppedConnection(saved.Namespace, resourceType, saved.ResourceName, saved.LocalPort, saved.RemotePort)
		}
	}
	
	// Signal restoration complete
	p.Send(restorationComplete{})
	p.Send(connectionsUpdated{})
}

// saveSessionState saves all connections to state file
func saveSessionState(pfManager *portforward.Manager) {
	all := pfManager.GetAllConnectionsForSave()
	
	state := &config.SessionState{
		Connections: make([]config.SavedConnection, len(all)),
	}
	
	for i, conn := range all {
		state.Connections[i] = config.SavedConnection{
			Namespace:    conn.Namespace,
			ResourceType: conn.ResourceType,
			ResourceName: conn.ResourceName,
			LocalPort:    conn.LocalPort,
			RemotePort:   conn.RemotePort,
			WasActive:    conn.WasActive,
		}
	}
	
	state.Save()
}
