// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
	tctesting "testing"

	"github.com/juju/tc"

	provider "github.com/juju/juju/internal/provider/kubernetes"
	"github.com/juju/juju/internal/testing"
)

type templateSuite struct {
	testing.BaseSuite
}

func TestTemplateSuite(t *tctesting.T) {
	tc.Run(t, &templateSuite{})
}

func (t *templateSuite) TestToYaml(c *tc.C) {
	in := struct {
		Command []string `yaml:"command,omitempty"`
	}{
		Command: []string{"sh", "-c", `
set -ex
echo "do some stuff here for gitlab container"
`[1:]},
	}
	out, err := provider.ToYaml(in)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.DeepEquals, `
command:
- sh
- -c
- |
  set -ex
  echo "do some stuff here for gitlab container"
`[1:])
}

func (t *templateSuite) TestIndent(c *tc.C) {
	out := provider.Indent(6, `
line 1
line 2
line 3`[1:])
	c.Assert(out, tc.DeepEquals, `
      line 1
      line 2
      line 3
`[1:])

	out = provider.Indent(8, `
line 1
line 2
line 3`[1:])
	c.Assert(out, tc.DeepEquals, `
        line 1
        line 2
        line 3
`[1:])
}
