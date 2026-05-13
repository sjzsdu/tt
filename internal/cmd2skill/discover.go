package cmd2skill

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

func Discover(commandName string, opts Options) (*CLIModel, error) {
	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Second
	}
	if opts.MaxCommands <= 0 {
		opts.MaxCommands = 200
	}
	model := &CLIModel{Name: commandName}
	visited := map[string]bool{}
	count := 0
	root, err := discoverNode(strings.Fields(commandName), opts, 0, visited, &count, model)
	if err != nil {
		return nil, err
	}
	model.Root = root
	return model, nil
}

func discoverNode(path []string, opts Options, level int, visited map[string]bool, count *int, model *CLIModel) (*CommandNode, error) {
	key := strings.Join(path, "\x00")
	if visited[key] {
		return nil, fmt.Errorf("cycle detected at %s", strings.Join(path, " "))
	}
	visited[key] = true
	*count++
	if *count > opts.MaxCommands {
		return nil, fmt.Errorf("max commands reached: %d", opts.MaxCommands)
	}

	help, err := runHelp(path, opts.Timeout)
	if err != nil && len(path) == 1 {
		help, err = runCommand(opts.Timeout, "man", "-P", "cat", path[0])
	}
	if err != nil {
		return nil, err
	}

	node := parseHelp(help, path)
	if opts.Examples {
		node.Examples = extractExamplesFromHelp(help)
	}
	if level >= opts.Depth {
		return node, nil
	}

	children := append([]*CommandNode(nil), node.Children...)
	sort.SliceStable(children, func(i, j int) bool { return children[i].Name < children[j].Name })
	node.Children = nil
	for _, child := range children {
		childPath := append(append([]string{}, path...), child.Name)
		childNode, err := discoverNode(childPath, opts, level+1, visited, count, model)
		if err != nil {
			model.Failures = append(model.Failures, DiscoveryFailure{Path: childPath, Error: err.Error()})
			node.Children = append(node.Children, child)
			continue
		}
		if child.Description != "" && childNode.Description == "" {
			childNode.Description = child.Description
		}
		if child.Section != "" {
			childNode.Section = child.Section
		}
		node.Children = append(node.Children, childNode)
	}
	return node, nil
}

func runHelp(path []string, timeout time.Duration) (string, error) {
	args := append([]string{}, path[1:]...)
	args = append(args, "--help")
	out, err := runCommand(timeout, path[0], args...)
	if err == nil {
		return out, nil
	}
	args = append([]string{"help"}, path[1:]...)
	return runCommand(timeout, path[0], args...)
}

func runCommand(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("%s %s timed out after %s", name, strings.Join(args, " "), timeout)
	}
	if err != nil {
		return "", fmt.Errorf("%s %s: %w - %s", name, strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String(), nil
}
