package help

import (
	"image/color"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

var (
	Charple     = lipgloss.Color("#6B50FF")
	Pony        = lipgloss.Color("#FF4FBF")
	Cheeky      = lipgloss.Color("#FF79D0")
	Charcoal    = lipgloss.Color("#3A3943")
	Squid       = lipgloss.Color("#858392")
	Smoke       = lipgloss.Color("#BFBCC8")
	Guac        = lipgloss.Color("#12C78F")
	Ash         = lipgloss.Color("#DFDBDD")
	Cherry      = lipgloss.Color("#FF388B")
	BrightGreen = lipgloss.Color("#A6E22E")
	DarkGreen   = lipgloss.Color("#5F8700")
	Cardinal    = lipgloss.Color("#D70000")
	Watermelon  = lipgloss.Color("#FF5F87")
	Basil       = lipgloss.Color("#0CB37F")
	Anchovy     = lipgloss.Color("#719AFC")
)

// ColorScheme defines colors for different help elements and reporting
type ColorScheme struct {
	Title       color.Color
	Command     color.Color
	Flag        color.Color
	Argument    color.Color
	Description color.Color
	Default     color.Color
	Section     color.Color
	Error       color.Color
	Success     color.Color
	Muted       color.Color
	Heading     color.Color
}

// Styles contains all the lipgloss styles for help output and reporting
type Styles struct {
	Title       lipgloss.Style
	Command     lipgloss.Style
	Flag        lipgloss.Style
	Argument    lipgloss.Style
	Description lipgloss.Style
	Default     lipgloss.Style
	Section     lipgloss.Style
	Error       lipgloss.Style
	Success     lipgloss.Style
	Muted       lipgloss.Style
	Heading     lipgloss.Style
}

// DefaultColorScheme returns a color scheme adapted from charm fang theme
func DefaultColorScheme(c lipgloss.LightDarkFunc) ColorScheme {
	return ColorScheme{
		Title:       c(Anchovy, Charple),
		Command:     c(Pony, Cheeky),
		Flag:        c(Basil, Guac),
		Argument:    c(Charcoal, Ash),
		Description: c(Charcoal, Ash),
		Default:     c(Smoke, Squid),
		Section:     c(DarkGreen, BrightGreen),
		Error:       c(Cardinal, Watermelon),
		Success:     c(Basil, Guac),
		Muted:       c(Smoke, Squid),
		Heading:     c(Anchovy, Charple),
	}
}

// ANSI256ColorScheme returns a color scheme using ANSI256 colors for better terminal compatibility
func ANSI256ColorScheme(c lipgloss.LightDarkFunc) ColorScheme {
	return ColorScheme{
		Title:       lipgloss.Color("99"),                            // purple
		Command:     c(lipgloss.Color("205"), lipgloss.Color("213")), // magenta/pink
		Flag:        c(lipgloss.Color("36"), lipgloss.Color("42")),   // cyan/green
		Argument:    c(lipgloss.Color("240"), lipgloss.Color("250")), // gray
		Description: c(lipgloss.Color("240"), lipgloss.Color("250")), // gray
		Default:     c(lipgloss.Color("244"), lipgloss.Color("246")), // gray
		Section:     c(lipgloss.Color("28"), lipgloss.Color("82")),   // green
		Error:       c(lipgloss.Color("160"), lipgloss.Color("204")), // red/pink
		Success:     c(lipgloss.Color("36"), lipgloss.Color("42")),   // cyan/green
		Muted:       c(lipgloss.Color("244"), lipgloss.Color("246")), // gray
		Heading:     lipgloss.Color("99"),                            // purple
	}
}

// NewStyles creates a new Styles instance from a color scheme
func NewStyles(scheme ColorScheme) Styles {
	return Styles{
		Title: lipgloss.NewStyle().
			Foreground(scheme.Title).
			Bold(true),
		Command: lipgloss.NewStyle().
			Foreground(scheme.Command).
			Bold(true),
		Flag: lipgloss.NewStyle().
			Foreground(scheme.Flag),
		Argument: lipgloss.NewStyle().
			Foreground(scheme.Argument),
		Description: lipgloss.NewStyle().
			Foreground(scheme.Description),
		Default: lipgloss.NewStyle().
			Foreground(scheme.Default).
			Faint(true),
		Section: lipgloss.NewStyle().
			Foreground(scheme.Section).
			Bold(true).
			Underline(true),
		Error: lipgloss.NewStyle().
			Foreground(scheme.Error).
			Bold(true),
		Success: lipgloss.NewStyle().
			Foreground(scheme.Success),
		Muted: lipgloss.NewStyle().
			Foreground(scheme.Muted),
		Heading: lipgloss.NewStyle().
			Foreground(scheme.Heading).
			Bold(true),
	}
}

// DefaultStyles returns the default styled theme, automatically detecting color support
func DefaultStyles() Styles {
	lightDark := lipgloss.LightDark(lipgloss.HasDarkBackground(os.Stdin, os.Stdout))

	// Detect terminal color support
	profile := colorprofile.Detect(os.Stdout, os.Environ())

	// Use ANSI256 colors for terminals with limited color support
	var scheme ColorScheme
	if profile < colorprofile.TrueColor {
		scheme = ANSI256ColorScheme(lightDark)
	} else {
		scheme = DefaultColorScheme(lightDark)
	}

	return NewStyles(scheme)
}

// FormatMCPStderr formats an MCP server stderr line with consistent styling
func (s Styles) FormatMCPStderr(line string) string {
	prefix := s.Muted.Render("[MCP] ")
	return prefix + s.Error.Render(line)
}
