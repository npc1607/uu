package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type File struct {
	Router     string
	Model      string
	InstallDir string
	LogDir     string
}

func Load(path string) (File, error) {
	file, err := os.Open(path)
	if err != nil {
		return File{}, err
	}
	defer file.Close()

	var cfg File
	scanner := bufio.NewScanner(file)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(stripComment(scanner.Text()))
		if line == "" || line == "---" || line == "..." {
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return File{}, fmt.Errorf("%s:%d: expected key: value", path, lineNo)
		}
		key = normalizeKey(key)
		value = unquote(strings.TrimSpace(value))

		switch key {
		case "router":
			cfg.Router = value
		case "model":
			cfg.Model = value
		case "install_dir":
			cfg.InstallDir = value
		case "log_dir":
			cfg.LogDir = value
		default:
			return File{}, fmt.Errorf("%s:%d: unknown config key %q", path, lineNo, key)
		}
	}
	if err := scanner.Err(); err != nil {
		return File{}, err
	}
	return cfg, nil
}

func (c File) Apply(opts *Options) {
	if c.Router != "" {
		opts.Router = c.Router
	}
	if c.Model != "" {
		opts.Model = c.Model
	}
	if c.InstallDir != "" {
		opts.InstallDir = c.InstallDir
	}
	if c.LogDir != "" {
		opts.LogDir = c.LogDir
	}
}

func normalizeKey(key string) string {
	key = strings.TrimSpace(strings.ToLower(key))
	return strings.ReplaceAll(key, "-", "_")
}

func stripComment(line string) string {
	inSingle := false
	inDouble := false
	for i, r := range line {
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return line[:i]
			}
		}
	}
	return line
}

func unquote(value string) string {
	if len(value) < 2 {
		return value
	}
	if (value[0] == '\'' && value[len(value)-1] == '\'') ||
		(value[0] == '"' && value[len(value)-1] == '"') {
		return value[1 : len(value)-1]
	}
	return value
}
