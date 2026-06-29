package main

import (
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	root := "."
	clean := false

	nodeProjects := []string{}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if !isIgnoredFileDir(path) && strings.Contains(path, "package.json") {
			parent := filepath.Dir(path)
			nodeProjects = append(nodeProjects, parent)
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	if clean {
		for _, project := range nodeProjects {
			if err := deleteAllInPath(project + "/node_modules"); err != nil {
				log.Fatal("remove project error", err)
			}
		}
	}
}

func isIgnoredFileDir(path string) bool {
	switch {
	case strings.HasPrefix(path, "."):
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
