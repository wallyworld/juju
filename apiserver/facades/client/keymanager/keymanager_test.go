// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager_test

import (
	"fmt"
	"strings"
	tctesting "testing"

	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3/ssh"
	sshtesting "github.com/juju/utils/v3/ssh/testing"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/client/keymanager"
	"github.com/juju/juju/apiserver/facades/client/keymanager/mocks"
	keymanagertesting "github.com/juju/juju/apiserver/facades/client/keymanager/testing"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type keyManagerSuite struct {
	testhelpers.CleanupSuite

	model        *mocks.MockModel
	blockChecker *mocks.MockBlockChecker
	apiUser      names.UserTag
	api          *keymanager.KeyManagerAPI

	authorizer apiservertesting.FakeAuthorizer
}

func TestKeyManagerSuite(t *tctesting.T) {
	tc.Run(t, &keyManagerSuite{})
}

func (s *keyManagerSuite) SetUpTest(c *tc.C) {
	s.PatchValue(&keymanager.RunSSHImportId, keymanagertesting.FakeImport)
	s.apiUser = names.NewUserTag("admin")
}

func (s *keyManagerSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.model = mocks.NewMockModel(ctrl)
	s.model.EXPECT().ModelTag().Return(coretesting.ModelTag).AnyTimes()
	s.blockChecker = mocks.NewMockBlockChecker(ctrl)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.apiUser,
	}

	s.api = keymanager.NewKeyManagerAPI(s.model, s.authorizer, s.blockChecker, coretesting.ControllerTag)

	return ctrl
}

func (s *keyManagerSuite) setAuthorizedKeys(c *tc.C, keys ...string) {
	joined := strings.Join(keys, "\n")
	attrs := coretesting.FakeConfig().Merge(coretesting.Attrs{
		"authorized-keys": joined,
	})
	s.model.EXPECT().ModelConfig().Return(config.New(config.UseDefaults, attrs)).AnyTimes()
}

func (s *keyManagerSuite) TestListKeys(c *tc.C) {
	defer s.setup(c).Finish()

	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	s.setAuthorizedKeys(c, key1, key2, "bad key")

	args := params.ListSSHKeys{
		Entities: params.Entities{Entities: []params.Entity{
			{Tag: names.NewUserTag("admin").String()},
			{Tag: "invalid"},
		}},
		Mode: ssh.FullKeys,
	}
	results, err := s.api.ListKeys(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{Result: []string{key1, key2, "Invalid key: bad key"}},
			{Result: []string{key1, key2, "Invalid key: bad key"}},
		},
	})
}

func (s *keyManagerSuite) TestListKeysHidesJujuInternal(c *tc.C) {
	defer s.setup(c).Finish()

	key1 := sshtesting.ValidKeyOne.Key + " juju-client-key"
	key2 := sshtesting.ValidKeyTwo.Key + " " + config.JujuSystemKey
	s.setAuthorizedKeys(c, key1, key2)

	args := params.ListSSHKeys{
		Entities: params.Entities{Entities: []params.Entity{
			{Tag: names.NewUserTag("admin").String()},
		}},
		Mode: ssh.FullKeys,
	}
	results, err := s.api.ListKeys(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{Result: nil},
		},
	})
}

func (s *keyManagerSuite) TestListJujuSystemKey(c *tc.C) {
	defer s.setup(c).Finish()

	key1 := sshtesting.ValidKeyOne.Key
	s.setAuthorizedKeys(c, key1)

	args := params.ListSSHKeys{
		Entities: params.Entities{Entities: []params.Entity{
			{Tag: config.JujuSystemKey},
		}},
		Mode: ssh.FullKeys,
	}
	results, err := s.api.ListKeys(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, "permission denied")
}

