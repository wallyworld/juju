// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/api"
	apiblock "github.com/juju/juju/api/client/block"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/rpc/params"
)

var logger = internallogger.GetLogger("juju.cmd.juju.block")

const (
	cmdAll          = "all"
	cmdDestroyModel = "destroy-model"
	cmdRemoveObject = "remove-object"

	apiAll          = "BlockChange"
	apiDestroyModel = "BlockDestroy"
	apiRemoveObject = "BlockRemove"
)

var (
	toAPIValue = map[string]string{
		cmdAll:          apiAll,
		cmdDestroyModel: apiDestroyModel,
		cmdRemoveObject: apiRemoveObject,
	}

	toCmdValue = map[string]string{
		apiAll:          cmdAll,
		apiDestroyModel: cmdDestroyModel,
		apiRemoveObject: cmdRemoveObject,
	}

	validTargets = cmdAll + ", " + cmdDestroyModel + ", " + cmdRemoveObject
)

func operationFromType(blockType string) string {
	value, ok := toCmdValue[blockType]
	if !ok {
		value = "<unknown>"
	}
	return value
}

type newAPIRoot interface {
	NewAPIRoot() (api.Connection, error)
}

// getBlockAPI returns a block api for block manipulation.
func getBlockAPI(c newAPIRoot) (*apiblock.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, err
	}
	return apiblock.NewClient(root), nil
}

// Block describes block type
type Block int8

const (
	// BlockDestroy describes the block that
	// blocks destroy- commands
	BlockDestroy Block = iota

	// BlockRemove describes the block that
	// blocks remove- commands
	BlockRemove

	// BlockChange describes the block that
	// blocks change commands
	BlockChange
)

var blockedMessages = map[Block]string{
	BlockDestroy: destroyMsg,
	BlockRemove:  removeMsg,
	BlockChange:  changeMsg,
}

// ProcessBlockedError ensures that correct and user-friendly message is
// displayed to the user based on the block type.
func ProcessBlockedError(err error, block Block) error {
	if err == nil {
		return nil
	}
	if params.IsCodeOperationBlocked(err) {
		msg := fmt.Sprintf("%v\n%v", err, blockedMessages[block])
		logger.Infof(msg)
		return errors.Errorf(msg)
	}
	return err
}

var removeMsg = `
All operations that remove machines, applications, units or
relations have been disabled for the current model.
To enable removal, run

    juju enable-command remove-object

`
var destroyMsg = `
destroy-model operation has been disabled for the current model.
To enable the command run

    juju enable-command destroy-model

`
var changeMsg = `
All operations that change model have been disabled for the current model.
To enable changes, run

    juju enable-command all

`
