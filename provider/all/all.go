// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all

import (
	// Register all the available providers.
	_ "github.com/juju/juju/provider/azure"
	_ "github.com/juju/juju/provider/ec2"
	_ "github.com/juju/juju/provider/joyent"
	_ "github.com/juju/juju/provider/local"
	_ "github.com/juju/juju/provider/maas"
	_ "github.com/juju/juju/provider/manual"
	_ "github.com/juju/juju/provider/openstack"

	// Register provider storage.
	_ "github.com/juju/juju/provider/ec2/storage"
)
