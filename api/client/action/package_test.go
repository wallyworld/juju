// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"github.com/juju/juju/api/base"
)

func NewPrunerFromCaller(caller base.FacadeCaller) *Facade {
	return &Facade{
		facade: caller,
	}
}

func NewClientFromCaller(caller base.FacadeCaller) *Client {
	return &Client{
		facade: caller,
	}
}
