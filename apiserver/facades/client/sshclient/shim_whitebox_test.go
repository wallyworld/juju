// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient

import (
	tctesting "testing"

	"github.com/gliderlabs/ssh"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
)

type shimSuite struct {
	testing.BaseSuite
}

func TestShimSuite(t *tctesting.T) {
	tc.Run(t, &shimSuite{})
}

var (
	hostKey = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACCP9y2SiMT+bxv25bNA3bpPtNqoZjFVQ5WRQ7/iqsXmRgAAAIiNBL3UjQS9
1AAAAAtzc2gtZWQyNTUxOQAAACCP9y2SiMT+bxv25bNA3bpPtNqoZjFVQ5WRQ7/iqsXmRg
AAAECXJNZYQFl7ccvfCeJPRgqteU7luG7g6lwMOPpPAPCUjo/3LZKIxP5vG/bls0Dduk+0
2qhmMVVDlZFDv+KqxeZGAAAABHRlc3QB
-----END OPENSSH PRIVATE KEY-----
`
)

func (s *shimSuite) TestGetAuthorizedKey(c *tc.C) {
	key, err := getPublicKeyWireFormat([]byte(hostKey))
	c.Assert(err, tc.IsNil)

	_, err = ssh.ParsePublicKey(key)
	c.Assert(err, tc.IsNil)
}
