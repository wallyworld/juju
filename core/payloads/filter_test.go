// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloads_test

import (
	tctesting "testing"

	"github.com/juju/charm/v12"
	"github.com/juju/tc"

	"github.com/juju/juju/core/payloads"
	"github.com/juju/juju/internal/testhelpers"
)

func TestFilterSuite(t *tctesting.T) {
	tc.Run(t, &filterSuite{})
}

type filterSuite struct {
	testhelpers.IsolationSuite
}

func (s *filterSuite) newPayload(name string) payloads.FullPayloadInfo {
	return payloads.FullPayloadInfo{
		Payload: payloads.Payload{
			PayloadClass: charm.PayloadClass{
				Name: name,
				Type: "docker",
			},
			ID:     "id" + name,
			Status: "running",
			Labels: []string{"a-tag"},
			Unit:   "a-application/0",
		},
		Machine: "1",
	}
}

func (s *filterSuite) TestFilterOkay(c *tc.C) {
	payloadInfo := []payloads.FullPayloadInfo{
		s.newPayload("spam"),
	}
	predicate := func(payloads.FullPayloadInfo) bool {
		return true
	}
	matched := payloads.Filter(payloadInfo, predicate)

	c.Check(matched, tc.DeepEquals, payloadInfo)
}

func (s *filterSuite) TestFilterMatchAll(c *tc.C) {
	payloadInfo := []payloads.FullPayloadInfo{
		s.newPayload("spam"),
		s.newPayload("eggs"),
	}
	predicate := func(payloads.FullPayloadInfo) bool {
		return true
	}
	matched := payloads.Filter(payloadInfo, predicate)

	c.Check(matched, tc.DeepEquals, payloadInfo)
}

func (s *filterSuite) TestFilterMatchNone(c *tc.C) {
	payloadInfo := []payloads.FullPayloadInfo{
		s.newPayload("spam"),
	}
	predicate := func(payloads.FullPayloadInfo) bool {
		return false
	}
	matched := payloads.Filter(payloadInfo, predicate)

	c.Check(matched, tc.HasLen, 0)
}

func (s *filterSuite) TestFilterNoPayloads(c *tc.C) {
	predicate := func(payloads.FullPayloadInfo) bool {
		return true
	}
	matched := payloads.Filter(nil, predicate)

	c.Check(matched, tc.HasLen, 0)
}

func (s *filterSuite) TestFilterMatchPartial(c *tc.C) {
	payloadInfo := []payloads.FullPayloadInfo{
		s.newPayload("spam"),
		s.newPayload("eggs"),
	}
	predicate := func(p payloads.FullPayloadInfo) bool {
		return p.Name == "spam"
	}
	matched := payloads.Filter(payloadInfo, predicate)

	c.Check(matched, tc.DeepEquals, payloadInfo[:1])
}

func (s *filterSuite) TestFilterMultiMatch(c *tc.C) {
	payloadInfo := []payloads.FullPayloadInfo{
		s.newPayload("spam"),
		s.newPayload("eggs"),
	}
	predA := func(p payloads.FullPayloadInfo) bool {
		return p.Name == "spam"
	}
	predB := func(p payloads.FullPayloadInfo) bool {
		return p.Name == "eggs"
	}
	matched := payloads.Filter(payloadInfo, predA, predB)

	c.Check(matched, tc.DeepEquals, payloadInfo)
}

func (s *filterSuite) TestFilterMultiMatchPartial(c *tc.C) {
	payloadInfo := []payloads.FullPayloadInfo{
		s.newPayload("spam"),
		s.newPayload("eggs"),
		s.newPayload("ham"),
	}
	predA := func(p payloads.FullPayloadInfo) bool {
		return p.Name == "ham"
	}
	predB := func(p payloads.FullPayloadInfo) bool {
		return p.Name == "spam"
	}
	matched := payloads.Filter(payloadInfo, predA, predB)

	c.Check(matched, tc.DeepEquals, []payloads.FullPayloadInfo{
		s.newPayload("spam"),
		s.newPayload("ham"),
	})
}

func (s *filterSuite) TestBuildPredicatesForOkay(c *tc.C) {
	pl := payloads.FullPayloadInfo{
		Payload: payloads.Payload{
			PayloadClass: charm.PayloadClass{
				Name: "spam",
				Type: "docker",
			},
			ID:     "idspam",
			Status: "running",
			Labels: []string{"tagA", "tagB"},
			Unit:   "a-application/0",
		},
		Machine: "1",
	}

	// Check matching patterns.

	patterns := []string{
		"spam",
		"docker",
		"idspam",
		"running",
		"tagA",
		"tagB",
		"a-application/0",
		"1",
	}
	for _, pattern := range patterns {
		predicates, err := payloads.BuildPredicatesFor([]string{
			pattern,
		})
		c.Assert(err, tc.ErrorIsNil)

		c.Check(predicates, tc.HasLen, 1)
		matched := predicates[0](pl)
		c.Check(matched, tc.IsTrue)
	}

	// Check a non-matching pattern.

	predicates, err := payloads.BuildPredicatesFor([]string{
		"tagC",
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(predicates, tc.HasLen, 1)
	matched := predicates[0](pl)
	c.Check(matched, tc.IsFalse)
}

func (s *filterSuite) TestBuildPredicatesForMulti(c *tc.C) {
	predicates, err := payloads.BuildPredicatesFor([]string{
		"tagC",
		"spam",
		"1",
		"2",
		"idspam",
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(predicates, tc.HasLen, 5)
	pl := s.newPayload("spam")
	var matches []bool
	for _, pred := range predicates {
		matched := pred(pl)
		matches = append(matches, matched)
	}
	c.Check(matches, tc.DeepEquals, []bool{
		false,
		true,
		true,
		false,
		true,
	})
}

func (s *filterSuite) TestMatch(c *tc.C) {
	pl := payloads.FullPayloadInfo{
		Payload: payloads.Payload{
			PayloadClass: charm.PayloadClass{
				Name: "spam",
				Type: "docker",
			},
			ID:     "idspam",
			Status: "running",
			Labels: []string{"tagA", "tagB"},
			Unit:   "a-application/0",
		},
		Machine: "1",
	}

	// match
	for _, pattern := range []string{
		"spam",
		"docker",
		"idspam",
		"running",
		"tagA",
		"tagB",
		"a-application/0",
		"1",
	} {
		c.Logf("check %q", pattern)
		matched := payloads.Match(pl, pattern)
		c.Check(matched, tc.IsTrue)
	}

	// no match
	for _, pattern := range []string{
		"tagC",
		"2",
	} {
		c.Logf("check %q", pattern)
		matched := payloads.Match(pl, pattern)
		c.Check(matched, tc.IsFalse)
	}
}
