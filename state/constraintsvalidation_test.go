// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"regexp"
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/tc"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
)

type constraintsValidationSuite struct {
	ConnSuite
}

func TestConstraintsValidationSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &constraintsValidationSuite{})
}

func (s *constraintsValidationSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.policy.GetConstraintsValidator = func() (constraints.Validator, error) {
		validator := constraints.NewValidator()
		validator.RegisterConflicts(
			[]string{constraints.InstanceType},
			[]string{constraints.Mem, constraints.Arch},
		)
		validator.RegisterUnsupported([]string{constraints.CpuPower})
		return validator, nil
	}
}

func (s *constraintsValidationSuite) addOneMachine(c *tc.C, cons constraints.Value) (*state.Machine, error) {
	return s.State.AddOneMachine(state.MachineTemplate{
		Base:        state.UbuntuBase("12.10"),
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: cons,
	})
}

var setConstraintsTests = []struct {
	about        string
	consToSet    string
	consFallback string

	effectiveModelCons       string // model constraints after setting consFallback
	effectiveApplicationCons string // application constraints after setting consToSet
	effectiveUnitCons        string // unit constraints after setting consToSet on the application
	effectiveMachineCons     string // machine constraints after setting consToSet
}{{
	about:        "(implicitly) empty constraints are OK and stored as empty",
	consToSet:    "",
	consFallback: "",

	effectiveModelCons:       "",
	effectiveApplicationCons: "",
	effectiveUnitCons:        "arch=amd64",
	effectiveMachineCons:     "",
}, {
	about:        "(implicitly) empty fallback constraints never override set constraints",
	consToSet:    "instance-type=foo-42 cpu-power=9001 spaces=bar zones=az1,az2",
	consFallback: "",

	effectiveModelCons: "", // model constraints are stored as empty.
	// i.e. there are no fallbacks and all the following cases are the same.
	effectiveApplicationCons: "instance-type=foo-42 cpu-power=9001 spaces=bar zones=az1,az2",
	effectiveUnitCons:        "instance-type=foo-42 cpu-power=9001 spaces=bar zones=az1,az2",
	effectiveMachineCons:     "instance-type=foo-42 cpu-power=9001 spaces=bar zones=az1,az2",
}, {
	about:        "(implicitly) empty constraints never override explicitly set fallbacks",
	consToSet:    "",
	consFallback: "arch=amd64 cores=42 mem=2G tags=foo",

	effectiveModelCons:       "arch=amd64 cores=42 mem=2G tags=foo",
	effectiveApplicationCons: "", // set as given.
	effectiveUnitCons:        "arch=amd64 cores=42 mem=2G tags=foo",
	// set as given, then merged with fallbacks; since consToSet is
	// empty, the effective values inherit everything from fallbacks;
	// like the unit, but only because the application constraints are
	// also empty.
	effectiveMachineCons: "arch=amd64 cores=42 mem=2G tags=foo",
}, {
	about:        "(explicitly) empty constraints are OK and stored as given",
	consToSet:    "cores= cpu-power= root-disk= instance-type= container= tags= spaces=",
	consFallback: "",

	effectiveModelCons:       "",
	effectiveApplicationCons: "cores= cpu-power= root-disk= instance-type= container= tags= spaces=",
	effectiveUnitCons:        "arch=amd64 cores= cpu-power= root-disk= instance-type= container= tags= spaces=",
	effectiveMachineCons:     "cores= cpu-power= instance-type= root-disk= tags= spaces=", // container= is dropped
}, {
	about:        "(explicitly) empty fallback constraints are OK and stored as given",
	consToSet:    "",
	consFallback: "cores= cpu-power= root-disk= instance-type= container= tags= spaces=",

	effectiveModelCons:       "cores= cpu-power= root-disk= instance-type= container= tags= spaces=",
	effectiveApplicationCons: "",
	effectiveUnitCons:        "arch=amd64 cores= cpu-power= root-disk= instance-type= container= tags= spaces=",
	effectiveMachineCons:     "cores= cpu-power= instance-type= root-disk= tags= spaces=", // container= is dropped
}, {
	about:                    "(explicitly) empty constraints and fallbacks are OK and stored as given",
	consToSet:                "arch= mem= cores= container=",
	consFallback:             "cores= cpu-power= root-disk= instance-type= container= tags= spaces=",
	effectiveModelCons:       "cores= cpu-power= root-disk= instance-type= container= tags= spaces=",
	effectiveApplicationCons: "arch= mem= cores= container=",
	effectiveUnitCons:        "arch=amd64 container= cores= cpu-power= mem= root-disk= tags= spaces=",
	effectiveMachineCons:     "arch= cores= cpu-power= mem= root-disk= tags= spaces=", // container= is dropped
}, {
	about:        "(explicitly) empty constraints override set fallbacks for deployment and provisioning",
	consToSet:    "cores= arch= spaces= cpu-power=",
	consFallback: "cores=42 arch=amd64 tags=foo spaces=default,^dmz mem=4G",

	effectiveModelCons:       "cores=42 arch=amd64 tags=foo spaces=default,^dmz mem=4G",
	effectiveApplicationCons: "cores= arch= spaces= cpu-power=",
	effectiveUnitCons:        "arch=amd64 cores= cpu-power= mem=4G tags=foo spaces=",
	effectiveMachineCons:     "arch= cores= cpu-power= mem=4G tags=foo spaces=",
	// we're also checking if m.SetConstraints() does the same with
	// regards to the effective constraints as AddMachine(), because
	// some of these tests proved they had different behavior (i.e.
	// container= was not set to empty)
}, {
	about:        "non-empty constraints always override empty or unset fallbacks",
	consToSet:    "cores=42 root-disk=20G arch=amd64 tags=foo,bar",
	consFallback: "cores= arch= tags=",

	effectiveModelCons:       "cores= arch= tags=",
	effectiveApplicationCons: "cores=42 root-disk=20G arch=amd64 tags=foo,bar",
	effectiveUnitCons:        "cores=42 root-disk=20G arch=amd64 tags=foo,bar",
	effectiveMachineCons:     "cores=42 root-disk=20G arch=amd64 tags=foo,bar",
}, {
	about:        "non-empty constraints always override set fallbacks",
	consToSet:    "cores=42 root-disk=20G arch=amd64 tags=foo,bar",
	consFallback: "cores=12 root-disk=10G arch=s390x  tags=bar",

	effectiveModelCons:       "cores=12 root-disk=10G arch=s390x  tags=bar",
	effectiveApplicationCons: "cores=42 root-disk=20G arch=amd64 tags=foo,bar",
	effectiveUnitCons:        "cores=42 root-disk=20G arch=amd64 tags=foo,bar",
	effectiveMachineCons:     "cores=42 root-disk=20G arch=amd64 tags=foo,bar",
}, {
	about:        "non-empty constraints override conflicting set fallbacks",
	consToSet:    "mem=8G arch=amd64 cores=4 tags=bar",
	consFallback: "instance-type=small cpu-power=1000", // instance-type conflicts mem, arch

	effectiveModelCons:       "instance-type=small cpu-power=1000",
	effectiveApplicationCons: "mem=8G arch=amd64 cores=4 tags=bar",
	// both of the following contain the explicitly set constraints after
	// resolving any conflicts with fallbacks (by dropping them).
	effectiveUnitCons:    "mem=8G arch=amd64 cores=4 tags=bar cpu-power=1000",
	effectiveMachineCons: "mem=8G arch=amd64 cores=4 tags=bar cpu-power=1000",
}, {
	about:        "set fallbacks are overridden the same way for provisioning and deployment",
	consToSet:    "tags= cpu-power= spaces=bar",
	consFallback: "tags=foo cpu-power=42",

	// a variation of the above case showing there's no difference
	// between deployment (application, unit) and provisioning (machine)
	// constraints when it comes to effective values.
	effectiveModelCons:       "tags=foo cpu-power=42",
	effectiveApplicationCons: "cpu-power= tags= spaces=bar",
	effectiveUnitCons:        "arch=amd64 cpu-power= tags= spaces=bar",
	effectiveMachineCons:     "cpu-power= tags= spaces=bar",
}, {
	about:        "container type can only be used for deployment, not provisioning",
	consToSet:    "container=kvm arch=amd64",
	consFallback: "container=lxd mem=8G",

	// application deployment constraints are transformed into machine
	// provisioning constraints, and the container type only makes
	// sense currently as a deployment constraint, so it's cleared
	// when merging application/model deployment constraints into
	// effective machine provisioning constraints.
	effectiveModelCons:       "container=lxd mem=8G",
	effectiveApplicationCons: "container=kvm arch=amd64",
	effectiveUnitCons:        "container=kvm mem=8G arch=amd64",
	effectiveMachineCons:     "mem=8G arch=amd64",
}, {
	about:        "specify image virt-type when deploying applications on multi-hypervisor aware openstack",
	consToSet:    "virt-type=kvm",
	consFallback: "",

	// application deployment constraints are transformed into machine
	// provisioning constraints. Unit constraints must also have virt-type set
	// to ensure consistency in scalability.
	effectiveModelCons:       "",
	effectiveApplicationCons: "virt-type=kvm",
	effectiveUnitCons:        "arch=amd64 virt-type=kvm",
	effectiveMachineCons:     "virt-type=kvm",
}, {
	about:        "ensure model and application constraints are separate",
	consToSet:    "virt-type=kvm",
	consFallback: "mem=2G",

	// application deployment constraints are transformed into machine
	// provisioning constraints. Unit constraints must also have virt-type set
	// to ensure consistency in scalability.
	effectiveModelCons:       "mem=2G",
	effectiveApplicationCons: "virt-type=kvm",
	effectiveUnitCons:        "arch=amd64 mem=2G virt-type=kvm",
	effectiveMachineCons:     "mem=2G virt-type=kvm",
}}

