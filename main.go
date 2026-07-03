package main

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
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

type Model struct {
	discovering bool
	projects    []NodeProject

	cursor   int
	selected map[int]struct{}

	errMsg string
}

type NodeProject struct {
	ProjectPath     string
	HasNodeModules  bool
	NodeModulesSize float64
	ErrorMessage    string
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case ProjectDiscoveredMsg:
		m.projects = append(m.projects, msg.project)

	case DicoveryDoneMsg:
		m.discovering = false

	case DiscoveryErrorMsg:
		m.errMsg = msg.errMsg

	case ProjectNodeModuleSize:
		for i := range m.projects {
			if m.projects[i].ProjectPath == msg.ProjectPath {
				m.projects[i].NodeModulesSize = msg.Size
			}
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
			if m.cursor < len(m.projects) {
				m.cursor++
			}

		case "space":
			_, selected := m.selected[m.cursor]
			if selected {
				delete(m.selected, m.cursor)
			} else {
				m.selected[m.cursor] = struct{}{}
			}
		}
	}

	return m, nil
}

func (m Model) View() tea.View {
	var s strings.Builder

	discovering := "discovering"
	if !m.discovering {
		discovering = "finished"
	}

	fmt.Fprintf(&s, "Select projects to delete node modules [%s]\n\n", discovering)

	if m.errMsg != "" {
		fmt.Fprintf(&s, "Encountered an error: %s\n\n", m.errMsg)
	}

	for i, project := range m.projects {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}

		checked := " "
		if _, selected := m.selected[i]; selected {
			checked = "x"
		}

		fmt.Fprintf(&s, "%s [%s] %s (%.1fMB)\n", cursor, checked, project.ProjectPath, project.NodeModulesSize)
	}

	v := tea.NewView(s.String())
	v.AltScreen = true
	return v
}

func main() {
	m := Model{
		discovering: true,
		projects:    make([]NodeProject, 0),
		selected:    make(map[int]struct{}),
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

			if _, err = os.Stat(projectPath + "/node_modules"); err != nil {
				hasNodeModules = false
			}

			project := NodeProject{
				ProjectPath:    projectPath,
				HasNodeModules: hasNodeModules,
			}

			notify(ProjectDiscoveredMsg{project})

			go func(p *NodeProject, notify func(tea.Msg)) {
				size, err := dirSizeMB(p.ProjectPath + "/node_modules")
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