func (s *keyManagerSuite) assertAddKeys(c *tc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	s.setAuthorizedKeys(c, key1, key2, "bad key")

	newKey := sshtesting.ValidKeyThree.Key + " newuser@host"
	newLineKey := sshtesting.ValidKeyFour.Key + " line1\nline2"

	newAttrs := map[string]interface{}{
		config.AuthorizedKeysKey: strings.Join([]string{key1, key2, "bad key", newKey}, "\n"),
	}
	s.model.EXPECT().UpdateModelConfig(newAttrs, nil)

	args := params.ModifyUserSSHKeys{
		User: names.NewUserTag("admin").Name(),
		Keys: []string{key2, newKey, newKey, "invalid-key", newLineKey},
	}
	results, err := s.api.AddKeys(args)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ServerError(fmt.Sprintf("duplicate ssh key: %s", key2))},
			{Error: nil},
			{Error: apiservertesting.ServerError(fmt.Sprintf("duplicate ssh key: %s", newKey))},
			{Error: apiservertesting.ServerError("invalid ssh key: invalid-key")},
			{Error: apiservertesting.ServerError(fmt.Sprintf("invalid ssh key: %s", newLineKey))},
		},
	})
}

func (s *keyManagerSuite) TestAddKeys(c *tc.C) {
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().ChangeAllowed().Return(nil)
	s.assertAddKeys(c)
}

func (s *keyManagerSuite) TestAddKeysSuperUser(c *tc.C) {
	s.apiUser = names.NewUserTag("superuser-fred")
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().ChangeAllowed().Return(nil)
	s.assertAddKeys(c)
}

func (s *keyManagerSuite) TestAddKeysModelAdmin(c *tc.C) {
	s.apiUser = names.NewUserTag("admin" + coretesting.ModelTag.String())
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().ChangeAllowed().Return(nil)
	s.assertAddKeys(c)
}

func (s *keyManagerSuite) TestAddKeysNonAuthorised(c *tc.C) {
	s.apiUser = names.NewUserTag("fred")
	defer s.setup(c).Finish()

	_, err := s.api.AddKeys(params.ModifyUserSSHKeys{})
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(params.ErrCode(err), tc.Equals, params.CodeUnauthorized)
}

func (s *keyManagerSuite) TestBlockAddKeys(c *tc.C) {
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().ChangeAllowed().Return(errors.OperationBlockedError("TestAddKeys"))

	_, err := s.api.AddKeys(params.ModifyUserSSHKeys{})

	c.Assert(params.IsCodeOperationBlocked(err), tc.IsTrue)
}

func (s *keyManagerSuite) TestAddJujuSystemKey(c *tc.C) {
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().ChangeAllowed().Return(nil)
	s.setAuthorizedKeys(c, sshtesting.ValidKeyOne.Key)

	newAttrs := map[string]interface{}{
		config.AuthorizedKeysKey: sshtesting.ValidKeyOne.Key,
	}
	s.model.EXPECT().UpdateModelConfig(newAttrs, nil)

	newKey := sshtesting.ValidKeyThree.Key + " " + config.JujuSystemKey
	args := params.ModifyUserSSHKeys{
		User: names.NewUserTag("admin").Name(),
		Keys: []string{newKey},
	}
	results, err := s.api.AddKeys(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ServerError("may not add key with comment juju-system-key: " + newKey)},
		},
	})
}

func (s *keyManagerSuite) assertDeleteKeys(c *tc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	s.setAuthorizedKeys(c, key1, key2, "bad key 1", "bad key 2")

	newAttrs := map[string]interface{}{
		config.AuthorizedKeysKey: strings.Join([]string{key1, "bad key 1"}, "\n"),
	}
	s.model.EXPECT().UpdateModelConfig(newAttrs, nil)

	args := params.ModifyUserSSHKeys{
		User: names.NewUserTag("admin").String(),
		Keys: []string{sshtesting.ValidKeyTwo.Fingerprint, sshtesting.ValidKeyThree.Fingerprint, "invalid-key", "bad key 2"},
	}
	results, err := s.api.DeleteKeys(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: apiservertesting.ServerError("key not found: " + sshtesting.ValidKeyThree.Fingerprint)},
			{Error: apiservertesting.ServerError("key not found: invalid-key")},
			{Error: nil},
		},
	})
}

