// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"runtime"
	"strings"
	tctesting "testing"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/tc"
	sshtesting "github.com/juju/utils/v3/ssh/testing"

	keymanagerserver "github.com/juju/juju/apiserver/facades/client/keymanager"
	keymanagertesting "github.com/juju/juju/apiserver/facades/client/keymanager/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/osenv"
	jujutesting "github.com/juju/juju/juju/testing"
)

type SSHKeysSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
}

func TestSSHKeysSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &SSHKeysSuite{})
}

func (s *SSHKeysSuite) assertHelpOutput(c *tc.C, cmd, args string) {
	if args != "" {
		args = " " + args
	}
	expected := fmt.Sprintf("Usage: juju %s [options]%s", cmd, args)
	out := badrun(c, 0, cmd, "--help")
	lines := strings.Split(out, "\n")
	c.Assert(lines[0], tc.Equals, expected)
}

func (s *SSHKeysSuite) TestHelpList(c *tc.C) {
	s.assertHelpOutput(c, "ssh-keys", "")
}

func (s *SSHKeysSuite) TestHelpAdd(c *tc.C) {
	s.assertHelpOutput(c, "add-ssh-key", "<ssh key> ...")
}

func (s *SSHKeysSuite) TestHelpRemove(c *tc.C) {
	s.assertHelpOutput(c, "remove-ssh-key", "<ssh key id> ...")
}

func (s *SSHKeysSuite) TestHelpImport(c *tc.C) {
	s.assertHelpOutput(c, "import-ssh-key", "<lp|gh>:<user identity> ...")
}

type keySuiteBase struct {
	jujutesting.JujuConnSuite
	coretesting.CmdBlockHelper
}

func (s *keySuiteBase) SetUpSuite(c *tc.C) {
	s.JujuConnSuite.SetUpSuite(c)
	s.PatchEnvironment(osenv.JujuModelEnvKey, "controller")
}

func (s *keySuiteBase) SetUpTest(c *tc.C) {
	if runtime.GOOS == "darwin" {
		c.Skip("Mongo failures on macOS")
	}
	s.JujuConnSuite.SetUpTest(c)
	s.CmdBlockHelper = coretesting.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, tc.NotNil)
	s.AddCleanup(func(*tc.C) { s.CmdBlockHelper.Close() })
}

func (s *keySuiteBase) setAuthorizedKeys(c *tc.C, keys ...string) {
	keyString := strings.Join(keys, "\n")
	err := s.Model.UpdateModelConfig(map[string]interface{}{"authorized-keys": keyString}, nil)
	c.Assert(err, tc.ErrorIsNil)
	envConfig, err := s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(envConfig.AuthorizedKeys(), tc.Equals, keyString)
}

func (s *keySuiteBase) assertEnvironKeys(c *tc.C, expected ...string) {
	envConfig, err := s.Model.ModelConfig()
	c.Assert(err, tc.ErrorIsNil)
	keys := envConfig.AuthorizedKeys()
	c.Assert(keys, tc.Equals, strings.Join(expected, "\n"))
}

type ListKeysSuite struct {
	keySuiteBase
}

func TestListKeysSuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &ListKeysSuite{})
}

func (s *ListKeysSuite) TestListKeys(c *tc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key + " another@host"
	s.setAuthorizedKeys(c, key1, key2)

	context, err := cmdtesting.RunCommand(c, NewListKeysCommand())
	c.Assert(err, tc.ErrorIsNil)
	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(output, tc.Matches, "Keys used in model: controller\n.*\\(user@host\\)\n.*\\(another@host\\)")
}

func (s *ListKeysSuite) TestListKeysWithModelUUID(c *tc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key + " another@host"
	s.setAuthorizedKeys(c, key1, key2)

	context, err := cmdtesting.RunCommand(c, NewListKeysCommand(), "-m", s.Model.UUID())
	c.Assert(err, tc.ErrorIsNil)
	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(output, tc.Matches,
		fmt.Sprintf("Keys used in model: %s\n.*\\(user@host\\)\n.*\\(another@host\\)", s.Model.UUID()))
}

