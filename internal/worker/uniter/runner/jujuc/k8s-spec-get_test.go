// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	tctesting "testing"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type K8sSpecGetSuite struct {
	ContextSuite
}

func TestK8sSpecGetSuite(t *tctesting.T) {
	tc.Run(t, &K8sSpecGetSuite{})
}

var k8sSpecGetInitTests = []struct {
	args []string
	err  string
}{
	{[]string{"extra"}, `unrecognized args: \["extra"\]`},
}

func (s *K8sSpecGetSuite) TestK8sSpecGetInit(c *tc.C) {
	for i, t := range k8sSpecGetInitTests {
		c.Logf("test %d: %#v", i, t.args)
		hctx := s.GetHookContext(c, -1, "")
		com, err := jujuc.NewCommand(hctx, "k8s-spec-get")
		c.Assert(err, tc.ErrorIsNil)
		cmdtesting.TestInit(c, jujuc.NewJujucCommandWrappedForTest(com), t.args, t.err)
	}
}

func (s *K8sSpecGetSuite) TestK8sSpecSet(c *tc.C) {
	hctx := s.GetHookContext(c, -1, "")
	hctx.info.K8sSpec = "k8sspec"
	com, err := jujuc.NewCommand(hctx, "k8s-spec-get")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)

	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, nil)
	c.Check(code, tc.Equals, 0)
	c.Assert(bufferString(ctx.Stderr), tc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), tc.Equals, "k8sspec")
}
