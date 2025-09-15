// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imageutils_test

import (
	"net/http"
	tctesting "testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/provider/azure/internal/azuretesting"
	"github.com/juju/juju/internal/provider/azure/internal/imageutils"
	"github.com/juju/juju/internal/testing"
)

type imageutilsSuite struct {
	testing.BaseSuite

	mockSender *azuretesting.MockSender
	client     *armcompute.VirtualMachineImagesClient
	callCtx    *context.CloudCallContext
}

func TestImageutilsSuite(t *tctesting.T) {
	tc.Run(t, &imageutilsSuite{})
}

func (s *imageutilsSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mockSender = &azuretesting.MockSender{}
	opts := &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: s.mockSender,
		},
	}
	var err error
	s.client, err = armcompute.NewVirtualMachineImagesClient("subscription-id", &azuretesting.FakeCredential{}, opts)
	c.Assert(err, tc.ErrorIsNil)
	s.callCtx = context.NewEmptyCloudCallContext()
}

func (s *imageutilsSuite) TestBaseImageOldStyleGen2(c *tc.C) {
	s.mockSender.AppendResponse(azuretesting.NewResponseWithContent(
		`[{"name": "20_04-lts-gen2"}, {"name": "20_04-lts"}, {"name": "20_04-lts-arm64"}]`,
	))
	image, err := imageutils.BaseImage(s.callCtx, corebase.MakeDefaultBase("ubuntu", "20.04"), "released", "westus", "", s.client, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(image, tc.NotNil)
	c.Assert(image, tc.DeepEquals, &instances.Image{
		Id:       "Canonical:0001-com-ubuntu-server-focal:20_04-lts-gen2:latest",
		Arch:     arch.AMD64,
		VirtType: "Hyper-V",
	})
}

func (s *imageutilsSuite) TestBaseImageOldStyleARM64(c *tc.C) {
	s.mockSender.AppendResponse(azuretesting.NewResponseWithContent(
		`[{"name": "20_04-lts-gen2"}, {"name": "20_04-lts"}, {"name": "20_04-lts-arm64"}]`,
	))
	image, err := imageutils.BaseImage(s.callCtx, corebase.MakeDefaultBase("ubuntu", "20.04"), "released", "westus", "arm64", s.client, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(image, tc.NotNil)
	c.Assert(image, tc.DeepEquals, &instances.Image{
		Id:       "Canonical:0001-com-ubuntu-server-focal:20_04-lts-arm64:latest",
		Arch:     arch.ARM64,
		VirtType: "Hyper-V",
	})
}

func (s *imageutilsSuite) TestBaseImageOldStyleFallbackToGen1(c *tc.C) {
	s.mockSender.AppendResponse(azuretesting.NewResponseWithContent(
		`[{"name": "20_04-lts-gen2"}, {"name": "20_04-lts"}, {"name": "20_04-lts-arm64"}]`,
	))
	image, err := imageutils.BaseImage(s.callCtx, corebase.MakeDefaultBase("ubuntu", "20.04"), "released", "westus", "", s.client, true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(image, tc.NotNil)
	c.Assert(image, tc.DeepEquals, &instances.Image{
		Id:       "Canonical:0001-com-ubuntu-server-focal:20_04-lts:latest",
		Arch:     arch.AMD64,
		VirtType: "Hyper-V",
	})
}

func (s *imageutilsSuite) TestBaseImageOldStyleInvalidSKU(c *tc.C) {
	s.mockSender.AppendResponse(azuretesting.NewResponseWithContent(
		`[{"name": "22_04-invalid"}, {"name": "22_04-lts"}]`,
	))
	image, err := imageutils.BaseImage(s.callCtx, corebase.MakeDefaultBase("ubuntu", "22.04"), "released", "westus", "", s.client, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(image, tc.NotNil)
	c.Assert(image, tc.DeepEquals, &instances.Image{
		Id:       "Canonical:0001-com-ubuntu-server-jammy:22_04-lts:latest",
		Arch:     arch.AMD64,
		VirtType: "Hyper-V",
	})
}

func (s *imageutilsSuite) TestBaseImageGen2(c *tc.C) {
	s.mockSender.AppendResponse(azuretesting.NewResponseWithContent(
		`[{"name": "server"}, {"name": "server-gen1"}, {"name": "server-arm64"}]`,
	))
	image, err := imageutils.BaseImage(s.callCtx, corebase.MakeDefaultBase("ubuntu", "24.04"), "released", "westus", "", s.client, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(image, tc.NotNil)
	c.Assert(image, tc.DeepEquals, &instances.Image{
		Id:       "Canonical:ubuntu-24_04-lts:server:latest",
		Arch:     arch.AMD64,
		VirtType: "Hyper-V",
	})
}

func (s *imageutilsSuite) TestBaseImageFallbackToGen1(c *tc.C) {
	s.mockSender.AppendResponse(azuretesting.NewResponseWithContent(
		`[{"name": "server"}, {"name": "server-gen1"}, {"name": "server-arm64"}]`,
	))
	image, err := imageutils.BaseImage(s.callCtx, corebase.MakeDefaultBase("ubuntu", "24.04"), "released", "westus", "", s.client, true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(image, tc.NotNil)
	c.Assert(image, tc.DeepEquals, &instances.Image{
		Id:       "Canonical:ubuntu-24_04-lts:server-gen1:latest",
		Arch:     arch.AMD64,
		VirtType: "Hyper-V",
	})
}

func (s *imageutilsSuite) TestBaseImageARM64(c *tc.C) {
	s.mockSender.AppendResponse(azuretesting.NewResponseWithContent(
		`[{"name": "server"}, {"name": "server-gen1"}, {"name": "server-arm64"}]`,
	))
	image, err := imageutils.BaseImage(s.callCtx, corebase.MakeDefaultBase("ubuntu", "24.04"), "released", "westus", "arm64", s.client, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(image, tc.NotNil)
	c.Assert(image, tc.DeepEquals, &instances.Image{
		Id:       "Canonical:ubuntu-24_04-lts:server-arm64:latest",
		Arch:     arch.ARM64,
		VirtType: "Hyper-V",
	})
}

func (s *imageutilsSuite) TestBaseImageNonLTS(c *tc.C) {
	s.mockSender.AppendResponse(azuretesting.NewResponseWithContent(
		`[{"name": "server"}, {"name": "server-gen1"}, {"name": "server-arm64"}]`,
	))
	image, err := imageutils.BaseImage(s.callCtx, corebase.MakeDefaultBase("ubuntu", "25.04"), "released", "westus", "", s.client, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(image, tc.NotNil)
	c.Assert(image, tc.DeepEquals, &instances.Image{
		Id:       "Canonical:ubuntu-25_04:server:latest",
		Arch:     arch.AMD64,
		VirtType: "Hyper-V",
	})
}

func (s *imageutilsSuite) TestBaseImageCentOS(c *tc.C) {
	for _, cseries := range []string{"7", "8"} {
		base := corebase.MakeDefaultBase("centos", cseries)
		image, err := imageutils.BaseImage(s.callCtx, base, "released", "westus", "", s.client, false)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(image.Id, tc.Equals, "OpenLogic:CentOS:7.3:latest")
	}
}

func (s *imageutilsSuite) TestBaseImageStreamDaily(c *tc.C) {
	s.mockSender.AppendAndRepeatResponse(azuretesting.NewResponseWithContent(
		`[{"name": "server"}, {"name": "minimal-gen1"}, {"name": "minimal-arm64"}, {"name": "minimal"}]`), 2)
	base := corebase.MakeDefaultBase("ubuntu", "24.04")

	image, err := imageutils.BaseImage(s.callCtx, base, "daily", "westus", "", s.client, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(image.Id, tc.Equals, "Canonical:ubuntu-24_04-lts-daily:server:latest")
}

func (s *imageutilsSuite) TestBaseImageOldStyleNotFound(c *tc.C) {
	s.mockSender.AppendResponse(azuretesting.NewResponseWithContent(`[]`))
	image, err := imageutils.BaseImage(s.callCtx, corebase.MakeDefaultBase("ubuntu", "22.04"), "released", "westus", "", s.client, false)
	c.Assert(err, tc.ErrorMatches, `selecting SKU for ubuntu@22.04: legacy ubuntu "jammy" SKUs for released stream not found`)
	c.Assert(image, tc.IsNil)
}

func (s *imageutilsSuite) TestBaseImageNotFound(c *tc.C) {
	s.mockSender.AppendResponse(azuretesting.NewResponseWithContent(`[]`))
	image, err := imageutils.BaseImage(s.callCtx, corebase.MakeDefaultBase("ubuntu", "24.04"), "released", "westus", "", s.client, false)
	c.Assert(err, tc.ErrorMatches, `selecting SKU for ubuntu@24.04: ubuntu "ubuntu@24.04/stable" SKUs for released stream not found`)
	c.Assert(image, tc.IsNil)
}

func (s *imageutilsSuite) TestBaseImageStreamNotFound(c *tc.C) {
	s.mockSender.AppendResponse(azuretesting.NewResponseWithContent(`[{"name": "22_04-beta1"}]`))
	_, err := imageutils.BaseImage(s.callCtx, corebase.MakeDefaultBase("ubuntu", "22.04"), "whatever", "westus", "", s.client, false)
	c.Assert(err, tc.ErrorMatches, `selecting SKU for ubuntu@22.04: legacy ubuntu "jammy" SKUs for whatever stream not found`)
}

func (s *imageutilsSuite) TestBaseImageStreamThrewCredentialError(c *tc.C) {
	s.mockSender.AppendResponse(azuretesting.NewResponseWithStatus("401 Unauthorized", http.StatusUnauthorized))
	called := false
	s.callCtx.InvalidateCredentialFunc = func(string) error {
		called = true
		return nil
	}

	_, err := imageutils.BaseImage(s.callCtx, corebase.MakeDefaultBase("ubuntu", "22.04"), "whatever", "westus", "", s.client, false)
	c.Assert(err.Error(), tc.Contains, "RESPONSE 401")
	c.Assert(called, tc.IsTrue)
}

func (s *imageutilsSuite) TestBaseImageStreamThrewNonCredentialError(c *tc.C) {
	s.mockSender.AppendResponse(azuretesting.NewResponseWithStatus("308 Permanent Redirect", http.StatusPermanentRedirect))
	called := false
	s.callCtx.InvalidateCredentialFunc = func(string) error {
		called = true
		return nil
	}

	_, err := imageutils.BaseImage(s.callCtx, corebase.MakeDefaultBase("ubuntu", "22.04"), "whatever", "westus", "", s.client, false)
	c.Assert(err.Error(), tc.Contains, "RESPONSE 308")
	c.Assert(called, tc.IsFalse)
}
