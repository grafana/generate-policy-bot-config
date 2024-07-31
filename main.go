package main

import (
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"

	"github.com/jessevdk/go-flags"
	"github.com/lmittmann/tint"
	"github.com/palantir/policy-bot/policy"
	"github.com/willabides/actionslog"
	"github.com/willabides/actionslog/human"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

const header = `# This file is generated by %s.
# Do not edit directly. Run "make .policy.yml" to update
`

const usage = `%s [path/to/deployment_tools]

Discovers GitHub Actions workflows and generates a policy bot configuration file
which enforces that they pass. If paths are specified in the workflow, the policy
will enforce that the workflow passes only when the paths are modified in the
PR.`

// rootDir represents the root directory to search for workflows. It is a
// wrapper around fs.FS which reads from a directory when unmarshaled from a
// flag. It exists so that the filesystem can be faked in tests.
type rootDir struct {
	fs.FS
}

func (rd *rootDir) UnmarshalFlag(value string) error {
	*rd = rootDir{os.DirFS(value)}
	return nil
}

// reader represents the config to merge with the generated config. If the value
// is "-", read from standard input. If the value is empty, no merging occurs.
// Otherwise, read from the file at the given path. It is a wrapper around an
// `io.Reader` so that it can be unmarshaled from a flag straight to a reader
// and faked in tests.
type reader struct {
	io.Reader
}

func (m *reader) UnmarshalFlag(value string) error {
	if value == "" {
		*m = reader{}
		return nil
	}

	if value == "-" {
		*m = reader{os.Stdin}
		return nil
	}

	file, err := os.Open(value)
	if err != nil {
		return fmt.Errorf("failed to open merge file: %w", err)
	}

	*m = reader{file}

	return nil
}

type level slog.Level

func (l *level) UnmarshalFlag(value string) error {
	var lvl slog.Level
	err := lvl.UnmarshalText([]byte(value))
	if err != nil {
		return fmt.Errorf("invalid log level: %w", err)
	}

	*l = level(lvl)
	return nil
}

type rootArgs struct {
	Root rootDir
}

type appFlags struct {
	OutputWriter *renamingWriter `long:"output" short:"o" description:"Output file. If this is \"-\", write to standard output" default:".policy.yml"`
	LogLevel     *level          `long:"log-level" short:"l" description:"Log level"`
	MergeConfig  reader          `long:"merge-with" short:"m" description:"File to merge with generated config. If this is \"-\", read from standard input. If empty, no merging occurs."`

	Args rootArgs `positional-args:"yes" required:"yes"`
}

// listWorkflows returns a list of all the workflows under the root directory
// given in the arguments.
func (af *appFlags) listWorkflows() ([]string, error) {
	ymlFiles, err := fs.Glob(af.Args.Root, ".github/workflows/*.yml")
	if err != nil {
		return nil, fmt.Errorf("failed to list workflows: %w", err)
	}

	yamlFiles, err := fs.Glob(af.Args.Root, ".github/workflows/*.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to list workflows: %w", err)
	}

	allWorkflows := append(ymlFiles, yamlFiles...)

	if len(allWorkflows) == 0 {
		return nil, errNoWorkflows{}
	}

	slog.Debug("Found workflows", "num_workflows", len(allWorkflows))
	return allWorkflows, nil
}

