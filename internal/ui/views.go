package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/alexsashin/portfwd/internal/k8s"
	"github.com/alexsashin/portfwd/internal/portforward"
)

// RenderResourceTypeMenu renders the resource type selection menu
func RenderResourceTypeMenu(selected int, width int) string {
	var b strings.Builder

	title := SubtitleStyle.Render("🎯 Select Resource Type")
	b.WriteString(title + "\n\n")

	types := []struct {
		icon string
		name string
		desc string
	}{
		{"🚀", "Pods", "Forward to a specific pod"},
		{"🌐", "Services", "Forward to a service"},
	}

	for i, t := range types {
		var item string
		if i == selected {
			item = SelectedItemStyle.Render(fmt.Sprintf(" ▶ %s %s ", t.icon, t.name))
			item += "\n" + ListItemStyle.Foreground(ColorTextDim).Render(fmt.Sprintf("     %s", t.desc))
		} else {
			item = ListItemStyle.Render(fmt.Sprintf("   %s %s", t.icon, t.name))
			item += "\n" + ListItemStyle.Foreground(ColorMuted).Render(fmt.Sprintf("     %s", t.desc))
		}
		b.WriteString(item + "\n")
	}

	// Quick keys hint
	b.WriteString("\n" + HelpDescStyle.Render("   Quick: ") + HelpKeyStyle.Render("p") + HelpDescStyle.Render(" pods  ") + HelpKeyStyle.Render("s") + HelpDescStyle.Render(" services"))

	return BoxStyle.Width(width).Render(b.String())
}

// RenderNamespaceList renders a list of namespaces with scrolling
func RenderNamespaceList(namespaces []string, selected int, width int, maxHeight int) string {
	var b strings.Builder

	title := SubtitleStyle.Render("📁 Select Namespace")
	b.WriteString(title + "\n\n")

	total := len(namespaces)
	if total == 0 {
		b.WriteString(ListItemStyle.Foreground(ColorMuted).Render("   No namespaces found"))
		return BoxStyle.Width(width).Render(b.String())
	}

	// Calculate visible items (1 line per item, reserve 4 lines for title + padding + scroll indicators)
	visibleItems := maxHeight - 4
	if visibleItems < 3 {
		visibleItems = 3
	}

	// Calculate scroll offset to keep selected item visible
	offset := calculateOffset(selected, total, visibleItems)

	// Show "more above" indicator
	if offset > 0 {
		b.WriteString(ScrollIndicatorStyle.Render(fmt.Sprintf("   ↑ %d more above\n", offset)))
	}

	// Render visible items
	endIdx := offset + visibleItems
	if endIdx > total {
		endIdx = total
	}

	for i := offset; i < endIdx; i++ {
		ns := namespaces[i]
		var item string
		if i == selected {
			item = SelectedItemStyle.Render(fmt.Sprintf(" ▶ %s ", ns))
		} else {
			item = ListItemStyle.Render(fmt.Sprintf("   %s", ns))
		}
		b.WriteString(item + "\n")
	}

	// Show "more below" indicator
	remaining := total - endIdx
	if remaining > 0 {
		b.WriteString(ScrollIndicatorStyle.Render(fmt.Sprintf("   ↓ %d more below", remaining)))
	}

	return BoxStyle.Width(width).Render(b.String())
}

// RenderPodList renders a list of pods with scrolling
func RenderPodList(pods []k8s.PodInfo, selected int, width int, maxHeight int) string {
	var b strings.Builder

	title := SubtitleStyle.Render("🚀 Select Pod")
	b.WriteString(title + "\n\n")

	total := len(pods)
	if total == 0 {
		b.WriteString(ListItemStyle.Foreground(ColorMuted).Render("   No pods found"))
		return BoxStyle.Width(width).Render(b.String())
	}

	// Each pod takes 2 lines, reserve 4 for title + padding + indicators
	visibleItems := (maxHeight - 4) / 2
	if visibleItems < 2 {
		visibleItems = 2
	}

	offset := calculateOffset(selected, total, visibleItems)

	// Show "more above" indicator
	if offset > 0 {
		b.WriteString(ScrollIndicatorStyle.Render(fmt.Sprintf("   ↑ %d more above\n", offset)))
	}

	endIdx := offset + visibleItems
	if endIdx > total {
		endIdx = total
	}

	for i := offset; i < endIdx; i++ {
		pod := pods[i]
		status := getStatusStyle(pod.Status)
		ports := formatPorts(pod.Ports)

		var item string
		if i == selected {
			item = SelectedItemStyle.Render(fmt.Sprintf(" ▶ %s ", pod.Name))
			item += "\n" + ListItemStyle.Render(fmt.Sprintf("     %s %s", status, ports))
		} else {
			item = ListItemStyle.Render(fmt.Sprintf("   %s", pod.Name))
			item += "\n" + ListItemStyle.Foreground(ColorTextDim).Render(fmt.Sprintf("     %s %s", status, ports))
		}
		b.WriteString(item + "\n")
	}

	// Show "more below" indicator
	remaining := total - endIdx
	if remaining > 0 {
		b.WriteString(ScrollIndicatorStyle.Render(fmt.Sprintf("   ↓ %d more below", remaining)))
	}

	return BoxStyle.Width(width).Render(b.String())
}

