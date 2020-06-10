package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"sync"
	"syscall"

	"github.com/pkg/term/termios"
)

type ptyPipe struct {
	stdoutPty, stdoutTty *os.File
	stderrPty, stderrTty *os.File
}

type multiOutput struct {
	maxNameLength int
	mutex         sync.Mutex
	pipes         map[*process]*ptyPipe
	printProcName bool
}

func (m *multiOutput) openPipe(proc *process) (pipe *ptyPipe) {
	var err error

	pipe = m.pipes[proc]

	pipe.stdoutPty, pipe.stdoutTty, err = termios.Pty()
	fatalOnErr(err)

	pipe.stderrPty, pipe.stderrTty, err = termios.Pty()
	fatalOnErr(err)

	proc.Stdout = pipe.stdoutTty
	proc.Stderr = pipe.stderrTty
	proc.Stdin = pipe.stdoutTty
	proc.SysProcAttr = &syscall.SysProcAttr{Setctty: true, Setsid: true}

	return
}

func (m *multiOutput) Connect(proc *process) {
	if len(proc.Name) > m.maxNameLength {
		m.maxNameLength = len(proc.Name)
	}

	if m.pipes == nil {
		m.pipes = make(map[*process]*ptyPipe)
	}

	m.pipes[proc] = &ptyPipe{}
}

func (m *multiOutput) PipeOutput(proc *process) {
	pipe := m.openPipe(proc)

	go func(proc *process, pipe *ptyPipe) {
		scanner := bufio.NewScanner(pipe.stdoutPty)

		for scanner.Scan() {
			m.WriteLine(proc, "stdout", scanner.Bytes())
		}
	}(proc, pipe)

	go func(proc *process, pipe *ptyPipe) {
		scanner := bufio.NewScanner(pipe.stderrPty)

		for scanner.Scan() {
			m.WriteLine(proc, "stderr", scanner.Bytes())
		}
	}(proc, pipe)
}

func (m *multiOutput) ClosePipe(proc *process) {
	if pipe := m.pipes[proc]; pipe != nil {
		pipe.stdoutPty.Close()
		pipe.stdoutTty.Close()
		pipe.stderrPty.Close()
		pipe.stderrTty.Close()
	}
}

func (m *multiOutput) WriteLine(proc *process, stream string, p []byte) {
	var buf bytes.Buffer

	if m.printProcName {
		color := fmt.Sprintf("\033[1;38;5;%vm", proc.Color)

		buf.WriteString(color)
		buf.WriteString(proc.Name)

		for buf.Len()-len(color) < m.maxNameLength {
			buf.WriteByte(' ')
		}

		buf.WriteString("\033[0m | ")

		// now for the stream name - stdout/stderr
		streamColor := 64
		if stream == "stderr" {
			streamColor = 63
		} else if stream == "error" {
			streamColor = 88
		} else if stream == "status" {
			streamColor = 80
		}
		color = fmt.Sprintf("\033[1;38;5;%vm", streamColor)

		buf.WriteString(color)
		buf.WriteString(stream)
		if stream == "error" {
			buf.WriteByte(' ')
		}

		buf.WriteString("\033[0m | ")
	}

	buf.Write(p)
	buf.WriteByte('\n')

	m.mutex.Lock()
	defer m.mutex.Unlock()

	buf.WriteTo(os.Stdout)
}

func (m *multiOutput) WriteErr(proc *process, err error) {
	m.WriteLine(proc, "error", []byte(
		fmt.Sprintf("\033[0;31m%v\033[0m", err),
	))
}