func (s *constraintsValidationSuite) TestMachineConstraints(c *tc.C) {
	for i, t := range setConstraintsTests {
		c.Logf(
			"test %d: %s\nconsToSet: %q\nconsFallback: %q\n",
			i, t.about, t.consToSet, t.consFallback,
		)
		// Set fallbacks as model constraints and verify them.
		err := s.State.SetModelConstraints(constraints.MustParse(t.consFallback))
		c.Check(err, tc.ErrorIsNil)
		econs, err := s.State.ModelConstraints()
		c.Check(err, tc.ErrorIsNil)
		c.Check(econs, tc.DeepEquals, constraints.MustParse(t.effectiveModelCons))
		// Set the machine provisioning constraints.
		m, err := s.addOneMachine(c, constraints.MustParse(t.consToSet))
		c.Check(err, tc.ErrorIsNil)
		// New machine provisioning constraints get merged with the fallbacks.
		cons, err := m.Constraints()
		c.Check(err, tc.ErrorIsNil)
		c.Check(cons, tc.DeepEquals, constraints.MustParse(t.effectiveMachineCons))
		// Changing them should result in the same result before provisioning.
		err = m.SetConstraints(constraints.MustParse(t.consToSet))
		c.Check(err, tc.ErrorIsNil)
		cons, err = m.Constraints()
		c.Check(err, tc.ErrorIsNil)
		c.Check(cons, tc.DeepEquals, constraints.MustParse(t.effectiveMachineCons))
	}
}

