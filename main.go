package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/saquib.mian/pgit/logwriter"
)

const (
	version = "0.1"
	runfile = "prun.json"
	timeout = time.Minute * 30
)

var (
	maxconcurrency     = 4
	excludeDirectories string
)

func init() {
	flag.StringVar(&excludeDirectories, "exclude", "", "directories to exclude from the command")
	flag.IntVar(&maxconcurrency, "n", 4, "number of commands to run at a time")
	flag.Parse()
}

// Command is a representation of a program to run
type Command struct {
	WorkingDir string
	Command    string
	Args       []string
}

type CommandResult struct {
	Success bool
	Error   error
	Command Command
}

func (c *Command) String() string {
	return fmt.Sprintf("'%s %s' in '%s'", c.Command, strings.Join(c.Args, " "), c.WorkingDir)
}

func main() {
	fmt.Printf("pgit v%s\n", version)

	additionalArgs := flag.Args()

	input := make(chan Command)
	output := make(chan CommandResult)

	// start workers
	for i := 1; i <= maxconcurrency; i++ {
		go worker(i, input, output)
	}

	repos := []string{}
	dirs, _ := ioutil.ReadDir("./")
	excludedDirs := strings.Split(excludeDirectories, ",")
includedDirectories:
	for _, dir := range dirs {
		if !dir.IsDir() || strings.HasSuffix(dir.Name(), ".git") {
			// not a directory
			continue
		}
		if _, err := os.Stat(filepath.Join(dir.Name(), ".git")); os.IsNotExist(err) {
			// not a git repo
			continue
		}

		// exclude certain dirs
		for _, excludedDir := range excludedDirs {
			if strings.EqualFold(dir.Name(), excludedDir) {
				continue includedDirectories
			}
		}

		repos = append(repos, dir.Name())
	}

	// publish all commands to run
	go func() {
		for _, repo := range repos {
			cmd := Command{
				WorkingDir: repo,
				Command:    "git",
				Args:       additionalArgs,
			}

			input <- cmd
		}
		close(input)
	}()

	// wait for all commands to finish
	failedCms := []CommandResult{}
	for i := 0; i < len(repos); i++ {
		result := <-output
		if !result.Success {
			failedCms = append(failedCms, result)
		}
	}

	if len(failedCms) > 0 {
		fmt.Printf("error: %d command(s) failed\n", len(failedCms))
		for _, result := range failedCms {
			fmt.Printf("command failed: %s\n", result.Command.String())
		}
		os.Exit(len(failedCms))
	}

	os.Exit(0)
}

func worker(id int, input <-chan Command, output chan<- CommandResult) {
	for cmd := range input {
		stdout := log.New(os.Stdout, fmt.Sprintf("[%s] ", filepath.Base(cmd.WorkingDir)), 0)
		stderr := log.New(os.Stderr, fmt.Sprintf("[%s] ", filepath.Base(cmd.WorkingDir)), 0)

		stdout.Printf("--> %s\n", cmd.String())

		stdoutWriter := logwriter.NewLogWriter(stdout)
		stderrWriter := logwriter.NewLogWriter(stderr)
		result := runCommand(stdoutWriter, stderrWriter, cmd)
		stdoutWriter.Flush()
		stderrWriter.Flush()

		if !result.Success {
			stderr.Printf("error: %s\n", result.Error.Error())
		}

		output <- result
	}
}

func runCommand(stdout io.Writer, stderr io.Writer, command Command) CommandResult {
	process := exec.Command(command.Command, command.Args...)
	process.Stdout = stdout
	process.Stderr = stderr
	if command.WorkingDir != "" {
		process.Dir = command.WorkingDir
	}

	if err := process.Start(); err != nil {
		return CommandResult{Error: err, Command: command}
	}

	timedOut := false
	timer := time.NewTimer(timeout)
	go func(timer *time.Timer, process *exec.Cmd) {
		for _ = range timer.C {
			process.Process.Signal(os.Kill)
			timedOut = true
			break
		}
	}(timer, process)

	if err := process.Wait(); err != nil {
		if timedOut {
			err = fmt.Errorf("process timed out: %s", command.String())
		} else if _, ok := err.(*exec.ExitError); ok {
			err = fmt.Errorf("exited with non-zero exit code")
		}
		return CommandResult{Error: err, Command: command}
	}

	return CommandResult{Success: true, Command: command}
}
