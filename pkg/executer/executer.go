package executer

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"syscall"
)

type Executer interface {
	CommandContext(ctx context.Context, command string, args ...string) *exec.Cmd
	Execute(command string, args ...string) (stdout string, stderr string, exitCode int)
	ExecuteWithContext(ctx context.Context, command string, args ...string) (stdout string, stderr string, exitCode int)
	ExecuteWithContextFromDir(ctx context.Context, workingDir string, command string, args []string, env ...string) (stdout string, stderr string, exitCode int)
	TempFile(dir, pattern string) (f *os.File, err error)
	LookPath(file string) (string, error)
}

type CommonExecuter struct{}

func (e *CommonExecuter) TempFile(dir, pattern string) (f *os.File, err error) {
	return os.CreateTemp(dir, pattern)
}

func (e *CommonExecuter) Execute(command string, args ...string) (stdout string, stderr string, exitCode int) {
	cmd := exec.Command(command, args...)
	return e.execute(cmd)
}

func (e *CommonExecuter) CommandContext(ctx context.Context, command string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
	return cmd
}

func (e *CommonExecuter) ExecuteWithContext(ctx context.Context, command string, args ...string) (stdout string, stderr string, exitCode int) {
	cmd := exec.CommandContext(ctx, command, args...)
	return e.execute(cmd)
}

func (e *CommonExecuter) ExecuteWithContextFromDir(ctx context.Context, workingDir string, command string, args []string, env ...string) (stdout string, stderr string, exitCode int) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = workingDir
	if len(env) > 0 {
		cmd.Env = env
	}
	return e.execute(cmd)
}

func (e *CommonExecuter) execute(cmd *exec.Cmd) (stdout string, stderr string, exitCode int) {
	var stdoutBytes, stderrBytes bytes.Buffer
	cmd.Stdout = &stdoutBytes
	cmd.Stderr = &stderrBytes
	// Set Pdeathsig to SIGTERM to kill the process and its children when the parent process is killed.
	// This should prevent orphaned processes and allow for the subprocess to gracefully terminate.
	// ref. https://github.com/golang/go/blob/release-branch.go1.21/src/syscall/exec_linux.go#L91
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
	err := cmd.Run()
	if errr, ok := err.(*exec.ExitError); ok {
		state, ok := errr.ProcessState.Sys().(syscall.WaitStatus)
		if ok && state.Signal() == syscall.SIGKILL {
			return stdoutBytes.String(), stderrBytes.String(), 137 // 128 + 9 (SIGKILL)
		}
	}
	return stdoutBytes.String(), getErrorStr(err, &stderrBytes), getExitCode(err)
}

func (e *CommonExecuter) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func getExitCode(err error) int {
	if err == nil {
		return 0
	}
	switch value := err.(type) {
	case *exec.ExitError:
		return value.ExitCode()
	default:
		return -1
	}
}

func getErrorStr(err error, stderr *bytes.Buffer) string {
	b := stderr.Bytes()
	if len(b) > 0 {
		return string(b)
	} else if err != nil {
		return err.Error()
	}
	return ""
}
