package main

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var (
	styleContainer = lipgloss.NewStyle().Padding(0, 1)
	stylePurple    = lipgloss.NewStyle().Foreground(lipgloss.Color("#7F77DD"))
	styleDimmed    = lipgloss.NewStyle().Foreground(lipgloss.Color("#5f5e5a"))
	styleTotal     = lipgloss.NewStyle().Foreground(lipgloss.Color("#1d9e75"))
	styleCursor    = lipgloss.NewStyle().Foreground(lipgloss.Color("#378add"))
	styleHigh      = lipgloss.NewStyle().Foreground(lipgloss.Color("#f55d5d"))
	styleMid       = lipgloss.NewStyle().Foreground(lipgloss.Color("#f0997b"))
	styleLow       = lipgloss.NewStyle().Foreground(lipgloss.Color("#ef9f27"))
)

type mode int

const (
	modeSearching mode = iota + 1
	modeBrowse
	modeDeleting
	modeSelecting
)

type ProjectDiscoveredMsg struct {
	project NodeProject
}
type DicoveryDoneMsg struct{}
type DiscoveryErrorMsg struct {
	errMsg string
}
type ProjectNodeModuleSize struct {
	ProjectPath string
	Size        float64
}
type ProjectNodeModuleSizeErrMsg struct {
	ProjectPath string
	ErrMsg      string
}
type deleteFinishedMsg struct {
	Index int
}

type Model struct {
	discovering bool
	projects    []NodeProject

	cursor   int
	selected map[int]struct{}

	errMsg   string
	flashMsg string

	mode    mode
	msg     string
	spinner spinner.Model
}

type NodeProject struct {
	ProjectPath     string
	HasNodeModules  bool
	NodeModulesSize float64
	ErrorMessage    string
	SizeKnown       bool
	Deleting        bool
}

func (m Model) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case ProjectDiscoveredMsg:
		m.projects = append(m.projects, msg.project)

	case DicoveryDoneMsg:
		m.discovering = false
		m.mode = modeBrowse

	case DiscoveryErrorMsg:
		m.errMsg = msg.errMsg

	case ProjectNodeModuleSize:
		for i := range m.projects {
			if m.projects[i].ProjectPath == msg.ProjectPath {
				m.projects[i].NodeModulesSize = msg.Size
				m.projects[i].SizeKnown = true
			}
		}

	case deleteFinishedMsg:
		delete(m.selected, msg.Index)
		m.projects[msg.Index].HasNodeModules = false
		m.projects[msg.Index].NodeModulesSize = 0
		if len(m.selected) == 0 {
			m.mode = modeBrowse
		}

	case ProjectNodeModuleSizeErrMsg:
		for i := range m.projects {
			if m.projects[i].ProjectPath == msg.ProjectPath {
				m.projects[i].ErrorMessage = msg.ErrMsg
			}
		}

	case tea.KeyPressMsg:

		switch msg.String() {

		case "ctrl+c", "q":
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.projects)-1 {
				m.cursor++
			}

		case "space":
			_, selected := m.selected[m.cursor]
			if selected {
				delete(m.selected, m.cursor)
				if len(m.selected) == 0 {
					m.mode = modeBrowse
				}
			} else {
				m.mode = modeSelecting
				m.selected[m.cursor] = struct{}{}
			}

		case "enter":
			m.mode = modeDeleting
			for i := range m.selected {
				if m.projects[i].HasNodeModules {
					m.projects[i].Deleting = true
					cmds = append(cmds, m.deleteCmd(i))
				}
			}
			m.errMsg = ""
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() tea.View {
	var s strings.Builder

	s.WriteString(stylePurple.Render("node-modules-remover"))
	s.WriteString("\n\n")

	switch m.mode {
	case modeSearching:
		s.WriteString(styleDimmed.Render("Searching for projects... "))
		s.WriteString(m.spinner.View())
	case modeSelecting:
		totalSize := 0.0
		for i := range m.selected {
			totalSize += m.projects[i].NodeModulesSize
		}
		s.WriteString(styleDimmed.Render("Total selected: "))
		s.WriteString(styleTotal.Render(fmt.Sprintf("%.f MB", totalSize)))
	case modeDeleting:
		s.WriteString(styleDimmed.Render("Cleaning up projects... "))
		s.WriteString(m.spinner.View())
	case modeBrowse:
		s.WriteString(styleDimmed.Render("Pick projects to clean up"))
	}

	s.WriteString("\n\n")

	for i, project := range m.projects {
		_, selected := m.selected[i]
		s.WriteString(renderRow(project, m.cursor == i, selected, m.spinner))
		s.WriteString("\n")
	}

	v := tea.NewView(styleContainer.Render(s.String()))
	v.AltScreen = true
	return v
}

