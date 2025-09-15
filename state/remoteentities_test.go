// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3"
)

type RemoteEntitiesSuite struct {
	ConnSuite
}

func TestRemoteEntitiesSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &RemoteEntitiesSuite{})
}

func (s *RemoteEntitiesSuite) assertExportLocalEntity(c *tc.C, entity names.Tag) string {
	re := s.State.RemoteEntities()
	token, err := re.ExportLocalEntity(entity)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(token, tc.Not(tc.Equals), "")
	return token
}

func (s *RemoteEntitiesSuite) TestAllRemoteEntities(c *tc.C) {
	entity := names.NewApplicationTag("mysql")
	token := s.assertExportLocalEntity(c, entity)

	expected, err := s.State.AllRemoteEntities()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(expected, tc.HasLen, 1)

	remoteEntity := expected[0]
	c.Assert(entity.String(), tc.Equals, remoteEntity.ID())
	c.Assert(token, tc.Equals, remoteEntity.Token())
}

func (s *RemoteEntitiesSuite) TestExportLocalEntity(c *tc.C) {
	entity := names.NewApplicationTag("mysql")
	token := s.assertExportLocalEntity(c, entity)

	re := s.State.RemoteEntities()
	expected, err := re.GetToken(entity)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(token, tc.Equals, expected)
}

func (s *RemoteEntitiesSuite) TestExportLocalEntityTwice(c *tc.C) {
	entity := names.NewApplicationTag("mysql")
	expected := s.assertExportLocalEntity(c, entity)
	re := s.State.RemoteEntities()
	token, err := re.ExportLocalEntity(entity)
	c.Assert(err, tc.Satisfies, errors.IsAlreadyExists)
	c.Assert(token, tc.Equals, expected)
}

func (s *RemoteEntitiesSuite) TestGetRemoteEntity(c *tc.C) {
	entity := names.NewApplicationTag("mysql")
	token := s.assertExportLocalEntity(c, entity)

	re := s.State.RemoteEntities()
	expected, err := re.GetRemoteEntity(token)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(entity, tc.Equals, expected)
}

func (s *RemoteEntitiesSuite) TestMacaroon(c *tc.C) {
	entity := names.NewRelationTag("mysql:db wordpress:db")
	s.assertExportLocalEntity(c, entity)

	re := s.State.RemoteEntities()
	mac, err := newMacaroon("id")
	c.Assert(err, tc.ErrorIsNil)

	err = re.SaveMacaroon(names.NewApplicationTag("foo"), mac)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)

	err = re.SaveMacaroon(entity, mac)
	c.Assert(err, tc.ErrorIsNil)

	re = s.State.RemoteEntities()
	expected, err := re.GetMacaroon(entity)
	c.Assert(err, tc.ErrorIsNil)
	assertMacaroonEquals(c, mac, expected)
}

func (s *RemoteEntitiesSuite) TestRemoveRemoteEntity(c *tc.C) {
	entity := names.NewApplicationTag("mysql")
	token := s.assertExportLocalEntity(c, entity)

	re := s.State.RemoteEntities()
	err := re.RemoveRemoteEntity(entity)
	c.Assert(err, tc.ErrorIsNil)
	re = s.State.RemoteEntities()
	_, err = re.GetRemoteEntity(token)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
}

func (s *RemoteEntitiesSuite) TestImportRemoteEntity(c *tc.C) {
	re := s.State.RemoteEntities()
	entity := names.NewApplicationTag("mysql")
	token := utils.MustNewUUID().String()
	err := re.ImportRemoteEntity(entity, token)
	c.Assert(err, tc.ErrorIsNil)

	re = s.State.RemoteEntities()
	expected, err := re.GetRemoteEntity(token)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(entity, tc.Equals, expected)
}

func (s *RemoteEntitiesSuite) TestImportRemoteEntityOverwrites(c *tc.C) {
	re := s.State.RemoteEntities()
	entity := names.NewApplicationTag("mysql")
	token := utils.MustNewUUID().String()
	err := re.ImportRemoteEntity(entity, token)
	c.Assert(err, tc.ErrorIsNil)

	anotherToken := utils.MustNewUUID().String()
	err = re.ImportRemoteEntity(entity, anotherToken)
	c.Assert(err, tc.ErrorIsNil)

	re = s.State.RemoteEntities()
	_, err = re.GetRemoteEntity(token)
	c.Assert(err, tc.Satisfies, errors.IsNotFound)
	expected, err := re.GetRemoteEntity(anotherToken)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(entity, tc.Equals, expected)
}

func (s *RemoteEntitiesSuite) TestImportRemoteEntityEmptyToken(c *tc.C) {
	re := s.State.RemoteEntities()
	entity := names.NewApplicationTag("mysql")
	err := re.ImportRemoteEntity(entity, "")
	c.Assert(err, tc.Satisfies, errors.IsNotValid)
}
