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
	styleHeader    = lipgloss.NewStyle().Foreground(lipgloss.Color("#7F77DD"))
	styleError     = lipgloss.NewStyle().Foreground(lipgloss.Color("#f70000"))
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
	modeError
)

type projectFoundMsg struct {
	project nodeProject
}
type searchingDoneMsg struct{}

type nodeModulesSizeMsg struct {
	path string
	size float64
}

type deleteFinishedMsg struct {
	index int
}

type errMsg struct {
	err  error
	path *string
}

type model struct {
	projects []nodeProject

	cursor   int
	selected map[int]struct{}

	mode    mode
	msg     string
	spinner spinner.Model
}

type nodeProject struct {
	path            string
	nodeModulesSize float64
	hasNodeModules  bool
	sizeKnown       bool
	deleting        bool
}

func (m model) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {

	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case projectFoundMsg:
		m.projects = append(m.projects, msg.project)

	case searchingDoneMsg:
		m.mode = modeBrowse

	case nodeModulesSizeMsg:
		for i := range m.projects {
			if m.projects[i].path == msg.path {
				m.projects[i].nodeModulesSize = msg.size
				m.projects[i].sizeKnown = true
			}
		}

	case deleteFinishedMsg:
		delete(m.selected, msg.index)
		m.projects[msg.index].hasNodeModules = false
		m.projects[msg.index].nodeModulesSize = 0
		if len(m.selected) == 0 {
			m.mode = modeBrowse
		}

	case errMsg:
		m.mode = modeError
		m.msg = msg.err.Error()

	case tea.KeyPressMsg:

		switch msg.String() {

		case "ctrl+c", "q":
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 && m.mode == modeBrowse {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.projects)-1 && m.mode == modeBrowse {
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
				if m.projects[i].hasNodeModules {
					m.projects[i].deleting = true
					cmds = append(cmds, m.deleteCmd(i))
				}
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() tea.View {
	var s strings.Builder

	s.WriteString(styleHeader.Render("node-modules-remover"))
	s.WriteString("\n\n")

	switch m.mode {
	case modeSearching:
		s.WriteString(styleDimmed.Render("Searching for projects... "))
		s.WriteString(m.spinner.View())
	case modeSelecting:
		totalSize := 0.0
		for i := range m.selected {
			totalSize += m.projects[i].nodeModulesSize
		}
		s.WriteString(styleDimmed.Render("Total selected: "))
		s.WriteString(styleTotal.Render(fmt.Sprintf("%.f MB", totalSize)))
	case modeDeleting:
		s.WriteString(styleDimmed.Render("Cleaning up projects... "))
		s.WriteString(m.spinner.View())
	case modeBrowse:
		s.WriteString(styleDimmed.Render("Pick projects to clean up"))
	case modeError:
		s.WriteString(styleError.Render(m.msg))
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
	m := model{
		mode:     modeSearching,
		projects: make([]nodeProject, 0),
		selected: make(map[int]struct{}),
		spinner:  spinner.New(spinner.WithSpinner(spinner.Dot)),
	}

	p := tea.NewProgram(m)

	go searchAndNotifyNodeProjects(".", p.Send)

	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

func searchAndNotifyNodeProjects(root string, notify func(tea.Msg)) {
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

			project := nodeProject{
				path:           projectPath,
				hasNodeModules: hasNodeModules,
			}

			notify(projectFoundMsg{project})

			if project.hasNodeModules {
				go func(p *nodeProject, notify func(tea.Msg)) {
					size, err := dirSizeMB(filepath.Join(p.path, "node_modules"))
					if err != nil {
						notify(errMsg{
							path: &p.path,
							err:  err,
						})
						return
					}

					notify(nodeModulesSizeMsg{
						path: p.path,
						size: size,
					})
				}(&project, notify)
			}

			return filepath.SkipDir
		}

		return nil
	}

	if err := filepath.WalkDir(root, walkFunc); err != nil {
		notify(errMsg{err: err})
	}

	notify(searchingDoneMsg{})
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

func dirSizeMB(path string) (float64, error) {
	var dirSizeBytes int64 = 0

	walkFunc := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

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

func renderRow(p nodeProject, isCursor, isSelected bool, sp spinner.Model) string {
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
	case !p.hasNodeModules:
		size = styleDimmed.Render("—")
	case !p.sizeKnown:
		size = sp.View()
	case p.deleting:
		size = sp.View()
	default:
		size = sizeStyle(p.nodeModulesSize, p.hasNodeModules, p.sizeKnown).Render(fmt.Sprintf("%.1f MB", p.nodeModulesSize))
	}

	line := fmt.Sprintf("%s %s %-30s %s", cursor, checked, p.path, size)
	if !p.hasNodeModules {
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
	case mb >= 1000:
		return styleHigh
	case mb >= 500:
		return styleMid
	default:
		return styleLow
	}
}

func (m model) deleteCmd(i int) tea.Cmd {
	m.projects[i].deleting = true
	return func() tea.Msg {
		if err := os.RemoveAll(filepath.Join(m.projects[i].path, "node_modules")); err == nil {
			return deleteFinishedMsg{index: i}
		}
		return nil
	}
}