func (s *constraintsValidationSuite) TestApplicationConstraints(c *tc.C) {
	charm := s.AddTestingCharm(c, "wordpress")
	application := s.AddTestingApplication(c, "wordpress", charm)
	for i, t := range setConstraintsTests {
		c.Logf(
			"test %d: %s\nconsToSet: %q\nconsFallback: %q\n",
			i, t.about, t.consToSet, t.consFallback,
		)
		// Set fallbacks as model constraints and verify them.
		err := s.State.SetModelConstraints(constraints.MustParse(t.consFallback))
		c.Check(err, tc.ErrorIsNil)
		econs, err := s.State.ModelConstraints()
		c.Check(err, tc.ErrorIsNil)
		c.Check(econs, tc.DeepEquals, constraints.MustParse(t.effectiveModelCons))
		// Set the application deployment constraints.
		err = application.SetConstraints(constraints.MustParse(t.consToSet))
		c.Check(err, tc.ErrorIsNil)
		u, err := application.AddUnit(state.AddUnitParams{})
		c.Check(err, tc.ErrorIsNil)
		// New unit deployment constraints get merged with the fallbacks.
		ucons, err := u.Constraints()
		c.Check(err, tc.ErrorIsNil)
		c.Check(*ucons, tc.DeepEquals, constraints.MustParse(t.effectiveUnitCons))
		// Application constraints remain as set.
		scons, err := application.Constraints()
		c.Check(err, tc.ErrorIsNil)
		c.Check(scons, tc.DeepEquals, constraints.MustParse(t.effectiveApplicationCons))
	}
}

