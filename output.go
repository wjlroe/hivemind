package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"syscall"

	"github.com/pkg/term/termios"
)

type ptyPipe struct {
	pty, tty *os.File
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

	pipe.pty, pipe.tty, err = termios.Pty()
	fatalOnErr(err)

	proc.Stdout = pipe.tty
	proc.Stderr = pipe.tty
	proc.Stdin = pipe.tty
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
	var prefixBuf bytes.Buffer
	for prefixBuf.Len() < m.maxNameLength {
		prefixBuf.WriteByte(' ')
	}
	prefixBuf.WriteString("   ")
	prefix := prefixBuf.String()

	go func(proc *process, pipe *ptyPipe) {
		scanner := bufio.NewScanner(pipe.pty)

		for scanner.Scan() {
			output := scanner.Bytes()
			var outputJSON interface{}
			if err := json.Unmarshal(output, &outputJSON); err == nil {
				if indentedJSON, err := json.MarshalIndent(&outputJSON, prefix, "  "); err == nil {
					m.WriteLine(proc, indentedJSON)
					continue
				}
			}
			m.WriteLine(proc, output)
		}
	}(proc, pipe)
}

func (m *multiOutput) ClosePipe(proc *process) {
	if pipe := m.pipes[proc]; pipe != nil {
		pipe.pty.Close()
		pipe.tty.Close()
	}
}

func (m *multiOutput) WriteLine(proc *process, p []byte) {
	var buf bytes.Buffer

	if m.printProcName {
		color := fmt.Sprintf("\033[1;38;5;%vm", proc.Color)

		buf.WriteString(color)
		buf.WriteString(proc.Name)

		for buf.Len()-len(color) < m.maxNameLength {
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
	m.WriteLine(proc, []byte(
		fmt.Sprintf("\033[0;31m%v\033[0m", err),
	))
}
