package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

type runProcessOpts struct {
	Name      string
	Args      []string
	Stdout    io.Writer
	Stdin     io.Reader
	NoConsole bool
}

func runProcess(opts runProcessOpts) error {
	if !opts.NoConsole {
		fmt.Fprintf(os.Stderr, "Executing: %s %s\n", opts.Name, strings.Join(opts.Args, " "))
	}

	cmd := exec.Command(opts.Name, opts.Args...)

	if opts.NoConsole {
		cmd.Stdout = opts.Stdout
	} else if opts.Stdout == nil {
		// Redirect all output to stderr
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
	} else {
		// Redirect all output to stderr too in addition to what the user requested
		cmd.Stdout = io.MultiWriter(os.Stderr, opts.Stdout)
		cmd.Stderr = os.Stderr
	}

	if opts.Stdin != nil {
		cmd.Stdin = opts.Stdin
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	return cmd.Run()
}

func runShellScript(script string, stdout io.Writer, noConsole bool) error {
	return runProcess(runProcessOpts{
		Name:      "/bin/bash",
		Args:      []string{"-c", script},
		Stdout:    stdout,
		NoConsole: noConsole,
	})
}
