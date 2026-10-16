//go:build !windows

// Package processgroup manages the complete lifetime of a host command.
package processgroup

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func Start(cmd *exec.Cmd) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	return cmd.Start()
}
func Stop(pid int, force bool) error {
	if pid <= 0 {
		return nil
	}
	signal := syscall.SIGTERM
	if force {
		signal = syscall.SIGKILL
	}
	if err := syscall.Kill(-pid, signal); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}
	if !force {
		return nil
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		alive, err := Alive(pid)
		if err != nil {
			return err
		}
		if !alive {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("process group %d has not exited", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Exited children can remain as zombies until their parent reaps them. They no
// longer execute commands or access configuration and do not block cleanup.
func Alive(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	if err := syscall.Kill(-pid, 0); errors.Is(err, syscall.ESRCH) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	data, err := exec.Command("ps", "-axo", "pgid=,stat=").Output()
	if err != nil {
		return false, fmt.Errorf("verify process group: %w", err)
	}
	group := strconv.Itoa(pid)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == group && !strings.HasPrefix(fields[1], "Z") {
			return true, nil
		}
	}
	return false, nil
}
