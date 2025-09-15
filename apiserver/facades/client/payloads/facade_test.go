// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloads_test

import (
	tctesting "testing"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"

	api "github.com/juju/juju/api/client/payloads"
	"github.com/juju/juju/apiserver/facades/client/payloads"
	corepayloads "github.com/juju/juju/core/payloads"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

func TestSuite(t *tctesting.T) {
	tc.Run(t, &Suite{})
}

type Suite struct {
	testhelpers.IsolationSuite

	stub  *testhelpers.Stub
	state *stubState
}

func (s *Suite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testhelpers.Stub{}
	s.state = &stubState{stub: s.stub}
}

func (Suite) newPayload(name string) (corepayloads.FullPayloadInfo, params.Payload) {
	ptype := "docker"
	id := "id" + name
	tags := []string{"a-tag"}
	unit := "a-application/0"
	machine := "1"

	pl := corepayloads.FullPayloadInfo{
		Payload: corepayloads.Payload{
			PayloadClass: charm.PayloadClass{
				Name: name,
				Type: ptype,
			},
			ID:     id,
			Status: corepayloads.StateRunning,
			Labels: tags,
			Unit:   unit,
		},
		Machine: machine,
	}
	apiPayload := params.Payload{
		Class:   name,
		Type:    ptype,
		ID:      id,
		Status:  corepayloads.StateRunning,
		Labels:  tags,
		Unit:    names.NewUnitTag(unit).String(),
		Machine: names.NewMachineTag(machine).String(),
	}
	return pl, apiPayload
}

func (s *Suite) TestListNoPatterns(c *tc.C) {
	payloadA, apiPayloadA := s.newPayload("spam")
	payloadB, apiPayloadB := s.newPayload("eggs")
	s.state.payloads = append(s.state.payloads, payloadA, payloadB)

	facade := payloads.NewAPI(s.state)
	args := params.PayloadListArgs{
		Patterns: []string{},
	}
	results, err := facade.List(args)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(results, tc.DeepEquals, params.PayloadListResults{
		Results: []params.Payload{
			apiPayloadA,
			apiPayloadB,
		},
	})
}

func (s *Suite) TestListAllMatch(c *tc.C) {
	payloadA, apiPayloadA := s.newPayload("spam")
	payloadB, apiPayloadB := s.newPayload("eggs")
	s.state.payloads = append(s.state.payloads, payloadA, payloadB)

	facade := payloads.NewAPI(s.state)
	args := params.PayloadListArgs{
		Patterns: []string{
			"a-application/0",
		},
	}
	results, err := facade.List(args)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(results, tc.DeepEquals, params.PayloadListResults{
		Results: []params.Payload{
			apiPayloadA,
			apiPayloadB,
		},
	})
}

func (s *Suite) TestListNoMatch(c *tc.C) {
	payloadA, _ := s.newPayload("spam")
	payloadB, _ := s.newPayload("eggs")
	s.state.payloads = append(s.state.payloads, payloadA, payloadB)

	facade := payloads.NewAPI(s.state)
	args := params.PayloadListArgs{
		Patterns: []string{
			"a-application/1",
		},
	}
	results, err := facade.List(args)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(results.Results, tc.HasLen, 0)
}

func (s *Suite) TestListNoPayloads(c *tc.C) {
	facade := payloads.NewAPI(s.state)
	args := params.PayloadListArgs{
		Patterns: []string{},
	}
	results, err := facade.List(args)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(results.Results, tc.HasLen, 0)
}

func (s *Suite) TestListMultiMatch(c *tc.C) {
	payloadA, apiPayloadA := s.newPayload("spam")
	payloadB, apiPayloadB := s.newPayload("eggs")
	s.state.payloads = append(s.state.payloads, payloadA, payloadB)

	facade := payloads.NewAPI(s.state)
	args := params.PayloadListArgs{
		Patterns: []string{
			"spam",
			"eggs",
		},
	}
	results, err := facade.List(args)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(results, tc.DeepEquals, params.PayloadListResults{
		Results: []params.Payload{
			apiPayloadA,
			apiPayloadB,
		},
	})
}

func (s *Suite) TestListPartialMatch(c *tc.C) {
	payloadA, apiPayloadA := s.newPayload("spam")
	payloadB, _ := s.newPayload("eggs")
	s.state.payloads = append(s.state.payloads, payloadA, payloadB)

	facade := payloads.NewAPI(s.state)
	args := params.PayloadListArgs{
		Patterns: []string{
			"spam",
		},
	}
	results, err := facade.List(args)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(results, tc.DeepEquals, params.PayloadListResults{
		Results: []params.Payload{
			apiPayloadA,
		},
	})
}

func (s *Suite) TestListPartialMultiMatch(c *tc.C) {
	payloadA, apiPayloadA := s.newPayload("spam")
	payloadB, _ := s.newPayload("eggs")
	payloadC, apiPayloadC := s.newPayload("ham")
	s.state.payloads = append(s.state.payloads, payloadA, payloadB, payloadC)

	facade := payloads.NewAPI(s.state)
	args := params.PayloadListArgs{
		Patterns: []string{
			"spam",
			"ham",
		},
	}
	results, err := facade.List(args)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(results, tc.DeepEquals, params.PayloadListResults{
		Results: []params.Payload{
			apiPayloadA,
			apiPayloadC,
		},
	})
}

func (s *Suite) TestListAllFilters(c *tc.C) {
	pl := corepayloads.FullPayloadInfo{
		Payload: corepayloads.Payload{
			PayloadClass: charm.PayloadClass{
				Name: "spam",
				Type: "docker",
			},
			ID:     "idspam",
			Status: corepayloads.StateRunning,
			Labels: []string{"a-tag"},
			Unit:   "a-application/0",
		},
		Machine: "1",
	}
	apiPayload := api.Payload2api(pl)
	s.state.payloads = append(s.state.payloads, pl)

	facade := payloads.NewAPI(s.state)
	patterns := []string{
		"spam",                    // name
		"docker",                  // type
		"idspam",                  // ID
		corepayloads.StateRunning, // status
		"a-application/0",         // unit
		"1",                       // machine
		"a-tag",                   // tags
	}
	for _, pattern := range patterns {
		c.Logf("trying pattern %q", pattern)

		args := params.PayloadListArgs{
			Patterns: []string{
				pattern,
			},
		}
		results, err := facade.List(args)
		c.Assert(err, tc.ErrorIsNil)

		c.Check(results, tc.DeepEquals, params.PayloadListResults{
			Results: []params.Payload{
				apiPayload,
			},
		})
	}
}

type stubState struct {
	stub *testhelpers.Stub

	payloads []corepayloads.FullPayloadInfo
}

func (s *stubState) ListAll() ([]corepayloads.FullPayloadInfo, error) {
	s.stub.AddCall("ListAll")
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.payloads, nil
}
