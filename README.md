# Gitar

Archive Git objects based on .gitignore🚀🚀🚀

## Description

Gitar is a command-line tool that archives Git repositories while respecting `.gitignore` rules. It creates a tar archive with gzip compression containing only the files that would be tracked by Git, excluding all files and directories specified in `.gitignore`. By default, the archive is output to stdout.

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
gitar [options] <repository-path>
```

### Options

- `-o, --output <file>`: Output archive to file instead of stdout
- `--start-ref <ref>`: Start Git reference to compare changes from
- `--end-ref <ref>`: End Git reference to compare changes to (defaults to HEAD)
- `-z, --gzip`: Use gzip compression (default)
- `-j, --bzip2`: Use bzip2 compression
- `-J, --xz`: Use xz compression
- `--no-compression`: Create uncompressed tar archive
- `-b, --base64`: Encode output in base64
- `-h, --help`: Show help message

### Examples

```bash
# Archive the current directory to stdout (gzipped tar)
gitar .

# Archive to a specific file
gitar -o my-project.tar.gz /path/to/my-project

# Archive with base64 encoding to stdout
gitar -b .

# Archive with bzip2 compression to file
gitar -j -o archive.tar.bz2 /path/to/repo

# Archive without compression
gitar --no-compression -o archive.tar .

# Archive only files changed since main branch
gitar --start-ref main -o changes.tar.gz .

# Archive files changed between two specific commits
gitar --start-ref v1.0.0 --end-ref v2.0.0 -o changes.tar.gz .
```

The default behavior creates a gzip-compressed tar archive and outputs it to stdout, making it easy to pipe to other commands or redirect to files.

## Features

- ✅ Respects `.gitignore` patterns (wildcards, directories, specific files)
- ✅ Always excludes the `.git` directory
- ✅ Excludes the archive file itself from being archived
- ✅ Handles nested directory structures
- ✅ Provides clear error messages for invalid inputs
- ✅ Works with repositories that don't have a `.gitignore` file

## License

See [LICENSE](LICENSE) file for details.