type applicationConstraintsSuite struct {
	ConnSuite

	applicationName string
	testCharm       *state.Charm
}

func TestApplicationConstraintsSuite(t *tctesting.T) {
	testing.MgoTestPackage(t, &applicationConstraintsSuite{})
}

func (s *applicationConstraintsSuite) SetUpTest(c *tc.C) {
	s.ConnSuite.SetUpTest(c)
	s.policy.GetConstraintsValidator = func() (constraints.Validator, error) {
		validator := constraints.NewValidator()
		validator.RegisterVocabulary(constraints.VirtType, []string{"kvm"})
		return validator, nil
	}
	s.applicationName = "wordpress"
	s.testCharm = s.AddTestingCharm(c, s.applicationName)
}

func (s *applicationConstraintsSuite) TestAddApplicationInvalidConstraints(c *tc.C) {
	cons := constraints.MustParse("virt-type=blah")
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: s.applicationName,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Charm:       s.testCharm,
		Constraints: cons,
	})
	c.Assert(errors.Cause(err), tc.ErrorMatches, regexp.QuoteMeta("invalid constraint value: virt-type=blah\nvalid values are: kvm"))
}

func (s *applicationConstraintsSuite) TestAddApplicationValidConstraints(c *tc.C) {
	cons := constraints.MustParse("virt-type=kvm")
	application, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: s.applicationName,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Charm:       s.testCharm,
		Constraints: cons,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(application, tc.NotNil)
}

func (s *applicationConstraintsSuite) TestConstraintsRetrieval(c *tc.C) {
	posCons := constraints.MustParse("arch=amd64 spaces=db")
	application, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: s.applicationName,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Charm:       s.testCharm,
		Constraints: posCons,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(application, tc.NotNil)

	negCons := constraints.MustParse("arch=amd64 spaces=^db2")
	negApplication, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "unimportant",
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Charm:       s.testCharm,
		Constraints: negCons,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(negApplication, tc.NotNil)

	cons, err := s.State.AllConstraints()
	c.Assert(err, tc.ErrorIsNil)

	vals := make([]string, len(cons))
	for i, cons := range cons {
		c.Log(cons.ID())
		vals[i] = cons.Value().String()
	}
	// In addition to the application constraints, there is a single empty
	// constraints document for the model.
	c.Check(vals, tc.SameContents, []string{posCons.String(), negCons.String(), ""})

	cons, err = s.State.ConstraintsBySpaceName("db")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cons, tc.HasLen, 1)
	c.Check(cons[0].Value(), tc.DeepEquals, posCons)

	cons, err = s.State.ConstraintsBySpaceName("db2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cons, tc.HasLen, 1)
	c.Check(cons[0].Value(), tc.DeepEquals, negCons)
}

func (s *applicationConstraintsSuite) TestConstraintsSpaceNameChangeOps(c *tc.C) {
	posCons := constraints.MustParse("spaces=db")
	application, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: s.applicationName,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
		Charm:       s.testCharm,
		Constraints: posCons,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(application, tc.NotNil)

	cons, err := s.State.ConstraintsBySpaceName("db")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cons, tc.HasLen, 1)

	ops := cons[0].ChangeSpaceNameOps("db", "external")
	c.Assert(ops, tc.HasLen, 1)
	op := ops[0]
	c.Check(op.C, tc.Equals, "constraints")
	c.Check(op.Assert, tc.Equals, txn.DocExists)

	bd, ok := op.Update.(bson.D)
	c.Assert(ok, tc.IsTrue)
	c.Assert(bd, tc.HasLen, 1)
}