func main() {
	m := Model{
		mode:        modeSearching,
		discovering: true,
		projects:    make([]NodeProject, 0),
		selected:    make(map[int]struct{}),
		spinner:     spinner.New(spinner.WithSpinner(spinner.Dot)),
	}

	p := tea.NewProgram(m)

	go findAndNotifyNodeProjects(".", p.Send)

	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

func findAndNotifyNodeProjects(root string, notify func(tea.Msg)) {
	walkFunc := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() && isIgnoreDir(d) {
			return filepath.SkipDir
		}

		if !d.IsDir() && d.Name() == "package.json" {
			projectPath := filepath.Dir(path)
			hasNodeModules := true

			if _, err = os.Stat(filepath.Join(projectPath, "node_modules")); err != nil {
				hasNodeModules = false
			}

			project := NodeProject{
				ProjectPath:    projectPath,
				HasNodeModules: hasNodeModules,
			}

			notify(ProjectDiscoveredMsg{project})

			if project.HasNodeModules {
				go func(p *NodeProject, notify func(tea.Msg)) {
					size, err := dirSizeMB(filepath.Join(p.ProjectPath, "node_modules"))
					if err != nil {
						notify(ProjectNodeModuleSizeErrMsg{
							ProjectPath: p.ProjectPath,
							ErrMsg:      err.Error(),
						})
						return
					}

					notify(ProjectNodeModuleSize{
						ProjectPath: p.ProjectPath,
						Size:        size,
					})
				}(&project, notify)
			}

			return filepath.SkipDir
		}

		return nil
	}

	if err := filepath.WalkDir(root, walkFunc); err != nil {
		notify(DiscoveryErrorMsg{errMsg: err.Error()})
	}

	notify(DicoveryDoneMsg{})
}

func isIgnoreDir(d fs.DirEntry) bool {
	var ignoredDirNames = map[string]bool{
		".next":        true,
		"node_modules": true,
		"dist":         true,
		"build":        true,
	}
	return ignoredDirNames[d.Name()]
}

func deleteAllInPath(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	return os.RemoveAll(path)
}

func dirSizeMB(path string) (float64, error) {
	var dirSizeBytes int64 = 0

	walkFunc := func(path string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			dirSizeBytes += info.Size()
		}

		return nil
	}

	if err := filepath.WalkDir(path, walkFunc); err != nil {
		return 0, err
	}

	sizeMB := float64(dirSizeBytes) / 1024.0 / 1024.0

	return sizeMB, nil
}

func renderRow(p NodeProject, isCursor, isSelected bool, sp spinner.Model) string {
	cursor := " "
	if isCursor {
		cursor = styleCursor.Render(">")
	}
	checked := "[ ]"
	if isSelected {
		checked = "[x]"
	}

	var size string
	switch {
	case !p.HasNodeModules:
		size = styleDimmed.Render("—")
	case !p.SizeKnown:
		size = sp.View()
	case p.Deleting:
		size = sp.View()
	default:
		size = sizeStyle(p.NodeModulesSize, p.HasNodeModules, p.SizeKnown).Render(fmt.Sprintf("%.1f MB", p.NodeModulesSize))
	}

	line := fmt.Sprintf("%s %s %-30s %s", cursor, checked, p.ProjectPath, size)
	if !p.HasNodeModules {
		return styleDimmed.Render(line)
	}
	return line
}

func sizeStyle(mb float64, hasNodeModules bool, sizeKnown bool) lipgloss.Style {
	switch {
	case !hasNodeModules:
		return styleDimmed
	case !sizeKnown:
		return styleDimmed
	case mb >= 500:
		return styleMid
	case mb >= 1000:
		return styleHigh
	default:
		return styleLow
	}
}

func (m *Model) deleteCmd(i int) tea.Cmd {
	m.projects[i].Deleting = true
	return func() tea.Msg {
		if err := deleteAllInPath(filepath.Join(m.projects[i].ProjectPath, "node_modules")); err == nil {
			return deleteFinishedMsg{Index: i}
		}
		return nil
	}
}
