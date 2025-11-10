package main

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: gitar <repository-path>")
		fmt.Println("Archives a Git repository according to .gitignore rules")
		os.Exit(1)
	}

	repoPath := os.Args[1]

	// Convert to absolute path
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		fmt.Printf("Error: Could not resolve path '%s': %v\n", repoPath, err)
		os.Exit(1)
	}
	repoPath = absPath

	// Check if the path exists
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		fmt.Printf("Error: Repository path '%s' does not exist\n", repoPath)
		os.Exit(1)
	}

	// Check if it's a Git repository
	gitPath := filepath.Join(repoPath, ".git")
	if _, err := os.Stat(gitPath); os.IsNotExist(err) {
		fmt.Printf("Error: '%s' is not a Git repository (no .git directory found)\n", repoPath)
		os.Exit(1)
	}

	// Parse .gitignore
	ignorePatterns, err := parseGitignore(repoPath)
	if err != nil {
		fmt.Printf("Warning: Could not parse .gitignore: %v\n", err)
		ignorePatterns = []string{}
	}

	// Create archive with proper name
	baseName := filepath.Base(repoPath)
	if baseName == "." || baseName == ".." {
		// Get the actual directory name
		baseName = filepath.Base(filepath.Clean(repoPath))
	}
	archiveName := baseName + ".zip"

	if err := createArchive(repoPath, archiveName, ignorePatterns); err != nil {
		fmt.Printf("Error creating archive: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully created archive: %s\n", archiveName)
}

// parseGitignore reads and parses the .gitignore file
func parseGitignore(repoPath string) ([]string, error) {
	gitignorePath := filepath.Join(repoPath, ".gitignore")
	file, err := os.Open(gitignorePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var patterns []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return patterns, nil
}

// shouldIgnore checks if a file path should be ignored based on .gitignore patterns
func shouldIgnore(path string, repoPath string, patterns []string) bool {
	relPath, err := filepath.Rel(repoPath, path)
	if err != nil {
		return false
	}

	// Normalize path separators
	relPath = filepath.ToSlash(relPath)

	// Always ignore .git directory
	if strings.HasPrefix(relPath, ".git/") || relPath == ".git" {
		return true
	}

	// Check against each pattern
	for _, pattern := range patterns {
		// Normalize pattern separators
		pattern = filepath.ToSlash(pattern)

		// Handle directory patterns (ending with /)
		if strings.HasSuffix(pattern, "/") {
			dirPattern := strings.TrimSuffix(pattern, "/")

			// Check if the path starts with this directory
			if relPath == dirPattern || strings.HasPrefix(relPath, dirPattern+"/") {
				return true
			}

			// Check if any directory component matches
			parts := strings.Split(relPath, "/")
			for _, part := range parts {
				if matched, _ := filepath.Match(dirPattern, part); matched {
					return true
				}
			}
		} else {
			// File pattern matching
			// Match against the basename
			if matched, _ := filepath.Match(pattern, filepath.Base(relPath)); matched {
				return true
			}

			// Match against the full path if pattern contains /
			if strings.Contains(pattern, "/") {
				if matched, _ := filepath.Match(pattern, relPath); matched {
					return true
				}
			}

			// For patterns like "node_modules", also check if it's a directory name in path
			parts := strings.Split(relPath, "/")
			for _, part := range parts {
				if matched, _ := filepath.Match(pattern, part); matched {
					return true
				}
			}
		}
	}

	return false
}

// createArchive creates a zip archive of the repository
func createArchive(repoPath, archiveName string, patterns []string) error {
	// Get absolute path to the archive file
	absArchivePath, err := filepath.Abs(archiveName)
	if err != nil {
		return err
	}

	// Create zip file
	zipFile, err := os.Create(archiveName)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Walk through the repository
	return filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the archive file itself
		absPath, _ := filepath.Abs(path)
		if absPath == absArchivePath {
			return nil
		}

		// Check if file should be ignored
		if shouldIgnore(path, repoPath, patterns) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Get relative path for archive
		relPath, err := filepath.Rel(repoPath, path)
		if err != nil {
			return err
		}

		// Skip the root directory itself
		if relPath == "." {
			return nil
		}

		// Create directory entry or file entry
		if info.IsDir() {
			// Add directory to archive with trailing slash
			_, err := zipWriter.Create(relPath + "/")
			return err
		}

		// Create file entry
		writer, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}

		// Copy file content
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(writer, file)
		return err
	})
}