// RenderServiceList renders a list of services with scrolling
func RenderServiceList(services []k8s.ServiceInfo, selected int, width int, maxHeight int) string {
	var b strings.Builder

	title := SubtitleStyle.Render("🌐 Select Service")
	b.WriteString(title + "\n\n")

	total := len(services)
	if total == 0 {
		b.WriteString(ListItemStyle.Foreground(ColorMuted).Render("   No services found"))
		return BoxStyle.Width(width).Render(b.String())
	}

	// Each service takes 2 lines, reserve 4 for title + padding + indicators
	visibleItems := (maxHeight - 4) / 2
	if visibleItems < 2 {
		visibleItems = 2
	}

	offset := calculateOffset(selected, total, visibleItems)

	// Show "more above" indicator
	if offset > 0 {
		b.WriteString(ScrollIndicatorStyle.Render(fmt.Sprintf("   ↑ %d more above\n", offset)))
	}

	endIdx := offset + visibleItems
	if endIdx > total {
		endIdx = total
	}

	for i := offset; i < endIdx; i++ {
		svc := services[i]
		ports := formatServicePorts(svc.Ports)
		svcType := lipgloss.NewStyle().Foreground(ColorMuted).Render(fmt.Sprintf("[%s]", svc.Type))

		var item string
		if i == selected {
			item = SelectedItemStyle.Render(fmt.Sprintf(" ▶ %s ", svc.Name))
			item += "\n" + ListItemStyle.Render(fmt.Sprintf("     %s %s", svcType, ports))
		} else {
			item = ListItemStyle.Render(fmt.Sprintf("   %s", svc.Name))
			item += "\n" + ListItemStyle.Foreground(ColorTextDim).Render(fmt.Sprintf("     %s %s", svcType, ports))
		}
		b.WriteString(item + "\n")
	}

	// Show "more below" indicator
	remaining := total - endIdx
	if remaining > 0 {
		b.WriteString(ScrollIndicatorStyle.Render(fmt.Sprintf("   ↓ %d more below", remaining)))
	}

	return BoxStyle.Width(width).Render(b.String())
}

// RenderConnectionList renders active port-forward connections with scrolling
func RenderConnectionList(connections []*portforward.Connection, selected int, width int, maxHeight int) string {
	var b strings.Builder

	// Header with count
	activeCount := 0
	for _, c := range connections {
		info := c.GetConnectionInfo()
		if info.Status == portforward.StatusActive {
			activeCount++
		}
	}

	title := SubtitleStyle.Render(fmt.Sprintf("⚡ Active Connections (%d)", activeCount))
	b.WriteString(title + "\n\n")

	total := len(connections)
	if total == 0 {
		b.WriteString(ListItemStyle.Foreground(ColorMuted).Render("   No active connections\n"))
		b.WriteString(ListItemStyle.Foreground(ColorMuted).Render("   Press 'n' to create new forward"))
		return BoxStyle.Width(width).Render(b.String())
	}

	// Each connection takes 2-3 lines, estimate 2.5 avg, reserve 4 for title + padding + indicators
	visibleItems := (maxHeight - 4) / 3
	if visibleItems < 2 {
		visibleItems = 2
	}

	offset := calculateOffset(selected, total, visibleItems)

	// Show "more above" indicator
	if offset > 0 {
		b.WriteString(ScrollIndicatorStyle.Render(fmt.Sprintf("   ↑ %d more above\n", offset)))
	}

	endIdx := offset + visibleItems
	if endIdx > total {
		endIdx = total
	}

	for i := offset; i < endIdx; i++ {
		conn := connections[i]
		info := conn.GetConnectionInfo()
		statusIcon := StatusIcon(string(info.Status))
		duration := formatDuration(info.Duration)

		portMapping := PortStyle.Render(fmt.Sprintf("localhost:%d → %d", info.LocalPort, info.RemotePort))
		resourcePrefix := "pod"
		if info.ResourceType == portforward.ResourceService {
			resourcePrefix = "svc"
		}
		target := NamespaceStyle.Render(info.Namespace) + "/" + resourcePrefix + "/" + PodStyle.Render(info.ResourceName)

		var item string
		if i == selected {
			item = SelectedItemStyle.Render(fmt.Sprintf(" ▶ %s %s ", statusIcon, target))
			item += "\n" + ListItemStyle.Render(fmt.Sprintf("     %s  ⏱ %s", portMapping, duration))
		} else {
			item = ListItemStyle.Render(fmt.Sprintf("   %s %s", statusIcon, target))
			item += "\n" + ListItemStyle.Foreground(ColorTextDim).Render(fmt.Sprintf("     %s  ⏱ %s", portMapping, duration))
		}

		if info.Error != "" {
			item += "\n" + StatusErrorStyle.Render(fmt.Sprintf("     ⚠ %s", info.Error))
		}

		b.WriteString(item + "\n")
	}

	// Show "more below" indicator
	remaining := total - endIdx
	if remaining > 0 {
		b.WriteString(ScrollIndicatorStyle.Render(fmt.Sprintf("   ↓ %d more below", remaining)))
	}

	return BoxStyle.Width(width).Render(b.String())
}

