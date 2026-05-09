package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type File struct {
	Router           string
	Model            string
	InstallDir       string
	LogDir           string
	FollowLogs       *bool
	FollowLogFile    string
	FollowLogLines   *int
	FollowLogTimeout *time.Duration
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
		case "follow_logs":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return File{}, fmt.Errorf("%s:%d: invalid bool for follow_logs", path, lineNo)
			}
			cfg.FollowLogs = &parsed
		case "follow_log_file":
			cfg.FollowLogFile = value
		case "follow_log_lines":
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 0 {
				return File{}, fmt.Errorf("%s:%d: invalid non-negative integer for follow_log_lines", path, lineNo)
			}
			cfg.FollowLogLines = &parsed
		case "follow_log_timeout":
			parsed, err := parseDuration(value)
			if err != nil {
				return File{}, fmt.Errorf("%s:%d: invalid duration for follow_log_timeout", path, lineNo)
			}
			cfg.FollowLogTimeout = &parsed
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
	if c.FollowLogs != nil {
		opts.FollowLogs = *c.FollowLogs
	}
	if c.FollowLogFile != "" {
		opts.FollowLogFile = c.FollowLogFile
	}
	if c.FollowLogLines != nil {
		opts.FollowLogLines = *c.FollowLogLines
	}
	if c.FollowLogTimeout != nil {
		opts.FollowLogTimeout = *c.FollowLogTimeout
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

func parseDuration(value string) (time.Duration, error) {
	if value == "" || value == "0" {
		return 0, nil
	}
	return time.ParseDuration(value)
}
