// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"os/exec"
	"strings"

	"github.com/juju/loggo"

	"github.com/juju/juju/storage"
)

var logger = loggo.GetLogger("juju.storage.provider")

func init() {

	storage.RegisterProvider(LoopProviderType, &loopProvider{RunCmdFn()})

	storage.RegisterDefaultPool("local", storage.StorageKindBlock, LoopPool)
}

// RunCmdFn returns a function which will run a command and return the
// output and any errors.
func RunCmdFn() RunCommandFn {
	return func(cmd string, args ...string) (string, error) {
		logger.Debugf("running: %s %s", cmd, strings.Join(args, " "))
		c := exec.Command(cmd, args...)
		output, err := c.CombinedOutput()
		return string(output), err
	}
}
