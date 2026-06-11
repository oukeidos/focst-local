package prompt

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

type Confirmer struct {
	In            io.Reader
	Out           io.Writer
	IsInteractive func() bool
}

func DefaultConfirmer() Confirmer {
	return Confirmer{
		In:  os.Stdin,
		Out: os.Stdout,
		IsInteractive: func() bool {
			info, err := os.Stdin.Stat()
			if err != nil {
				return false
			}
			return (info.Mode() & os.ModeCharDevice) != 0
		},
	}
}

func (c Confirmer) ConfirmOverwrite(path string, force bool) (bool, error) {
	if force {
		return true, nil
	}
	if c.IsInteractive == nil || !c.IsInteractive() {
		return false, fmt.Errorf("non-interactive stdin: use -y to overwrite existing output")
	}
	if c.Out != nil {
		fmt.Fprintf(c.Out, "Warning: Output file %s already exists. Overwrite? (y/n): ", path)
	}
	reader := bufio.NewReader(c.In)
	response, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y", nil
}
