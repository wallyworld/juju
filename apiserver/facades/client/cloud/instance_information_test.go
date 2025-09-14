// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	tctesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	credentialcommonmocks "github.com/juju/juju/apiserver/common/credentialcommon/mocks"
	cloudfacade "github.com/juju/juju/apiserver/facades/client/cloud"
	"github.com/juju/juju/apiserver/facades/client/cloud/mocks"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	environsmocks "github.com/juju/juju/environs/mocks"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type instanceTypesSuite struct {
	backend                     *mocks.MockBackend
	ctrlBackend                 *mocks.MockBackend
	pool                        *mocks.MockModelPoolBackend
	authorizer                  *testing.FakeAuthorizer
	credcommonPersistentBackend *credentialcommonmocks.MockPersistentBackend
}

func (s *instanceTypesSuite) setup(c *tc.C, userTag names.UserTag) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.backend = mocks.NewMockBackend(ctrl)
	s.ctrlBackend = mocks.NewMockBackend(ctrl)
	s.pool = mocks.NewMockModelPoolBackend(ctrl)
	s.credcommonPersistentBackend = credentialcommonmocks.NewMockPersistentBackend(ctrl)
	s.authorizer = &testing.FakeAuthorizer{
		Tag: userTag,
	}

	return ctrl
}

func TestInstanceTypesSuite(t *tctesting.T) {
	tc.Run(t, &instanceTypesSuite{})
}

var over9kCPUCores uint64 = 9001

func (p *instanceTypesSuite) TestInstanceTypes(c *tc.C) {
	adminTag := names.NewUserTag("admin")
	ctrl := p.setup(c, adminTag)
	defer ctrl.Finish()

	mockModel := mocks.NewMockModel(ctrl)

	itCons := constraints.Value{CpuCores: &over9kCPUCores}
	failureCons := constraints.Value{}

	mockEnv := environsmocks.NewMockEnviron(ctrl)
	mockEnv.EXPECT().InstanceTypes(gomock.Any(),
		itCons).Return(instances.InstanceTypesWithCostMetadata{
		CostUnit:     "USD/h",
		CostCurrency: "USD",
		InstanceTypes: []instances.InstanceType{
			{Name: "instancetype-1"},
			{Name: "instancetype-2"}},
	}, nil)
	mockEnv.EXPECT().InstanceTypes(gomock.Any(),
		failureCons).Return(instances.InstanceTypesWithCostMetadata{},
		errors.NotFoundf("Instances matching constraint "))

	fakeEnvironGet := func(
		st environs.EnvironConfigGetter,
		newEnviron environs.NewEnvironFunc,
	) (environs.Environ, error) {
		c.Assert(st.ControllerUUID(), tc.Equals, coretesting.FakeControllerConfig().ControllerUUID())
		return mockEnv, nil
	}

	mockModel.EXPECT().UUID().Return(coretesting.ModelTag.Id()).AnyTimes()
	mockModel.EXPECT().CloudName().Return("aws").AnyTimes()

	p.ctrlBackend.EXPECT().Model().Return(mockModel, nil)
	p.pool.EXPECT().GetModelCallContext(coretesting.ModelTag.Id()).Return(p.credcommonPersistentBackend,
		context.NewEmptyCloudCallContext(), func() bool { return false }, nil)
	p.backend.EXPECT().ControllerTag().Return(coretesting.ControllerTag)
	p.backend.EXPECT().ControllerConfig().Return(
		controller.Config{
			"controller-uuid": coretesting.ControllerTag.Id(),
		}, nil)

	api, err := cloudfacade.NewCloudAPI(p.backend, p.ctrlBackend, p.pool, p.authorizer)
	c.Assert(err, tc.ErrorIsNil)

	cons := params.CloudInstanceTypesConstraints{
		Constraints: []params.CloudInstanceTypesConstraint{
			{CloudTag: "cloud-aws",
				CloudRegion: "a-region",
				Constraints: &itCons},
			{CloudTag: "cloud-aws",
				CloudRegion: "a-region",
				Constraints: &failureCons},
			{CloudTag: "cloud-gce",
				CloudRegion: "a-region",
				Constraints: &itCons}},
	}
	r, err := cloudfacade.InstanceTypes(api, fakeEnvironGet, cons)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(r.Results, tc.HasLen, 3)
	expected := []params.InstanceTypesResult{
		{
			InstanceTypes: []params.InstanceType{
				{Name: "instancetype-1"},
				{Name: "instancetype-2"}},
			CostUnit:     "USD/h",
			CostCurrency: "USD",
		},
		{
			Error: &params.Error{Message: "Instances matching constraint  not found", Code: "not found"}},
		{
			Error: &params.Error{Message: "asking gce cloud information to aws cloud not valid", Code: "not valid"}}}
	c.Assert(r.Results, tc.DeepEquals, expected)
}

func (p *instanceTypesSuite) TestInstanceTypesGettingControllerConfigFail(c *tc.C) {
	adminTag := names.NewUserTag("admin")
	ctrl := p.setup(c, adminTag)
	defer ctrl.Finish()

	mockModel := mocks.NewMockModel(ctrl)

	mockEnv := environsmocks.NewMockEnviron(ctrl)
	fakeEnvironGet := func(
		st environs.EnvironConfigGetter,
		newEnviron environs.NewEnvironFunc,
	) (environs.Environ, error) {
		return mockEnv, nil
	}

	mockModel.EXPECT().UUID().Return(coretesting.ModelTag.Id()).AnyTimes()
	p.ctrlBackend.EXPECT().Model().Return(mockModel, nil)
	p.pool.EXPECT().GetModelCallContext(coretesting.ModelTag.Id()).Return(p.credcommonPersistentBackend,
		context.NewEmptyCloudCallContext(), func() bool { return false }, nil)
	p.backend.EXPECT().ControllerTag().Return(coretesting.ControllerTag)
	p.backend.EXPECT().ControllerConfig().Return(controller.Config{}, errors.New("broken controller"))

	api, err := cloudfacade.NewCloudAPI(p.backend, p.ctrlBackend, p.pool, p.authorizer)
	c.Assert(err, tc.ErrorIsNil)

	itCons := constraints.Value{CpuCores: &over9kCPUCores}
	failureCons := constraints.Value{}
	cons := params.CloudInstanceTypesConstraints{
		Constraints: []params.CloudInstanceTypesConstraint{
			{CloudTag: "cloud-aws",
				CloudRegion: "a-region",
				Constraints: &itCons},
			{CloudTag: "cloud-aws",
				CloudRegion: "a-region",
				Constraints: &failureCons},
			{CloudTag: "cloud-gce",
				CloudRegion: "a-region",
				Constraints: &itCons}},
	}
	r, err := cloudfacade.InstanceTypes(api, fakeEnvironGet, cons)
	c.Assert(err, tc.ErrorMatches, "getting controller config: broken controller")
	c.Assert(r, tc.DeepEquals, params.InstanceTypesResults{})
}
