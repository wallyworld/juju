// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"bytes"
	"os"
	"path/filepath"
	tctesting "testing"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type K8sSpecSetSuite struct {
	ContextSuite
}

func TestK8sSpecSetSuite(t *tctesting.T) {
	tc.Run(t, &K8sSpecSetSuite{})
}

var (
	podSpecYaml = `
podSpec:
  foo: bar
`[1:]

	k8sResourcesYaml = `
kubernetesResources:
  pod:
    restartPolicy: OnFailure
    activeDeadlineSeconds: 10
    terminationGracePeriodSeconds: 20
    securityContext:
      runAsNonRoot: true
      supplementalGroups: [1,2]
    priority: 30
    readinessGates:
      - conditionType: PodScheduled
    dnsPolicy: ClusterFirstWithHostNet
  secrets:
    - name: build-robot-secret
      annotations:
          kubernetes.io/service-account.name: build-robot
      type: kubernetes.io/service-account-token
      stringData:
          config.yaml: |-
              apiUrl: "https://my.api.com/api/v1"
              username: fred
              password: shhhh
`[1:]
)

var k8sSpecSetInitTests = []struct {
	args []string
	err  string
}{
	{[]string{"--file", "file", "extra"}, `unrecognized args: \["extra"\]`},
}

func (s *K8sSpecSetSuite) TestK8sSpecSetInit(c *tc.C) {
	for i, t := range k8sSpecSetInitTests {
		c.Logf("test %d: %#v", i, t.args)
		hctx := s.GetHookContext(c, -1, "")
		com, err := jujuc.NewCommand(hctx, "k8s-spec-set")
		c.Assert(err, tc.ErrorIsNil)
		cmdtesting.TestInit(c, jujuc.NewJujucCommandWrappedForTest(com), t.args, t.err)
	}
}

func (s *K8sSpecSetSuite) TestK8sSpecSetNoData(c *tc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "k8s-spec-set")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)

	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, nil)
	c.Check(code, tc.Equals, 1)
	c.Assert(bufferString(
		ctx.Stderr), tc.Matches,
		".*no k8s spec specified: pipe k8s spec to command, or specify a file with --file\n")
	c.Assert(bufferString(ctx.Stdout), tc.Equals, "")
}

func (s *K8sSpecSetSuite) TestK8sSpecSet(c *tc.C) {
	s.assertK8sSpecSet(c, "specfile.yaml", false)
}

func (s *K8sSpecSetSuite) TestK8sSpecSetStdIn(c *tc.C) {
	s.assertK8sSpecSet(c, "-", false)
}

func (s *K8sSpecSetSuite) TestK8sSpecSetWithK8sResource(c *tc.C) {
	s.assertK8sSpecSet(c, "specfile.yaml", true)
}

func (s *K8sSpecSetSuite) TestK8sSpecSetStdInWithK8sResource(c *tc.C) {
	s.assertK8sSpecSet(c, "-", true)
}

func (s *K8sSpecSetSuite) assertK8sSpecSet(c *tc.C, filename string, withK8sResource bool) {
	hctx := s.GetHookContext(c, -1, "")
	com, args, ctx := s.initCommand(c, hctx, podSpecYaml, filename, withK8sResource)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, args)
	c.Check(code, tc.Equals, 0)
	c.Assert(bufferString(ctx.Stderr), tc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), tc.Equals, "")
	expectedSpecYaml := podSpecYaml
	if withK8sResource {
		expectedSpecYaml += k8sResourcesYaml
	}
	c.Assert(hctx.info.K8sSpec, tc.Equals, expectedSpecYaml)
}

func (s *K8sSpecSetSuite) initCommand(
	c *tc.C, hctx jujuc.Context, yaml string, filename string, withK8sResource bool,
) (cmd.Command, []string, *cmd.Context) {
	com, err := jujuc.NewCommand(hctx, "k8s-spec-set")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)

	var args []string
	if filename == "-" {
		ctx.Stdin = bytes.NewBufferString(yaml)
	} else if filename != "" {
		filename = filepath.Join(c.MkDir(), filename)
		err := os.WriteFile(filename, []byte(yaml), 0644)
		c.Assert(err, tc.ErrorIsNil)
		args = append(args, "--file", filename)
	}
	if withK8sResource {
		k8sResourceFileName := "k8sresources.yaml"
		k8sResourceFileName = filepath.Join(c.MkDir(), k8sResourceFileName)
		err := os.WriteFile(k8sResourceFileName, []byte(k8sResourcesYaml), 0644)
		c.Assert(err, tc.ErrorIsNil)
		args = append(args, "--k8s-resources", k8sResourceFileName)
	}
	return jujuc.NewJujucCommandWrappedForTest(com), args, ctx
}
