// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/tc"

	"github.com/juju/juju/api/controller/controller"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

// This suite only exists because no user facing calls exercise
// the WatchModelSummaries or WatchAllModelSummaries.
// The primary caller is the JAAS dashboard which uses the javascript
// library. It is expected that JIMM will call these methods using
// the Go API.

type ControllerSuite struct {
	testing.JujuConnSuite
	client *controller.Client
}

func (s *ControllerSuite) SetUpTest(c *tc.C) {
	s.JujuConnSuite.SetUpTest(c)

	userConn := s.OpenControllerAPI(c)
	s.client = controller.NewClient(userConn)
	s.AddCleanup(func(*tc.C) { s.client.Close() })
}

func (s *ControllerSuite) TestWatchModelSummaries(c *tc.C) {

	watcher, err := s.client.WatchModelSummaries()
	c.Assert(err, tc.ErrorIsNil)
	defer func() {
		c.Check(watcher.Stop(), tc.ErrorIsNil)
	}()

	summaries, err := watcher.Next()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(summaries, tc.DeepEquals, []params.ModelAbstract{
		{
			UUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			Name:       "controller",
			Admins:     []string{"admin"},
			Cloud:      "dummy",
			Region:     "dummy-region",
			Credential: "dummy/admin/cred",
			Status:     "green",
		},
	})
}

func (s *ControllerSuite) TestWatchAllModelSummaries(c *tc.C) {

	watcher, err := s.client.WatchAllModelSummaries()
	c.Assert(err, tc.ErrorIsNil)
	defer func() {
		c.Check(watcher.Stop(), tc.ErrorIsNil)
	}()

	summaries, err := watcher.Next()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(summaries, tc.DeepEquals, []params.ModelAbstract{
		{
			UUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
			Name:       "controller",
			Admins:     []string{"admin"},
			Cloud:      "dummy",
			Region:     "dummy-region",
			Credential: "dummy/admin/cred",
			Status:     "green",
		},
	})
}
