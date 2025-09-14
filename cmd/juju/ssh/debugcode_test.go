// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"encoding/base64"
	"regexp"
	"strings"
	tctesting "testing"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/tc"
	goyaml "gopkg.in/yaml.v2"

	coretesting "github.com/juju/juju/internal/testing"
)

func TestDebugCodeSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &DebugCodeSuite{})
}

type DebugCodeSuite struct {
	SSHMachineSuite
}

func (s *DebugCodeSuite) TestArgFormatting(c *tc.C) {
	s.setupModel(c)
	s.setHostChecker(validAddresses("0.public"))
	ctx, err := cmdtesting.RunCommand(c, NewDebugCodeCommand(s.hostChecker, baseTestingRetryStrategy, baseTestingRetryStrategy),
		"--at=foo,bar", "mysql/0", "install", "start")
	c.Assert(err, tc.ErrorIsNil)
	base64Regex := regexp.MustCompile("echo ([A-Za-z0-9+/]+=*) \\| base64")
	c.Check(err, tc.ErrorIsNil)
	rawContent := base64Regex.FindString(cmdtesting.Stdout(ctx))
	c.Check(rawContent, tc.Not(tc.Equals), "")
	// Strip off the "echo " and " | base64"
	prefix := "echo "
	suffix := " | base64"
	c.Check(strings.HasPrefix(rawContent, prefix), tc.IsTrue)
	c.Check(strings.HasSuffix(rawContent, suffix), tc.IsTrue)
	b64content := rawContent[len(prefix) : len(rawContent)-len(suffix)]
	scriptContent, err := base64.StdEncoding.DecodeString(b64content)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(scriptContent), tc.Not(tc.Equals), "")
	// Inside the script is another base64 encoded string telling us the debug-hook args
	debugArgsRegex := regexp.MustCompile(`echo "([A-Z-a-z0-9+/]+=*)" \| base64.*-debug-hooks`)
	debugArgsCommand := debugArgsRegex.FindString(string(scriptContent))
	debugArgsB64 := debugArgsCommand[len(`echo "`):strings.Index(debugArgsCommand, `" | base64`)]
	yamlContent, err := base64.StdEncoding.DecodeString(debugArgsB64)
	c.Assert(err, tc.ErrorIsNil)
	var args map[string]interface{}
	err = goyaml.Unmarshal(yamlContent, &args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(args, tc.DeepEquals, map[string]interface{}{
		"hooks":    []interface{}{"install", "start"},
		"debug-at": "foo,bar",
	})
}
