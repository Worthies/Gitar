package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dsnet/compress/bzip2"
	"github.com/ulikunitz/xz"
)

type CompressionType int

const (
	Gzip CompressionType = iota
	Bzip2
	Xz
	None
)

type Config struct {
	OutputFile     string
	Compression    CompressionType
	Base64Encode   bool
	RepositoryPath string
	ShowHelp       bool
	StartRef       string
	EndRef         string
}

func main() {
	config := parseFlags()

	if config.ShowHelp {
		printHelp()
		return
	}

	if config.RepositoryPath == "" {
		fmt.Fprintf(os.Stderr, "Error: repository path is required\n")
		printUsage()
		os.Exit(1)
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(config.RepositoryPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Could not resolve path '%s': %v\n", config.RepositoryPath, err)
		os.Exit(1)
	}
	config.RepositoryPath = absPath

	// Check if the path exists
	if _, err := os.Stat(config.RepositoryPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: Repository path '%s' does not exist\n", config.RepositoryPath)
		os.Exit(1)
	}

	// Check if it's a Git repository
	gitPath := filepath.Join(config.RepositoryPath, ".git")
	if _, err := os.Stat(gitPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: '%s' is not a Git repository (no .git directory found)\n", config.RepositoryPath)
		os.Exit(1)
	}

	// Parse .gitignore
	ignorePatterns, err := parseGitignore(config.RepositoryPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not parse .gitignore: %v\n", err)
		ignorePatterns = []string{}
	}

	// Get list of files to archive
	var filesToArchive []string
	if config.StartRef != "" {
		// Archive only changed files
		filesToArchive, err = getChangedFiles(config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting changed files: %v\n", err)
			os.Exit(1)
		}
	}

	// Create archive
	if err := createArchive(config, ignorePatterns, filesToArchive); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating archive: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() Config {
	var config Config

	flag.StringVar(&config.OutputFile, "o", "", "Output archive to file instead of stdout")
	flag.StringVar(&config.OutputFile, "output", "", "Output archive to file instead of stdout")

	flag.StringVar(&config.StartRef, "start-ref", "", "Start Git reference (commit, branch, tag) to compare changes from")
	flag.StringVar(&config.EndRef, "end-ref", "", "End Git reference (commit, branch, tag) to compare changes to (defaults to HEAD)")

	var gzipFlag = flag.Bool("z", false, "Use gzip compression (default)")
	var gzipLongFlag = flag.Bool("gzip", false, "Use gzip compression (default)")
	var bzip2Flag = flag.Bool("j", false, "Use bzip2 compression")
	var bzip2LongFlag = flag.Bool("bzip2", false, "Use bzip2 compression")
	var xzFlag = flag.Bool("J", false, "Use xz compression")
	var xzLongFlag = flag.Bool("xz", false, "Use xz compression")
	var noCompressionFlag = flag.Bool("no-compression", false, "Create uncompressed tar archive")

	flag.BoolVar(&config.Base64Encode, "b", false, "Encode output in base64")
	flag.BoolVar(&config.Base64Encode, "base64", false, "Encode output in base64")

	flag.BoolVar(&config.ShowHelp, "h", false, "Show help message")
	flag.BoolVar(&config.ShowHelp, "help", false, "Show help message")

	flag.Parse()

	// Get repository path from remaining arguments
	if len(flag.Args()) > 0 {
		config.RepositoryPath = flag.Args()[0]
	}

	// Set default end ref to HEAD if start ref is provided but end ref is not
	if config.StartRef != "" && config.EndRef == "" {
		config.EndRef = "HEAD"
	}

	// Determine compression type
	config.Compression = Gzip // default
	if *bzip2Flag || *bzip2LongFlag {
		config.Compression = Bzip2
	} else if *xzFlag || *xzLongFlag {
		config.Compression = Xz
	} else if *noCompressionFlag {
		config.Compression = None
	} else if *gzipFlag || *gzipLongFlag {
		config.Compression = Gzip
	}

	return config
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: gitar [options] <repository-path>\n")
}

func printHelp() {
	fmt.Println("Gitar - Archive Git repositories respecting .gitignore rules")
	fmt.Println()
	printUsage()
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -o, --output <file>     Output archive to file instead of stdout")
	fmt.Println("  --start-ref <ref>       Start Git reference to compare changes from")
	fmt.Println("  --end-ref <ref>         End Git reference to compare changes to (defaults to HEAD)")
	fmt.Println("  -z, --gzip              Use gzip compression (default)")
	fmt.Println("  -j, --bzip2             Use bzip2 compression")
	fmt.Println("  -J, --xz                Use xz compression")
	fmt.Println("  --no-compression        Create uncompressed tar archive")
	fmt.Println("  -b, --base64            Encode output in base64")
	fmt.Println("  -h, --help              Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  gitar .                                    # Archive all files in current directory")
	fmt.Println("  gitar -o archive.tar.gz /path/to/repo      # Archive to file")
	fmt.Println("  gitar -b .                                 # Archive with base64 encoding")
	fmt.Println("  gitar --start-ref main .                   # Archive only files changed since main branch")
	fmt.Println("  gitar --start-ref v1.0 --end-ref v2.0 .   # Archive files changed between v1.0 and v2.0")
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

// getChangedFiles returns a list of files that have changed between the specified Git references
func getChangedFiles(config Config) ([]string, error) {
	var cmd *exec.Cmd

	if config.EndRef != "" {
		// Compare between two refs
		cmd = exec.Command("git", "diff", "--name-only", config.StartRef, config.EndRef)
	} else {
		// Compare from start ref to working directory
		cmd = exec.Command("git", "diff", "--name-only", config.StartRef)
	}

	cmd.Dir = config.RepositoryPath
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get changed files: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var files []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			// Convert to absolute path
			absPath := filepath.Join(config.RepositoryPath, line)
			files = append(files, absPath)
		}
	}

	return files, nil
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

// createArchive creates a tar archive of the repository
func createArchive(config Config, patterns []string, filesToArchive []string) error {
	var output io.Writer
	var file *os.File
	var err error

	// Determine output destination
	if config.OutputFile != "" {
		file, err = os.Create(config.OutputFile)
		if err != nil {
			return err
		}
		defer file.Close()
		output = file
	} else {
		output = os.Stdout
	}

	// Setup base64 encoding if requested
	if config.Base64Encode {
		encoder := base64.NewEncoder(base64.StdEncoding, output)
		defer encoder.Close()
		output = encoder
	}

	// Setup compression
	var compressedWriter io.WriteCloser
	switch config.Compression {
	case Gzip:
		compressedWriter = gzip.NewWriter(output)
	case Bzip2:
		// Use dsnet bzip2 writer
		bw, err := bzip2.NewWriter(output, &bzip2.WriterConfig{Level: 9})
		if err != nil {
			return err
		}
		compressedWriter = bw
	case Xz:
		compressedWriter, err = xz.NewWriter(output)
		if err != nil {
			return err
		}
	case None:
		// No compression, write directly to output
		compressedWriter = &nopCloser{output}
	}
	defer compressedWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(compressedWriter)
	defer tarWriter.Close()

	// Get absolute path to the output file to avoid archiving it
	var absOutputPath string
	if config.OutputFile != "" {
		absOutputPath, _ = filepath.Abs(config.OutputFile)
	}

	// Archive files based on mode
	if len(filesToArchive) > 0 {
		// Archive only specific files (changed files mode)
		return archiveSpecificFiles(config, patterns, filesToArchive, tarWriter, absOutputPath)
	} else {
		// Archive all files (full repository mode)
		return archiveAllFiles(config, patterns, tarWriter, absOutputPath)
	}
}

// archiveAllFiles archives all files in the repository (original behavior)
func archiveAllFiles(config Config, patterns []string, tarWriter *tar.Writer, absOutputPath string) error {
	// Walk through the repository
	return filepath.Walk(config.RepositoryPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the output file itself
		if config.OutputFile != "" {
			absPath, _ := filepath.Abs(path)
			if absPath == absOutputPath {
				return nil
			}
		}

		// Check if file should be ignored
		if shouldIgnore(path, config.RepositoryPath, patterns) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Get relative path for archive
		relPath, err := filepath.Rel(config.RepositoryPath, path)
		if err != nil {
			return err
		}

		// Skip the root directory itself
		if relPath == "." {
			return nil
		}

		return addFileToArchive(tarWriter, path, relPath, info)
	})
}

// archiveSpecificFiles archives only the specified files
func archiveSpecificFiles(config Config, patterns []string, filesToArchive []string, tarWriter *tar.Writer, absOutputPath string) error {
	for _, filePath := range filesToArchive {
		// Skip the output file itself
		if config.OutputFile != "" {
			absPath, _ := filepath.Abs(filePath)
			if absPath == absOutputPath {
				continue
			}
		}

		// Check if file should be ignored
		if shouldIgnore(filePath, config.RepositoryPath, patterns) {
			continue
		}

		// Check if file exists
		info, err := os.Stat(filePath)
		if err != nil {
			// File might have been deleted, skip it
			continue
		}

		// Get relative path for archive
		relPath, err := filepath.Rel(config.RepositoryPath, filePath)
		if err != nil {
			return err
		}

		if err := addFileToArchive(tarWriter, filePath, relPath, info); err != nil {
			return err
		}
	}
	return nil
}

// addFileToArchive adds a single file to the tar archive
func addFileToArchive(tarWriter *tar.Writer, filePath, relPath string, info os.FileInfo) error {
	// Create tar header
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}

	// Set the name to the relative path
	header.Name = relPath

	// Write header
	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}

	// If it's a file, write the content
	if !info.IsDir() {
		file, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tarWriter, file)
		if err != nil {
			return err
		}
	}

	return nil
}

// nopCloser wraps an io.Writer with a no-op Close method
type nopCloser struct {
	io.Writer
}

func (nc *nopCloser) Close() error {
	return nil
}
