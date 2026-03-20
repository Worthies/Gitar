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
	OutputFile           string
	Compression          CompressionType
	CompressionSpecified bool // Whether compression was explicitly specified by user
	Base64               bool // Base64 encode (archive mode) or decode (extract mode)
	RepositoryPath       string
	ShowHelp             bool
	StartRef             string
	EndRef               string
	ExtractMode          bool
	ExtractArchive       string
	OutputDir            string
}

func main() {
	config := parseFlags()

	if config.ShowHelp {
		printHelp()
		return
	}

	// Handle extract mode
	if config.ExtractMode {
		// Archive file is optional - can use stdin if not specified or if "-"
		if err := extractArchive(config); err != nil {
			fmt.Fprintf(os.Stderr, "Error extracting archive: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Archive mode
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

	flag.BoolVar(&config.Base64, "b", false, "Base64 encode (archive) or decode (extract)")
	flag.BoolVar(&config.Base64, "base64", false, "Base64 encode (archive) or decode (extract)")

	// Extract mode flags
	flag.BoolVar(&config.ExtractMode, "extract", false, "Extract mode: extract archive instead of creating")
	flag.BoolVar(&config.ExtractMode, "x", false, "Extract mode: extract archive instead of creating (shorthand)")
	flag.StringVar(&config.ExtractArchive, "archive", "", "Archive file to extract (use '-' for stdin)")
	flag.StringVar(&config.ExtractArchive, "f", "", "Archive file to extract (use '-' for stdin, shorthand)")
	flag.StringVar(&config.OutputDir, "directory", "", "Output directory for extraction (defaults to current directory)")
	flag.StringVar(&config.OutputDir, "C", "", "Output directory for extraction (defaults to current directory, shorthand)")

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
		config.CompressionSpecified = true
	} else if *xzFlag || *xzLongFlag {
		config.Compression = Xz
		config.CompressionSpecified = true
	} else if *noCompressionFlag {
		config.Compression = None
		config.CompressionSpecified = true
	} else if *gzipFlag || *gzipLongFlag {
		config.Compression = Gzip
		config.CompressionSpecified = true
	}

	return config
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: gitar [options] <repository-path>\n")
}

func printHelp() {
	fmt.Println("Gitar - Archive/Extract Git repositories respecting .gitignore rules")
	fmt.Println()
	printUsage()
	fmt.Println()
	fmt.Println("Compression Options (works for both archive and extract):")
	fmt.Println("  -z, --gzip              Use gzip compression (default)")
	fmt.Println("  -j, --bzip2             Use bzip2 compression")
	fmt.Println("  -J, --xz                Use xz compression")
	fmt.Println("  --no-compression        No compression (plain tar)")
	fmt.Println("  -b, --base64            Base64 encode (archive) or decode (extract)")
	fmt.Println()
	fmt.Println("Archive Options:")
	fmt.Println("  -o, --output <file>     Output archive to file instead of stdout")
	fmt.Println("  --start-ref <ref>       Start Git reference to compare changes from")
	fmt.Println("  --end-ref <ref>         End Git reference to compare changes to (defaults to HEAD)")
	fmt.Println()
	fmt.Println("Extract Options:")
	fmt.Println("  -x, --extract           Extract mode: extract archive instead of creating")
	fmt.Println("  -f, --archive <file>    Archive file to extract (use '-' for stdin)")
	fmt.Println("  -C, --directory <dir>   Output directory for extraction (defaults to current directory)")
	fmt.Println()
	fmt.Println("General:")
	fmt.Println("  -h, --help              Show this help message")
	fmt.Println()
	fmt.Println("Archive Examples:")
	fmt.Println("  gitar .                                    # Archive all files in current directory")
	fmt.Println("  gitar -o archive.tar.gz /path/to/repo      # Archive to file")
	fmt.Println("  gitar -j -o repo.tar.bz2 /path/to/repo     # Archive with bzip2")
	fmt.Println("  gitar -b .                                 # Archive with base64 encoding")
	fmt.Println("  gitar --start-ref main .                   # Archive only files changed since main branch")
	fmt.Println("  gitar --start-ref v1.0 --end-ref v2.0 .   # Archive files changed between v1.0 and v2.0")
	fmt.Println()
	fmt.Println("Extract Examples:")
	fmt.Println("  gitar -x -f archive.tar.gz                # Extract to current directory")
	fmt.Println("  gitar -x -J -f archive.tar.xz             # Extract xz archive")
	fmt.Println("  gitar -x -f archive.tar.gz -C /tmp/output # Extract to specific directory")
	fmt.Println("  gitar -x -f archive.b64 -b                # Extract base64-encoded archive")
	fmt.Println("  gitar -x                                  # Extract from stdin (defaults to gzip)")
	fmt.Println("  gitar -x -j -f -                          # Extract bzip2 from stdin")
	fmt.Println("  cat archive.tar.gz | gitar -x             # Extract from pipe")
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
	if config.Base64 {
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
	// Use Lstat to get the file's own info (don't follow symlinks)
	lstat, err := os.Lstat(filePath)
	if err != nil {
		return err
	}

	var header *tar.Header

	// Handle symlinks specially
	if lstat.Mode()&os.ModeSymlink != 0 {
		// Read the symlink target
		linkTarget, err := os.Readlink(filePath)
		if err != nil {
			return err
		}
		header = &tar.Header{
			Name:     relPath,
			Mode:     int64(lstat.Mode().Perm()),
			Typeflag: tar.TypeSymlink,
			Linkname: linkTarget,
			ModTime:  lstat.ModTime(),
			// Size for symlinks should be 0
		}
	} else {
		// For regular files and directories, use FileInfoHeader
		header, err = tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath
	}

	// Write header
	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}

	// If it's a regular file (not a directory or symlink), write the content
	if !info.IsDir() && lstat.Mode()&os.ModeSymlink == 0 {
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

// nopReadCloser wraps an io.Reader with a no-op Close method
type nopReadCloser struct {
	io.Reader
}

func (nc *nopReadCloser) Close() error {
	return nil
}

// detectCompressionType auto-detects compression type from file extension
func detectCompressionType(filename string) CompressionType {
	lower := strings.ToLower(filename)

	// Check extensions in order of specificity
	if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		return Gzip
	}
	if strings.HasSuffix(lower, ".tar.bz2") || strings.HasSuffix(lower, ".tbz2") || strings.HasSuffix(lower, ".tbz") {
		return Bzip2
	}
	if strings.HasSuffix(lower, ".tar.xz") || strings.HasSuffix(lower, ".txz") {
		return Xz
	}
	if strings.HasSuffix(lower, ".tar") {
		return None
	}
	// Fallback: check for single compression extensions
	if strings.HasSuffix(lower, ".gz") {
		return Gzip
	}
	if strings.HasSuffix(lower, ".bz2") {
		return Bzip2
	}
	if strings.HasSuffix(lower, ".xz") {
		return Xz
	}

	// Default to gzip for unknown extensions
	return Gzip
}

// extractArchive extracts an archive to the specified directory
func extractArchive(config Config) error {
	// Determine output directory (default to current directory)
	outputDir := config.OutputDir
	if outputDir == "" {
		outputDir = "."
	}

	// Convert to absolute path for validation
	absOutputDir, err := filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("failed to resolve output directory: %w", err)
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(absOutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Determine input source: file or stdin
	// Use stdin if archive file is "-" or if no archive file was specified
	useStdin := config.ExtractArchive == "-" || config.ExtractArchive == ""

	var input io.Reader
	if useStdin {
		input = os.Stdin
	} else {
		file, err := os.Open(config.ExtractArchive)
		if err != nil {
			return fmt.Errorf("failed to open archive: %w", err)
		}
		defer file.Close()
		input = file
	}

	// Handle base64 decoding if requested
	if config.Base64 {
		decoder := base64.NewDecoder(base64.StdEncoding, input)
		input = decoder
	}

	// Determine compression type
	compression := config.Compression
	// Auto-detect compression from filename if not explicitly specified and not reading from stdin
	if !config.CompressionSpecified && !useStdin && config.ExtractArchive != "" {
		compression = detectCompressionType(config.ExtractArchive)
	}
	// If reading from stdin and compression not specified, default to gzip
	if !config.CompressionSpecified && useStdin {
		compression = Gzip
	}

	// Decompress based on type
	var tarReader io.ReadCloser
	switch compression {
	case Gzip:
		gzReader, err := gzip.NewReader(input)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzReader.Close()
		tarReader = gzReader
	case Bzip2:
		bzReader, err := bzip2.NewReader(input, &bzip2.ReaderConfig{})
		if err != nil {
			return fmt.Errorf("failed to create bzip2 reader: %w", err)
		}
		tarReader = &nopReadCloser{bzReader}
	case Xz:
		xzReader, err := xz.NewReader(input)
		if err != nil {
			return fmt.Errorf("failed to create xz reader: %w", err)
		}
		tarReader = &nopReadCloser{xzReader}
	case None:
		tarReader = &nopReadCloser{input}
	default:
		return fmt.Errorf("unsupported compression type: %v", compression)
	}

	// Extract tar contents
	return extractTar(tarReader, absOutputDir)
}

// extractTar extracts tar contents to the output directory with security validation
func extractTar(tarReader io.Reader, outputDir string) error {
	tr := tar.NewReader(tarReader)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		// Sanitize the path to prevent directory traversal attacks
		targetPath, err := sanitizeFilePath(outputDir, header.Name)
		if err != nil {
			return fmt.Errorf("invalid path '%s': %w", header.Name, err)
		}

		// Handle different file types
		switch header.Typeflag {
		case tar.TypeDir:
			// Create directory
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory '%s': %w", targetPath, err)
			}

		case tar.TypeReg, tar.TypeRegA:
			// Create parent directory if needed
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory: %w", err)
			}

			// Create file
			file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file '%s': %w", targetPath, err)
			}

			// Copy file contents
			if _, err := io.Copy(file, tr); err != nil {
				file.Close()
				return fmt.Errorf("failed to write file '%s': %w", targetPath, err)
			}
			file.Close()

		case tar.TypeSymlink:
			// Get link target
			linkTarget := header.Linkname

			// Sanitize symlink target to prevent directory traversal
			// If the symlink is absolute, we need to be careful
			if filepath.IsAbs(linkTarget) {
				// For absolute symlinks, we'll keep them as-is but warn
				fmt.Fprintf(os.Stderr, "Warning: absolute symlink '%s' -> '%s'\n", targetPath, linkTarget)
			} else {
				// For relative symlinks, sanitize the target
				linkTarget = filepath.ToSlash(linkTarget) // Normalize separators
			}

			// Create parent directory if needed
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory: %w", err)
			}

			// Create symlink
			if err := os.Symlink(linkTarget, targetPath); err != nil {
				// Symlink creation might fail on Windows without developer mode
				// or without admin privileges. Log a warning and continue.
				fmt.Fprintf(os.Stderr, "Warning: failed to create symlink '%s': %v\n", targetPath, err)
			}

		case tar.TypeLink:
			// Hard link
			linkTarget, err := sanitizeFilePath(outputDir, header.Linkname)
			if err != nil {
				return fmt.Errorf("invalid hard link target '%s': %w", header.Linkname, err)
			}

			// Create parent directory if needed
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory: %w", err)
			}

			// Create hard link
			if err := os.Link(linkTarget, targetPath); err != nil {
				return fmt.Errorf("failed to create hard link '%s' -> '%s': %w", targetPath, linkTarget, err)
			}

		default:
			// Skip unsupported file types (devices, fifos, etc.)
			fmt.Fprintf(os.Stderr, "Warning: skipping unsupported file type '%c' for '%s'\n", header.Typeflag, header.Name)
		}
	}

	return nil
}

