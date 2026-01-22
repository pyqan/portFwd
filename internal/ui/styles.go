package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// Color palette - Cyberpunk/Terminal aesthetic
var (
	ColorPrimary    = lipgloss.Color("#00FF9F") // Neon green
	ColorSecondary  = lipgloss.Color("#00B8FF") // Cyan
	ColorAccent     = lipgloss.Color("#FF006E") // Magenta/Pink
	ColorWarning    = lipgloss.Color("#FFB800") // Orange/Yellow
	ColorError      = lipgloss.Color("#FF3366") // Red
	ColorSuccess    = lipgloss.Color("#00FF9F") // Green
	ColorMuted      = lipgloss.Color("#6B7280") // Gray
	ColorBackground = lipgloss.Color("#0D1117") // Dark background
	ColorSurface    = lipgloss.Color("#161B22") // Surface
	ColorBorder     = lipgloss.Color("#30363D") // Border
	ColorText       = lipgloss.Color("#E6EDF3") // Light text
	ColorTextDim    = lipgloss.Color("#8B949E") // Dim text
)

// Styles
var (
	// Base styles
	BaseStyle = lipgloss.NewStyle().
			Background(ColorBackground)

	// Title styles
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			Background(ColorSurface).
			Padding(0, 2).
			MarginBottom(1)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			MarginBottom(1)

	// Container styles
	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(1, 2)

	ActiveBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(1, 2)

	// List styles
	ListItemStyle = lipgloss.NewStyle().
			Foreground(ColorText).
			PaddingLeft(2)

	SelectedItemStyle = lipgloss.NewStyle().
				Foreground(ColorBackground).
				Background(ColorPrimary).
				Bold(true).
				PaddingLeft(2).
				PaddingRight(2)

	// Status styles
	StatusActiveStyle = lipgloss.NewStyle().
				Foreground(ColorSuccess).
				Bold(true)

	StatusStoppedStyle = lipgloss.NewStyle().
				Foreground(ColorMuted)

	StatusErrorStyle = lipgloss.NewStyle().
				Foreground(ColorError).
				Bold(true)

	StatusStartingStyle = lipgloss.NewStyle().
				Foreground(ColorWarning)

	// Info styles
	LabelStyle = lipgloss.NewStyle().
			Foreground(ColorTextDim).
			Width(12)

	ValueStyle = lipgloss.NewStyle().
			Foreground(ColorText)

	HighlightStyle = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true)

	// Input styles
	InputStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(ColorBorder).
			Padding(0, 1)

	FocusedInputStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(ColorPrimary).
				Padding(0, 1)

	// Help styles
	HelpStyle = lipgloss.NewStyle().
			Foreground(ColorTextDim).
			MarginTop(1)

	HelpKeyStyle = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true)

	HelpDescStyle = lipgloss.NewStyle().
			Foreground(ColorTextDim)

	// Tab styles
	TabStyle = lipgloss.NewStyle().
			Foreground(ColorTextDim).
			Padding(0, 2)

	ActiveTabStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true).
			Padding(0, 2).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(ColorPrimary)

	// Badge styles
	BadgeStyle = lipgloss.NewStyle().
			Foreground(ColorBackground).
			Background(ColorSecondary).
			Padding(0, 1).
			Bold(true)

	ErrorBadgeStyle = lipgloss.NewStyle().
			Foreground(ColorBackground).
			Background(ColorError).
			Padding(0, 1).
			Bold(true)

	SuccessBadgeStyle = lipgloss.NewStyle().
				Foreground(ColorBackground).
				Background(ColorSuccess).
				Padding(0, 1).
				Bold(true)

	// Namespace/Pod styles
	NamespaceStyle = lipgloss.NewStyle().
			Foreground(ColorSecondary)

	PodStyle = lipgloss.NewStyle().
			Foreground(ColorText)

	PortStyle = lipgloss.NewStyle().
			Foreground(ColorWarning)

	// Header
	HeaderStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true).
			Width(80).
			Align(lipgloss.Center).
			Border(lipgloss.DoubleBorder(), false, false, true, false).
			BorderForeground(ColorBorder).
			MarginBottom(1)

	// Footer
	FooterStyle = lipgloss.NewStyle().
			Foreground(ColorTextDim).
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(ColorBorder).
			MarginTop(1).
			Padding(0, 1)

	// Spinner
	SpinnerStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary)

	// Dialog
	DialogStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(ColorAccent).
			Padding(1, 2).
			Background(ColorSurface)

	// Progress
	ProgressStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary)

	ProgressEmptyStyle = lipgloss.NewStyle().
				Foreground(ColorBorder)
)

// StatusIcon returns an icon for the given status
func StatusIcon(status string) string {
	switch status {
	case "active":
		return StatusActiveStyle.Render("●")
	case "stopped":
		return StatusStoppedStyle.Render("○")
	case "error":
		return StatusErrorStyle.Render("✗")
	case "starting":
		return StatusStartingStyle.Render("◐")
	case "reconnecting":
		return StatusWarningStyle.Render("⟳")
	default:
		return StatusStoppedStyle.Render("?")
	}
}

// StatusWarningStyle for reconnecting status
var StatusWarningStyle = lipgloss.NewStyle().
	Foreground(ColorWarning).
	Bold(true)

// DimStyle for subtle text (debug info, paths, etc)
var DimStyle = lipgloss.NewStyle().
	Foreground(ColorMuted).
	Italic(true)

// Logo returns the application logo
func Logo() string {
	logo := `
╔═══════════════════════════════════════════╗
║  ██████╗  ██████╗ ██████╗ ████████╗       ║
║  ██╔══██╗██╔═══██╗██╔══██╗╚══██╔══╝       ║
║  ██████╔╝██║   ██║██████╔╝   ██║          ║
║  ██╔═══╝ ██║   ██║██╔══██╗   ██║          ║
║  ██║     ╚██████╔╝██║  ██║   ██║          ║
║  ╚═╝      ╚═════╝ ╚═╝  ╚═╝   ╚═╝          ║
║  ███████╗██╗    ██╗██████╗                ║
║  ██╔════╝██║    ██║██╔══██╗               ║
║  █████╗  ██║ █╗ ██║██║  ██║               ║
║  ██╔══╝  ██║███╗██║██║  ██║               ║
║  ██║     ╚███╔███╔╝██████╔╝               ║
║  ╚═╝      ╚══╝╚══╝ ╚═════╝                ║
╚═══════════════════════════════════════════╝`
	return lipgloss.NewStyle().Foreground(ColorPrimary).Render(logo)
}

// CompactLogo returns a compact version of the logo
func CompactLogo() string {
	return lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true).
		Render("⚡ PortFwd")
}