func (s *ListKeysSuite) TestListFullKeys(c *tc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key + " another@host"
	s.setAuthorizedKeys(c, key1, key2)

	context, err := cmdtesting.RunCommand(c, NewListKeysCommand(), "--full")
	c.Assert(err, tc.ErrorIsNil)
	output := strings.TrimSpace(cmdtesting.Stdout(context))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(output, tc.Matches, "Keys used in model: controller\n.*user@host\n.*another@host")
}

func (s *ListKeysSuite) TestTooManyArgs(c *tc.C) {
	_, err := cmdtesting.RunCommand(c, NewListKeysCommand(), "foo")
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["foo"\]`)
}

type AddKeySuite struct {
	keySuiteBase
}

func TestAddKeySuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &AddKeySuite{})
}

func (s *AddKeySuite) TestAddKey(c *tc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	s.setAuthorizedKeys(c, key1)

	key2 := sshtesting.ValidKeyTwo.Key + " another@host"
	context, err := cmdtesting.RunCommand(c, NewAddKeysCommand(), key2, "invalid-key")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Matches, `cannot add key "invalid-key".*\n`)
	s.assertEnvironKeys(c, key1, key2)
}

func (s *AddKeySuite) TestBlockAddKey(c *tc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	s.setAuthorizedKeys(c, key1)

	key2 := sshtesting.ValidKeyTwo.Key + " another@host"
	// Block operation
	s.BlockAllChanges(c, "TestBlockAddKey")
	_, err := cmdtesting.RunCommand(c, NewAddKeysCommand(), key2, "invalid-key")
	coretesting.AssertOperationWasBlocked(c, err, ".*TestBlockAddKey.*")
}

type RemoveKeySuite struct {
	keySuiteBase
}

func TestRemoveKeySuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &RemoveKeySuite{})
}

func (s *RemoveKeySuite) TestRemoveKeys(c *tc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key + " another@host"
	s.setAuthorizedKeys(c, key1, key2)

	context, err := cmdtesting.RunCommand(c, NewRemoveKeysCommand(),
		sshtesting.ValidKeyTwo.Fingerprint, "invalid-key")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Matches, `cannot remove key id "invalid-key".*\n`)
	s.assertEnvironKeys(c, key1)
}

func (s *RemoveKeySuite) TestBlockRemoveKeys(c *tc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key + " another@host"
	s.setAuthorizedKeys(c, key1, key2)

	// Block operation
	s.BlockAllChanges(c, "TestBlockRemoveKeys")
	_, err := cmdtesting.RunCommand(c, NewRemoveKeysCommand(),
		sshtesting.ValidKeyTwo.Fingerprint, "invalid-key")
	coretesting.AssertOperationWasBlocked(c, err, ".*TestBlockRemoveKeys.*")
}

type ImportKeySuite struct {
	keySuiteBase
}

func TestImportKeySuite(t *tctesting.T) {
	coretesting.MgoTestPackage(t, &ImportKeySuite{})
}

func (s *ImportKeySuite) SetUpTest(c *tc.C) {
	s.keySuiteBase.SetUpTest(c)
	s.PatchValue(&keymanagerserver.RunSSHImportId, keymanagertesting.FakeImport)
}

func (s *ImportKeySuite) TestImportKeys(c *tc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	s.setAuthorizedKeys(c, key1)

	context, err := cmdtesting.RunCommand(c, NewImportKeysCommand(), "lp:validuser", "lp:invalid-key")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(context), tc.Matches, `cannot import key id "lp:invalid-key".*\n`)
	s.assertEnvironKeys(c, key1, sshtesting.ValidKeyThree.Key)
}

func (s *ImportKeySuite) TestBlockImportKeys(c *tc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	s.setAuthorizedKeys(c, key1)

	// Block operation
	s.BlockAllChanges(c, "TestBlockImportKeys")
	_, err := cmdtesting.RunCommand(c, NewImportKeysCommand(), "lp:validuser", "lp:invalid-key")
	coretesting.AssertOperationWasBlocked(c, err, ".*TestBlockImportKeys.*")
}
