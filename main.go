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

type Model struct {
	discovering bool
	projects    []NodeProject

	cursor   int
	selected map[int]struct{}
}

type NodeProject struct {
	Name            string
	RelativePath    string
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

	for i, project := range m.projects {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}

		checked := " "
		if _, selected := m.selected[i]; selected {
			checked = "x"
		}

		fmt.Fprintf(&s, "%s [%s] %s (%.1fMB)\n", cursor, checked, project.Name, project.NodeModulesSize)
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

	go func() {
		_ = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
			if !isIgnoredFileDir(path) && strings.Contains(path, "package.json") {
				parentPath := filepath.Dir(path)
				projectName := filepath.Base(parentPath)
				hasNodeModules := true
				var nodeModulesSize float64

				_, err := os.Stat(parentPath + "/node_modules")
				if err != nil {
					hasNodeModules = false
				} else {
					nodeModulesSize = dirSizeMB(parentPath + "/node_modules")
				}

				project := NodeProject{
					Name:            projectName,
					RelativePath:    parentPath,
					HasNodeModules:  hasNodeModules,
					NodeModulesSize: nodeModulesSize,
				}

				p.Send(ProjectDiscoveredMsg{project})

				return filepath.SkipDir
			}
			return nil
		})

		p.Send(DicoveryDoneMsg{})
	}()

	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

func isIgnoredFileDir(path string) bool {
	switch {
	case strings.Contains(path, ".next"):
		fallthrough
	case strings.Contains(path, "node_modules"):
		return true
	}
	return false
}

func deleteAllInPath(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	return os.RemoveAll(path)
}

func dirSizeMB(path string) float64 {
	var dirSizeBytes int64 = 0

	_ = filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			dirSizeBytes += info.Size()
		}

		return nil
	})

	sizeMB := float64(dirSizeBytes) / 1024.0 / 1024.0

	return sizeMB
}
