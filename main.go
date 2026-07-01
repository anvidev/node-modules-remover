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

type Model struct {
	projects []NodeProject

	cursor   int
	selected map[int]struct{}
}

type NodeProject struct {
	Name            string
	RelativePath    string
	HasNodeModules  bool
	NodeModulesSize int64
	ErrorMessage    string
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

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

	s.WriteString("Select projects to delete node modules\n\n")

	for i, project := range m.projects {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}

		checked := " "
		if _, selected := m.selected[i]; selected {
			checked = "x"
		}

		fmt.Fprintf(&s, "%s [%s] %s\n", cursor, checked, project.Name)
	}

	v := tea.NewView(s.String())
	v.AltScreen = true
	return v
}

func main() {
	m := Model{
		projects: make([]NodeProject, 0),
		selected: make(map[int]struct{}),
	}

	root := "."

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if !isIgnoredFileDir(path) && strings.Contains(path, "package.json") {
			parentPath := filepath.Dir(path)
			projectName := filepath.Base(parentPath)
			hasNodeModules := true
			var nodeModulesSize int64

			nodeModulesInfo, err := os.Stat(parentPath + "/node_modules")
			if err != nil {
				hasNodeModules = false
			} else {
				nodeModulesSize = nodeModulesInfo.Size()
			}

			project := NodeProject{
				Name:            projectName,
				RelativePath:    parentPath,
				HasNodeModules:  hasNodeModules,
				NodeModulesSize: nodeModulesSize,
			}

			m.projects = append(m.projects, project)
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	_, err = tea.NewProgram(m).Run()
	if err != nil {
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
