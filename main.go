package main

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type NodeProject struct {
	Name            string
	RelativePath    string
	HasNodeModules  bool
	NodeModulesSize int64
	ErrorMessage    string
}

func main() {
	root := "."
	clean := false

	nodeProjects := []NodeProject{}

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

			nodeProjects = append(nodeProjects, project)
			fmt.Printf("project: %+v\n", project)
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	if clean {
		for _, project := range nodeProjects {
			if err := deleteAllInPath(project.RelativePath + "/node_modules"); err != nil {
				log.Fatal("remove project error", err)
			}
		}
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