// RenderPortInput renders port input form
func RenderPortInput(localPort, remotePort string, focusedField int, width int) string {
	var b strings.Builder

	title := SubtitleStyle.Render("🔌 Configure Port Forward")
	b.WriteString(title + "\n\n")

	// Style for input values
	inputValueStyle := lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true)
	
	inputDimStyle := lipgloss.NewStyle().
		Foreground(ColorTextDim)

	cursorStyle := lipgloss.NewStyle().
		Foreground(ColorAccent).
		Bold(true)
	
	warningStyle := lipgloss.NewStyle().
		Foreground(ColorWarning)

	// Local port (on your machine)
	localLabel := LabelStyle.Render("Local Port:  ")
	localHint := lipgloss.NewStyle().Foreground(ColorMuted).Render(" (localhost)")
	var localValue string
	if focusedField == 0 {
		localValue = inputValueStyle.Render(localPort) + cursorStyle.Render("█")
	} else {
		if localPort == "" {
			localValue = inputDimStyle.Render("_____")
		} else {
			localValue = inputDimStyle.Render(localPort)
		}
	}
	b.WriteString(localLabel + localValue + localHint + "\n")
	
	// Warning for privileged ports
	if localPort != "" {
		if port, err := strconv.Atoi(localPort); err == nil && port > 0 && port < 1024 {
			b.WriteString(warningStyle.Render("   ⚠ Port < 1024 requires sudo") + "\n")
		} else {
			b.WriteString("\n")
		}
	} else {
		b.WriteString("\n")
	}

	// Remote port (in pod/container)
	remoteLabel := LabelStyle.Render("Remote Port: ")
	remoteHint := lipgloss.NewStyle().Foreground(ColorMuted).Render(" (pod/container)")
	var remoteValue string
	if focusedField == 1 {
		remoteValue = inputValueStyle.Render(remotePort) + cursorStyle.Render("█")
	} else {
		if remotePort == "" {
			remoteValue = inputDimStyle.Render("_____")
		} else {
			remoteValue = inputDimStyle.Render(remotePort)
		}
	}
	b.WriteString(remoteLabel + remoteValue + remoteHint + "\n\n")
	
	// Example
	if localPort != "" && remotePort != "" {
		example := lipgloss.NewStyle().Foreground(ColorSecondary).Render(
			fmt.Sprintf("   → localhost:%s  ➜  pod:%s", localPort, remotePort))
		b.WriteString(example)
	}

	return BoxStyle.Width(width).Render(b.String())
}

