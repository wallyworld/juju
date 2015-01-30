// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
)

const volumeCmdDoc = `
"juju storage volume" is used to manage storage volumes in
 Juju environment.
`

const volumeCmdPurpose = "manage storage volumes"

// NewVolumeSuperCommand creates the storage volume super subcommand and
// registers the subcommands that it supports.
func NewVolumeSuperCommand() cmd.Command {
	poolcmd := Command{
		SuperCommand: *jujucmd.NewSubSuperCommand(cmd.SuperCommandParams{
			Name:        "volume",
			Doc:         volumeCmdDoc,
			UsagePrefix: "juju storage",
			Purpose:     volumeCmdPurpose,
		})}
	poolcmd.Register(envcmd.Wrap(&VolumeListCommand{}))
	return &poolcmd
}

// VolumeCommandBase is a helper base structure for volume commands.
type VolumeCommandBase struct {
	StorageCommandBase
}
