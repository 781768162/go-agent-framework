package tools

import (
	"context"
	"os/exec"
)

func ExecBash(ctx context.Context, dir, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	return string(output), err
}
