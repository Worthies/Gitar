# Gitar

Archive and extract Git repositories based on .gitignore🚀🚀🚀

## Description

Gitar is a command-line tool that archives and extracts Git repositories while respecting `.gitignore` rules.

**Archive Mode**: Creates a tar archive containing only the files that would be tracked by Git, excluding all files and directories specified in `.gitignore`. Supports multiple compression formats (gzip, bzip2, xz) and base64 encoding.

**Extract Mode**: Extracts tar archives (with optional compression) to a specified directory. Features automatic compression detection, cross-platform path handling, and security validation to prevent directory traversal attacks.

## Installation

**Requirements:** Go 1.22 or later

```bash
go install github.com/worthies/gitar@latest
```

Or build from source:

```bash
git clone https://github.com/Worthies/Gitar.git
cd Gitar
go build -o gitar
```

## Usage

```bash
# Archive mode
gitar [options] <repository-path>

# Extract mode
gitar --extract [options]
```

### Options

#### Compression Options (works for both archive and extract)
- `-z, --gzip`: Use gzip compression (default)
- `-j, --bzip2`: Use bzip2 compression
- `-J, --xz`: Use xz compression
- `--no-compression`: No compression (plain tar)
- `-b, --base64`: Base64 encode (archive) or decode (extract)

#### Archive Options
- `-o, --output <file>`: Output archive to file instead of stdout
- `--start-ref <ref>`: Start Git reference to compare changes from
- `--end-ref <ref>`: End Git reference to compare changes to (defaults to HEAD)

#### Extract Options
- `-x, --extract`: Extract mode: extract archive instead of creating
- `-f, --archive <file>`: Archive file to extract (use '-' for stdin)
- `-C, --directory <dir>`: Output directory for extraction (defaults to current directory)

#### General
- `-h, --help`: Show help message

### Archive Examples

```bash
# Archive the current directory to stdout (gzipped tar)
gitar .

# Archive to a specific file
gitar -o my-project.tar.gz /path/to/my-project

# Archive with base64 encoding to stdout
gitar -b .

# Archive with bzip2 compression to file
gitar -j -o archive.tar.bz2 /path/to/repo

# Archive with xz compression
gitar -J -o archive.tar.xz /path/to/repo

# Archive without compression
gitar --no-compression -o archive.tar .

# Archive only files changed since main branch
gitar --start-ref main -o changes.tar.gz .

# Archive files changed between two specific commits
gitar --start-ref v1.0.0 --end-ref v2.0.0 -o changes.tar.gz .
```

### Extract Examples

```bash
# Extract archive to current directory (auto-detects compression)
gitar -x -f archive.tar.gz

# Extract xz archive (explicit compression type)
gitar -x -J -f archive.tar.xz

# Extract to specific directory
gitar -x -f archive.tar.gz -C /path/to/output

# Extract base64-encoded archive
gitar -x -f archive.b64 -b

# Extract from stdin (defaults to gzip)
cat archive.tar.gz | gitar -x

# Extract bzip2 from stdin via pipe
cat archive.tar.bz2 | gitar -x -j

# Extract from stdin (explicit)
cat archive.tar.gz | gitar -x -f -

# Extract from stdin without specifying file
gitar -x < archive.tar.gz
```

## Features

### Archive Mode
- ✅ Respects `.gitignore` patterns (wildcards, directories, specific files)
- ✅ Always excludes the `.git` directory
- ✅ Excludes the archive file itself from being archived
- ✅ Handles nested directory structures
- ✅ Supports differential archiving (only changed files between refs)
- ✅ Multiple compression formats (gzip, bzip2, xz)
- ✅ Optional base64 encoding

### Extract Mode
- ✅ Automatic compression detection from file extension
- ✅ Cross-platform path handling (Windows/Unix)
- ✅ Security: path validation prevents directory traversal attacks
- ✅ Extracts from stdin or file
- ✅ Supports gzip, bzip2, xz, and uncompressed tar
- ✅ Optional base64 decoding
- ✅ Handles symlinks and hard links

### Cross-Platform Compatibility
- ✅ Handles archives created on Windows when extracting on Linux
- ✅ Handles archives created on Linux when extracting on Windows
- ✅ Properly converts path separators (`\` ↔ `/`)

## License

See [LICENSE](LICENSE) file for details.