// sanitizeFilePath sanitizes a file path from a tar archive to prevent directory traversal attacks
// It handles both Unix (/) and Windows (\) path separators correctly
func sanitizeFilePath(outputDir, archivePath string) (string, error) {
	// First, explicitly replace all backslashes with forward slashes
	// This is necessary because filepath.ToSlash() only converts the OS-specific separator.
	// On Linux, backslashes are NOT the path separator, so ToSlash() won't convert them.
	// We need to handle Windows-created archives regardless of the platform we're running on.
	archivePath = strings.ReplaceAll(archivePath, "\\", "/")

	// Also use filepath.ToSlash for OS-specific separator conversion
	archivePath = filepath.ToSlash(archivePath)

	// Remove any leading slashes or drive letters
	archivePath = strings.TrimPrefix(archivePath, "/")

	// Windows drive letter handling (e.g., "C:/path" or "C:\path")
	if len(archivePath) >= 2 && archivePath[1] == ':' {
		// Remove the drive letter prefix
		archivePath = archivePath[2:]
		archivePath = strings.TrimPrefix(archivePath, "/")
	}

	// Split path and validate each component doesn't contain directory traversal
	parts := strings.Split(archivePath, "/")
	var cleanParts []string
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return "", fmt.Errorf("path contains directory traversal component '..'")
		}
		// Check for Windows-style traversal in the component itself
		if strings.Contains(part, "..") {
			return "", fmt.Errorf("path contains potentially dangerous component '%s'", part)
		}
		cleanParts = append(cleanParts, part)
	}

	// Join with OS-specific separator
	relPath := filepath.Join(cleanParts...)

	// Join with output directory
	targetPath := filepath.Join(outputDir, relPath)

	// Verify the target path is within the output directory
	absOutputDir, err := filepath.Abs(outputDir)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path for output directory: %w", err)
	}

	absTargetPath, err := filepath.Abs(targetPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path for target: %w", err)
	}

	// Check if the target path is within the output directory
	relPathFromOutput, err := filepath.Rel(absOutputDir, absTargetPath)
	if err != nil {
		return "", fmt.Errorf("failed to validate path: %w", err)
	}

	// If the relative path starts with "..", the target is outside the output directory
	if strings.HasPrefix(relPathFromOutput, "..") {
		return "", fmt.Errorf("path '%s' is outside the output directory", archivePath)
	}

	return targetPath, nil
}
