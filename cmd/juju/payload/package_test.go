// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payload

import (
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

func NewListCommandForTest(newClient func() (ListAPI, error)) *ListCommand {
	cmd := &ListCommand{newAPIClient: newClient}
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	return cmd
}