func (s *keyManagerSuite) TestDeleteKeys(c *tc.C) {
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().RemoveAllowed().Return(nil)
	s.assertDeleteKeys(c)
}

func (s *keyManagerSuite) TestDeleteKeysSuperUser(c *tc.C) {
	s.apiUser = names.NewUserTag("superuser-fred")
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().RemoveAllowed().Return(nil)
	s.assertDeleteKeys(c)
}

func (s *keyManagerSuite) TestDeleteKeysModelAdmin(c *tc.C) {
	s.apiUser = names.NewUserTag("admin" + coretesting.ModelTag.String())
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().RemoveAllowed().Return(nil)
	s.assertDeleteKeys(c)
}

func (s *keyManagerSuite) TestDeleteKeysNonAuthorised(c *tc.C) {
	s.apiUser = names.NewUserTag("fred")
	defer s.setup(c).Finish()

	_, err := s.api.DeleteKeys(params.ModifyUserSSHKeys{})
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(params.ErrCode(err), tc.Equals, params.CodeUnauthorized)
}

func (s *keyManagerSuite) TestBlockDeleteKeys(c *tc.C) {
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().RemoveAllowed().Return(errors.OperationBlockedError("TestDeleteKeys"))

	_, err := s.api.DeleteKeys(params.ModifyUserSSHKeys{})

	c.Assert(params.IsCodeOperationBlocked(err), tc.IsTrue)
}

func (s *keyManagerSuite) TestDeleteJujuSystemKey(c *tc.C) {
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().RemoveAllowed().Return(nil)

	key1 := sshtesting.ValidKeyOne.Key + " juju-client-key"
	key2 := sshtesting.ValidKeyTwo.Key + " " + config.JujuSystemKey
	key3 := sshtesting.ValidKeyThree.Key + " a user key"
	s.setAuthorizedKeys(c, key1, key2, key3)

	newAttrs := map[string]interface{}{
		config.AuthorizedKeysKey: strings.Join([]string{key1, key2, key3}, "\n"),
	}
	s.model.EXPECT().UpdateModelConfig(newAttrs, nil)

	args := params.ModifyUserSSHKeys{
		User: names.NewUserTag("admin").Name(),
		Keys: []string{"juju-client-key", config.JujuSystemKey},
	}
	results, err := s.api.DeleteKeys(args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ServerError("may not delete internal key: juju-client-key")},
			{Error: apiservertesting.ServerError("may not delete internal key: " + config.JujuSystemKey)},
		},
	})
}

// This should be impossible to do anyway since it's impossible to request
// to remove the client and system key
func (s *keyManagerSuite) TestCannotDeleteAllKeys(c *tc.C) {
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().RemoveAllowed().Return(nil)

	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	s.setAuthorizedKeys(c, key1, key2)

	args := params.ModifyUserSSHKeys{
		User: names.NewUserTag("admin").String(),
		Keys: []string{sshtesting.ValidKeyTwo.Fingerprint, "user@host"},
	}
	_, err := s.api.DeleteKeys(args)
	c.Assert(err, tc.ErrorMatches, "cannot delete all keys")
}

