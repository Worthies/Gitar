# Gitar
Archive Git objects based on .gitignore🚀🚀🚀

## Description

Gitar is a command-line tool that archives Git repositories while respecting `.gitignore` rules. It creates a zip archive containing only the files that would be tracked by Git, excluding all files and directories specified in `.gitignore`.

## Installation

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
gitar <repository-path>
```

### Example

```bash
# Archive the current directory
gitar .

# Archive a specific repository
gitar /path/to/my-project
```

This will create a zip file named after the repository (e.g., `my-project.zip`) containing all files except those matched by `.gitignore` patterns.

## Features

- ✅ Respects `.gitignore` patterns (wildcards, directories, specific files)
- ✅ Always excludes the `.git` directory
- ✅ Excludes the archive file itself from being archived
- ✅ Handles nested directory structures
- ✅ Provides clear error messages for invalid inputs
- ✅ Works with repositories that don't have a `.gitignore` file

## License

See [LICENSE](LICENSE) file for details.
