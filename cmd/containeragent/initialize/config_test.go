// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initialize_test

import (
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/containeragent/initialize"
)

func (s *initCommandSuit) TestConfigFromEnv(c *tc.C) {
	cfg := initialize.ConfigFromEnv{}
	c.Assert(cfg.Tag(), tc.DeepEquals, names.NewApplicationTag("gitlab"))
	c.Assert(cfg.CACert(), tc.DeepEquals, `ca-cert`)

	addrs, err := cfg.APIAddresses()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addrs, tc.DeepEquals, []string{
		`1.1.1.1`, `2.2.2.2`,
	})

	apiInfo, ok := cfg.APIInfo()
	c.Assert(ok, tc.IsTrue)
	c.Assert(apiInfo, tc.DeepEquals, &api.Info{
		Addrs: []string{
			`1.1.1.1`, `2.2.2.2`,
		},
		CACert:   `ca-cert`,
		ModelTag: names.NewModelTag("model1"),
		Tag:      names.NewApplicationTag("gitlab"),
		Password: `passwd`,
	})

}

func (s *initCommandSuit) TestDefaultIdentityOnK8S(c *tc.C) {
	ID, err := initialize.Identity()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ID.PodName, tc.DeepEquals, `gitlab-0`)
	c.Assert(ID.PodUUID, tc.DeepEquals, `gitlab-uuid`)
}
