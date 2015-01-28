// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
)

const poolCmdDoc = `
"juju storage pool" is used to manage storage pool instances in
 Juju environment.
`

const poolCmdPurpose = "manage storage pools"

// NewPoolSuperCommand creates the storage pool super subcommand and
// registers the subcommands that it supports.
func NewPoolSuperCommand() cmd.Command {
	poolcmd := Command{
		SuperCommand: *jujucmd.NewSubSuperCommand(cmd.SuperCommandParams{
			Name:        "pool",
			Doc:         poolCmdDoc,
			UsagePrefix: "juju storage",
			Purpose:     poolCmdPurpose,
		})}
	poolcmd.Register(envcmd.Wrap(&PoolListCommand{}))
	return &poolcmd
}

// PoolCommandBase is a helper base structure for pool commands.
type PoolCommandBase struct {
	StorageCommandBase
}