func (s *keyManagerSuite) assertImportKeys(c *tc.C) {
	key1 := sshtesting.ValidKeyOne.Key + " user@host"
	key2 := sshtesting.ValidKeyTwo.Key
	key3 := sshtesting.ValidKeyThree.Key
	key4 := sshtesting.ValidKeyFour.Key
	keymv := strings.Split(sshtesting.ValidKeyMulti, "\n")
	keymp := strings.Split(sshtesting.PartValidKeyMulti, "\n")
	keymi := strings.Split(sshtesting.MultiInvalid, "\n")
	s.setAuthorizedKeys(c, key1, key2, "bad key")

	newAttrs := map[string]interface{}{
		config.AuthorizedKeysKey: strings.Join([]string{
			key1, key2, "bad key", key3, keymv[0], keymv[1], keymp[0], key4,
		}, "\n"),
	}
	s.model.EXPECT().UpdateModelConfig(newAttrs, nil)

	args := params.ModifyUserSSHKeys{
		User: names.NewUserTag("admin").String(),
		Keys: []string{
			"lp:existing",
			"lp:validuser",
			"invalid-key",
			"lp:multi",
			"lp:multiempty",
			"lp:multipartial",
			"lp:multiinvalid",
			"lp:multionedup",
		},
	}
	results, err := s.api.ImportKeys(args)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 8)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ServerError(fmt.Sprintf("duplicate ssh key: %s", key2))},
			{Error: nil},
			{Error: apiservertesting.ServerError("invalid ssh key id: invalid-key")},
			{Error: nil},
			{Error: apiservertesting.ServerError("invalid ssh key id: lp:multiempty")},
			{Error: apiservertesting.ServerError(fmt.Sprintf(
				`invalid ssh key for lp:multipartial: `+
					`generating key fingerprint: `+
					`invalid authorized_key "%s"`, keymp[1]))},
			{Error: apiservertesting.ServerError(fmt.Sprintf(
				`invalid ssh key for lp:multiinvalid: `+
					`generating key fingerprint: `+
					`invalid authorized_key "%s"`+"\n"+
					`invalid ssh key for lp:multiinvalid: `+
					`generating key fingerprint: `+
					`invalid authorized_key "%s"`, keymi[0], keymi[1]))},
			{Error: apiservertesting.ServerError(fmt.Sprintf("duplicate ssh key: %s", key2))},
		},
	})
}

func (s *keyManagerSuite) TestImportKeys(c *tc.C) {
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().ChangeAllowed().Return(nil)
	s.assertImportKeys(c)
}

func (s *keyManagerSuite) TestImportKeysSuperUser(c *tc.C) {
	s.apiUser = names.NewUserTag("superuser-fred")
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().ChangeAllowed().Return(nil)
	s.assertImportKeys(c)
}

func (s *keyManagerSuite) TestImportKeysModelAdmin(c *tc.C) {
	s.apiUser = names.NewUserTag("admin" + coretesting.ModelTag.String())
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().ChangeAllowed().Return(nil)
	s.assertImportKeys(c)
}

func (s *keyManagerSuite) TestImportKeysNonAuthorised(c *tc.C) {
	s.apiUser = names.NewUserTag("fred")
	defer s.setup(c).Finish()

	_, err := s.api.ImportKeys(params.ModifyUserSSHKeys{})
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(params.ErrCode(err), tc.Equals, params.CodeUnauthorized)
}

func (s *keyManagerSuite) TestImportJujuSystemKey(c *tc.C) {
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().ChangeAllowed().Return(nil)

	key1 := sshtesting.ValidKeyOne.Key
	s.setAuthorizedKeys(c, key1)
	newAttrs := map[string]interface{}{
		config.AuthorizedKeysKey: key1,
	}
	s.model.EXPECT().UpdateModelConfig(newAttrs, nil)

	args := params.ModifyUserSSHKeys{
		User: names.NewUserTag("admin").String(),
		Keys: []string{"lp:systemkey"},
	}
	results, err := s.api.ImportKeys(args)
	c.Assert(err, tc.IsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ServerError("may not add key with comment juju-system-key: " + keymanagertesting.SystemKey)},
		},
	})
}

func (s *keyManagerSuite) TestBlockImportKeys(c *tc.C) {
	defer s.setup(c).Finish()
	s.blockChecker.EXPECT().ChangeAllowed().Return(errors.OperationBlockedError("TestImportKeys"))

	_, err := s.api.ImportKeys(params.ModifyUserSSHKeys{})

	c.Assert(params.IsCodeOperationBlocked(err), tc.IsTrue)
}