// parsePRWorkflows parses all the workflows under the root directory given in
// the arguments and returns a map of the workflows that are `pull_request`
// or `pull_request_target` workflows. The key is the path to the workflow file
// and the value is the parsed workflow. Workflows that are not `pull_request`
// or `pull_request_target`, as well as invalid workflows which cannot be
// parsed or read, are ignored. The only way this function can fail is if it
// encounters an error while listing the workflows.
func (af *appFlags) parsePRWorkflows() (gitHubWorkflowCollection, error) {
	paths, err := af.listWorkflows()
	if err != nil {
		return nil, err
	}

	workflows := make(map[string]gitHubWorkflow)

	for _, workflowPath := range paths {
		contents, err := fs.ReadFile(af.Args.Root, workflowPath)
		if err != nil {
			slog.Warn("failed to read workflow", "path", workflowPath, "error", err)
			continue
		}

		slog.Debug("parsing workflow", "path", workflowPath)
		var workflow gitHubWorkflow
		err = yaml.Unmarshal(contents, &workflow)
		if err != nil {
			err = errInvalidWorkflow{Path: workflowPath, Err: err}
			slog.Warn("failed to parse workflow", "path", workflowPath, "error", err)
			continue
		}

		if !workflow.isPullRequestWorkflow() {
			slog.Debug("skipping non-PR workflow", "path", workflowPath)
			continue
		}

		// We can't assume that there will be a workflow run if the PR doesn't
		// run on `synchronize`, so we skip enforcing the workflow for this
		// case. For example, if `types` is `[opened]`, a run will only be
		// present when the PR is first opened but if it gets another push then
		// there won't be one.
		if !workflow.runsOnSynchronize() {
			slog.Debug("skipping workflow that doesn't run on synchronize", "path", workflowPath)
			continue
		}

		workflows[workflowPath] = workflow
	}

	return workflows, nil
}

// loadConfigFromReader reads a policy bot config from the given reader. This is
// used to merge the generated config with an existing config.
func loadConfigFromReader(r io.Reader) (policy.Config, error) {
	var config policy.Config
	decoder := yaml.NewDecoder(r)
	if err := decoder.Decode(&config); err != nil {
		return policy.Config{}, errInvalidPolicyBotConfig{Err: err}
	}
	return config, nil
}

func (af *appFlags) run(name string) error {
	dest := af.OutputWriter
	defer dest.Close()

	// Find and parse all the workflows
	workflows, err := af.parsePRWorkflows()
	if err != nil {
		if abortErr := af.OutputWriter.Abort(); abortErr != nil {
			slog.Warn("failed to abort", "error", abortErr)
		}
		return err
	}

	// Generate a policy bot config from them
	config := workflows.policyBotConfig()

	// Merge the generated config with an existing config, if one was provided
	if af.MergeConfig.Reader != nil {
		mergeConfig, err := loadConfigFromReader(af.MergeConfig)
		if err != nil {
			return err
		}

		config, err = mergeConfigs(config, mergeConfig)
		if err != nil {
			return fmt.Errorf("failed to merge generated config with existing config: %w", err)
		}
	}

	// Write the config to the output file
	fmt.Fprintf(dest, header, name)
	if err := writeYamlToWriter(dest, config); err != nil {
		if abortErr := af.OutputWriter.Abort(); abortErr != nil {
			slog.Warn("failed to abort", "error", abortErr)
		}
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func setupLogger() *slog.LevelVar {
	var lv slog.LevelVar

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: &lv,
	}))
	defer func() { slog.SetDefault(logger) }()

	if os.Getenv("GITHUB_ACTIONS") == "true" {
		logger = slog.New(&actionslog.Wrapper{
			Handler: (&human.Handler{
				AddSource:   true,
				ExcludeTime: true,
				Level:       &lv,
			}).WithOutput,
			Output: os.Stderr,
		})

		if os.Getenv("RUNNER_DEBUG") == "1" {
			lv.Set(slog.LevelDebug)
		}
		return &lv
	}

	if term.IsTerminal(int(os.Stderr.Fd())) {
		logger = slog.New(tint.NewHandler(os.Stderr, &tint.Options{
			AddSource: true,
			Level:     &lv,
		}))
	}

	return &lv
}

func main() {
	lv := setupLogger()

	var conf appFlags
	defer func() {
		err := conf.OutputWriter.Abort()
		if err != nil {
			slog.Warn("attempted to abort when exiting, but failed", "error", err)
		}
	}()

	parser := flags.NewParser(&conf, flags.Default)
	parser.Usage = fmt.Sprintf(usage, parser.Name)

	if _, err := parser.Parse(); err != nil {
		switch err.(type) {
		// The flags package prints its own error messages, don't repeat them
		case flags.ErrorType:
		default:
			slog.Error(err.Error())
			if !flags.WroteHelp(err) {
				parser.WriteHelp(os.Stderr)
			}
		}
		os.Exit(1)
	}

	if conf.LogLevel != nil {
		lv.Set(slog.Level(*conf.LogLevel))
	}
	slog.Debug("debug logging enabled")

	if err := conf.run(parser.Name); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