// RenderLogWindow renders a small log window
func RenderLogWindow(logs []string, title string, width int, maxLines int) string {
	var b strings.Builder

	titleStr := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Bold(true).
		Render("📋 " + title)
	b.WriteString(titleStr + "\n")

	if len(logs) == 0 {
		b.WriteString(ListItemStyle.Foreground(ColorMuted).Render("  No logs yet..."))
	} else {
		// Show last maxLines
		start := 0
		if len(logs) > maxLines {
			start = len(logs) - maxLines
		}
		
		for i := start; i < len(logs); i++ {
			logLine := logs[i]
			// Color based on content
			var style lipgloss.Style
			if strings.Contains(logLine, "✓") {
				style = lipgloss.NewStyle().Foreground(ColorSuccess)
			} else if strings.Contains(logLine, "✗") || strings.Contains(logLine, "Error") {
				style = lipgloss.NewStyle().Foreground(ColorError)
			} else {
				style = lipgloss.NewStyle().Foreground(ColorTextDim)
			}
			b.WriteString(style.Render("  " + logLine) + "\n")
		}
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(0, 1).
		Width(width).
		Render(b.String())
}

// RenderHelp renders help text based on current view
func RenderHelp(view string) string {
	var keys []string

	commonKeys := []string{
		HelpKeyStyle.Render("q") + HelpDescStyle.Render(" quit"),
		HelpKeyStyle.Render("?") + HelpDescStyle.Render(" help"),
	}

	switch view {
	case "resource_type":
		keys = []string{
			HelpKeyStyle.Render("↑/↓") + HelpDescStyle.Render(" navigate"),
			HelpKeyStyle.Render("enter") + HelpDescStyle.Render(" select"),
			HelpKeyStyle.Render("p") + HelpDescStyle.Render(" pods"),
			HelpKeyStyle.Render("s") + HelpDescStyle.Render(" services"),
			HelpKeyStyle.Render("esc") + HelpDescStyle.Render(" back"),
		}
	case "connecting":
		keys = []string{
			HelpKeyStyle.Render("esc") + HelpDescStyle.Render(" cancel"),
		}
	case "namespace", "pod", "service":
		keys = []string{
			HelpKeyStyle.Render("↑/↓") + HelpDescStyle.Render(" navigate"),
			HelpKeyStyle.Render("enter") + HelpDescStyle.Render(" select"),
			HelpKeyStyle.Render("esc") + HelpDescStyle.Render(" back"),
		}
	case "connections":
		keys = []string{
			HelpKeyStyle.Render("↑/↓") + HelpDescStyle.Render(" navigate"),
			HelpKeyStyle.Render("n") + HelpDescStyle.Render(" new"),
			HelpKeyStyle.Render("d") + HelpDescStyle.Render(" stop"),
			HelpKeyStyle.Render("r") + HelpDescStyle.Render(" reconnect"),
			HelpKeyStyle.Render("x") + HelpDescStyle.Render(" delete"),
			HelpKeyStyle.Render("l") + HelpDescStyle.Render(" logs"),
		}
	case "logs":
		keys = []string{
			HelpKeyStyle.Render("esc") + HelpDescStyle.Render(" back"),
		}
	case "help":
		keys = []string{
			HelpKeyStyle.Render("esc/?") + HelpDescStyle.Render(" close"),
		}
	case "port_input":
		keys = []string{
			HelpKeyStyle.Render("tab") + HelpDescStyle.Render(" next field"),
			HelpKeyStyle.Render("enter") + HelpDescStyle.Render(" confirm"),
			HelpKeyStyle.Render("esc") + HelpDescStyle.Render(" cancel"),
		}
	}

	keys = append(keys, commonKeys...)
	return HelpStyle.Render(strings.Join(keys, "  │  "))
}

// RenderHelpScreen renders the full help screen
func RenderHelpScreen(width, height int) string {
	var b strings.Builder

	title := TitleStyle.Render("⌨️  Keyboard Shortcuts")
	b.WriteString(title + "\n\n")

	sections := []struct {
		name string
		keys [][]string
	}{
		{
			name: "Global",
			keys: [][]string{
				{"q, Ctrl+C", "Quit application"},
				{"?", "Show/hide this help"},
				{"Esc", "Go back / Cancel"},
			},
		},
		{
			name: "Connections List",
			keys: [][]string{
				{"↑/↓, k/j", "Navigate"},
				{"n", "New port-forward"},
				{"d", "Stop selected connection"},
				{"r", "Reconnect stopped connection"},
				{"x, Delete", "Delete connection from list"},
				{"l", "View connection logs"},
			},
		},
		{
			name: "Selection Lists",
			keys: [][]string{
				{"↑/↓, k/j", "Navigate"},
				{"Enter", "Select item"},
				{"p", "Quick select Pods (resource type)"},
				{"s", "Quick select Services (resource type)"},
			},
		},
		{
			name: "Port Input",
			keys: [][]string{
				{"Tab", "Switch between local/remote port"},
				{"Enter", "Start port-forward"},
			},
		},
	}

	for _, section := range sections {
		b.WriteString(SubtitleStyle.Render(section.name) + "\n")
		for _, kv := range section.keys {
			key := HelpKeyStyle.Render(fmt.Sprintf("  %-14s", kv[0]))
			desc := HelpDescStyle.Render(kv[1])
			b.WriteString(key + " " + desc + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(HelpDescStyle.Render("Press ? or Esc to close"))

	return BoxStyle.Width(width).Render(b.String())
}

// RenderHeader renders the application header
func RenderHeader(context, namespace string, width int) string {
	left := CompactLogo()
	right := ""

	if context != "" {
		right += NamespaceStyle.Render("ctx: ") + ValueStyle.Render(context)
	}
	if namespace != "" {
		right += "  " + NamespaceStyle.Render("ns: ") + ValueStyle.Render(namespace)
	}

	// Calculate spacing
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	spacing := width - leftWidth - rightWidth - 4

	if spacing < 0 {
		spacing = 1
	}

	header := left + strings.Repeat(" ", spacing) + right

	return HeaderStyle.Width(width).Render(header)
}

// RenderTabs renders navigation tabs
func RenderTabs(tabs []string, activeTab int) string {
	var renderedTabs []string

	for i, tab := range tabs {
		if i == activeTab {
			renderedTabs = append(renderedTabs, ActiveTabStyle.Render(tab))
		} else {
			renderedTabs = append(renderedTabs, TabStyle.Render(tab))
		}
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, renderedTabs...)
}

// RenderConfirmDialog renders a confirmation dialog
func RenderConfirmDialog(title, message string, width int) string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render(title) + "\n\n")
	b.WriteString(ValueStyle.Render(message) + "\n\n")
	b.WriteString(HelpKeyStyle.Render("y") + HelpDescStyle.Render(" yes  "))
	b.WriteString(HelpKeyStyle.Render("n") + HelpDescStyle.Render(" no"))

	return DialogStyle.Width(width).Align(lipgloss.Center).Render(b.String())
}

// RenderError renders an error message
func RenderError(err string, width int) string {
	return ErrorBadgeStyle.Render("ERROR") + " " + StatusErrorStyle.Render(err)
}

// RenderSuccess renders a success message
func RenderSuccess(msg string, width int) string {
	return SuccessBadgeStyle.Render("OK") + " " + StatusActiveStyle.Render(msg)
}

// RenderLoading renders a loading indicator
func RenderLoading(msg string) string {
	spinner := SpinnerStyle.Render("⠋")
	return spinner + " " + ValueStyle.Render(msg)
}

// Helper functions

func getStatusStyle(status string) string {
	switch status {
	case "Running":
		return StatusActiveStyle.Render("●")
	case "Pending":
		return StatusStartingStyle.Render("◐")
	case "Failed", "Error":
		return StatusErrorStyle.Render("✗")
	default:
		return StatusStoppedStyle.Render("○")
	}
}

func formatPorts(ports []k8s.ContainerPort) string {
	if len(ports) == 0 {
		return PortStyle.Foreground(ColorMuted).Render("no ports")
	}

	var portStrs []string
	for _, p := range ports {
		portStrs = append(portStrs, fmt.Sprintf("%d/%s", p.ContainerPort, strings.ToLower(p.Protocol)))
	}
	return PortStyle.Render(strings.Join(portStrs, ", "))
}

func formatServicePorts(ports []k8s.ServicePort) string {
	if len(ports) == 0 {
		return PortStyle.Foreground(ColorMuted).Render("no ports")
	}

	var portStrs []string
	for _, p := range ports {
		portStrs = append(portStrs, fmt.Sprintf("%d→%s", p.Port, p.TargetPort))
	}
	return PortStyle.Render(strings.Join(portStrs, ", "))
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

// calculateOffset calculates the scroll offset to keep selected item visible
func calculateOffset(selected, total, visibleItems int) int {
	if total <= visibleItems {
		return 0
	}

	// Keep selected item roughly in the middle of visible area
	offset := selected - visibleItems/2
	if offset < 0 {
		offset = 0
	}

	// Don't scroll past the end
	maxOffset := total - visibleItems
	if offset > maxOffset {
		offset = maxOffset
	}

	return offset
}

// ScrollIndicatorStyle for scroll indicators
var ScrollIndicatorStyle = lipgloss.NewStyle().
	Foreground(ColorSecondary).
	Italic(true)
