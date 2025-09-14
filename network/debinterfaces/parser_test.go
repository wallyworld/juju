// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debinterfaces_test

import (
	"fmt"
	"os"
	"path/filepath"
	tctesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/network/debinterfaces"
)

type ParserSuite struct {
	testhelpers.IsolationSuite

	expander debinterfaces.WordExpander
}

func TestParserSuite(t *tctesting.T) {
	tc.Run(t, &ParserSuite{})
}

// Ensure wordExpanderWithError is a WordExpander
var _ debinterfaces.WordExpander = (*wordExpanderErrors)(nil)

type wordExpanderErrors struct {
	errmsg string
}

func wordExpanderWithError(errmsg string) debinterfaces.WordExpander {
	return &wordExpanderErrors{errmsg: errmsg}
}

func (w *wordExpanderErrors) Expand(s string) ([]string, error) {
	return nil, fmt.Errorf("word expansion failed: %s", w.errmsg)
}

func (s *ParserSuite) SetUpTest(c *tc.C) {
	s.expander = debinterfaces.NewWordExpander()
}

func (s *ParserSuite) TestNilInput(c *tc.C) {
	interfaces, err := debinterfaces.ParseSource("", nil, s.expander)
	c.Assert(interfaces, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "filename and input is nil")
}

func (s *ParserSuite) TestNilInputAndNoFilename(c *tc.C) {
	stanzas, err := debinterfaces.ParseSource("", "", s.expander)
	c.Assert(err, tc.IsNil)
	c.Check(stanzas, tc.HasLen, 0)
}

func (s *ParserSuite) TestUnsupportedInputType(c *tc.C) {
	stanzas, err := debinterfaces.ParseSource("", []float64{3.141}, s.expander)
	c.Assert(err, tc.ErrorMatches, "invalid source type")
	c.Check(stanzas, tc.IsNil)
}

func (s *ParserSuite) TestFilenameEmpty(c *tc.C) {
	emptyFile := filepath.Join(c.MkDir(), "TestFilenameEmpty")
	err := os.WriteFile(emptyFile, []byte(""), 0644)
	c.Assert(err, tc.IsNil)
	stanzas, err := debinterfaces.ParseSource(emptyFile, nil, s.expander)
	c.Assert(err, tc.IsNil)
	c.Check(stanzas, tc.HasLen, 0)
}

func (s *ParserSuite) TestParseErrorObject(c *tc.C) {
	content := `hunky dory`
	_, err := debinterfaces.ParseSource("", content, s.expander)
	c.Assert(err, tc.NotNil)
	_, parseError := err.(*debinterfaces.ParseError)
	c.Assert(parseError, tc.NotNil)
	c.Assert(err, tc.ErrorMatches, "misplaced option")
}

func (s *ParserSuite) TestCommentsAndBlankLinesOnly(c *tc.C) {
	content := `

# Comment 1.

# Comment 2, after empty line.

  # An indented comment, followed by a line with leading whitespace

`
	stanzas, err := debinterfaces.ParseSource("", content, s.expander)
	c.Assert(err, tc.IsNil)
	c.Assert(stanzas, tc.HasLen, 0)
}

func (s *ParserSuite) TestAllowStanzaMissingArg(c *tc.C) {
	_, err := debinterfaces.ParseSource("", "allow-hotplug", s.expander)
	c.Assert(err, tc.NotNil)
	c.Check(err, tc.ErrorMatches, "missing device name")
}

func (s *ParserSuite) TestAutoStanzaMissingArg(c *tc.C) {
	_, err := debinterfaces.ParseSource("", "auto", s.expander)
	c.Assert(err, tc.NotNil)
	c.Check(err, tc.ErrorMatches, "missing device name")
}

func (s *ParserSuite) TestIfaceStanzaMissingArg(c *tc.C) {
	_, err := debinterfaces.ParseSource("", "iface", s.expander)
	c.Assert(err, tc.NotNil)
	c.Check(err, tc.ErrorMatches, "missing device name")
}

func (s *ParserSuite) TestMappingStanzaMissingArg(c *tc.C) {
	_, err := debinterfaces.ParseSource("", "mapping", s.expander)
	c.Assert(err, tc.NotNil)
	c.Check(err, tc.ErrorMatches, "missing device name")
}

func (s *ParserSuite) TestNoAutoDownStanzaMissingArg(c *tc.C) {
	_, err := debinterfaces.ParseSource("", "no-auto-down", s.expander)
	c.Assert(err, tc.NotNil)
	c.Check(err, tc.ErrorMatches, "missing device name")
}

func (s *ParserSuite) TestNoScriptsStanzaMissingArg(c *tc.C) {
	_, err := debinterfaces.ParseSource("", "no-scripts", s.expander)
	c.Assert(err, tc.NotNil)
	c.Check(err, tc.ErrorMatches, "missing device name")
}

func (s *ParserSuite) TestSourceStanzaMissingArg(c *tc.C) {
	_, err := debinterfaces.ParseSource("", "source", s.expander)
	c.Assert(err, tc.NotNil)
	c.Check(err, tc.ErrorMatches, "missing filename")
}

func (s *ParserSuite) TestSourceDirectoryStanzaMissingArg(c *tc.C) {
	_, err := debinterfaces.ParseSource("", "source-directory", s.expander)
	c.Assert(err, tc.NotNil)
	c.Check(err, tc.ErrorMatches, "missing directory")
}

func (s *ParserSuite) TestAllowStanza(c *tc.C) {
	definition := "allow-hotplug eth0 eth1 eth2"
	stanzas, err := debinterfaces.ParseSource("", definition, s.expander)
	c.Assert(err, tc.IsNil)
	c.Assert(stanzas, tc.HasLen, 1)
	c.Assert(stanzas[0], tc.FitsTypeOf, debinterfaces.AllowStanza{})
	allow := stanzas[0].(debinterfaces.AllowStanza)
	c.Check(allow.Location().Filename, tc.Equals, "")
	c.Check(allow.Location().LineNum, tc.Equals, 1)
	c.Assert(allow.Definition(), tc.HasLen, 1)
	c.Check(allow.Definition()[0], tc.Equals, definition)
	c.Check(allow.DeviceNames, tc.DeepEquals, []string{"eth0", "eth1", "eth2"})
}

func (s *ParserSuite) TestAutoStanza(c *tc.C) {
	definition := "auto eth0 eth1 eth2"
	stanzas, err := debinterfaces.ParseSource("", definition, s.expander)
	c.Assert(err, tc.IsNil)
	c.Assert(stanzas, tc.HasLen, 1)
	c.Assert(stanzas[0], tc.FitsTypeOf, debinterfaces.AutoStanza{})
	auto := stanzas[0].(debinterfaces.AutoStanza)
	c.Check(auto.Location().Filename, tc.Equals, "")
	c.Check(auto.Location().LineNum, tc.Equals, 1)
	c.Assert(auto.Definition(), tc.HasLen, 1)
	c.Check(auto.Definition()[0], tc.Equals, definition)
	c.Check(auto.DeviceNames, tc.DeepEquals, []string{"eth0", "eth1", "eth2"})
}

func (s *ParserSuite) TestWithIfaceStanzaAndOneOption(c *tc.C) {
	content := `
iface eth0 inet manual
  # A comment.
  address 192.168.1.254/24 `
	stanzas, err := debinterfaces.ParseSource("", content, s.expander)
	c.Assert(err, tc.IsNil)
	c.Assert(stanzas, tc.HasLen, 1)
	c.Assert(stanzas[0], tc.FitsTypeOf, debinterfaces.IfaceStanza{})
	iface := stanzas[0].(debinterfaces.IfaceStanza)
	c.Assert(iface.Definition(), tc.HasLen, 2)
	c.Check(iface.Definition()[0], tc.Equals, "iface eth0 inet manual")
	c.Check(iface.Definition()[1], tc.Equals, "address 192.168.1.254/24")
	c.Check(iface.Location().Filename, tc.Equals, "")
	c.Check(iface.Location().LineNum, tc.Equals, 2)
	c.Check(iface.Options, tc.HasLen, 1)
	c.Check(iface.Options[0], tc.Equals, "address 192.168.1.254/24")
	c.Check(iface.HasBondMasterOption, tc.Equals, false)
	c.Check(iface.HasBondOptions, tc.Equals, false)
	c.Check(iface.IsAlias, tc.Equals, false)
	c.Check(iface.IsBridged, tc.Equals, false)
	c.Check(iface.IsVLAN, tc.Equals, false)
}

func (s *ParserSuite) TestWithIfaceStanzaAndMultipleOptions(c *tc.C) {
	content := `
iface eth0 inet manual
  # A comment.
  address 192.168.1.254/24
  dns-nameservers 8.8.8.8
  # Another comment.
  dns-search ubuntu.com
  # An ending comment`
	stanzas, err := debinterfaces.ParseSource("", content, s.expander)
	c.Assert(err, tc.IsNil)
	c.Assert(stanzas, tc.HasLen, 1)
	c.Assert(stanzas[0], tc.FitsTypeOf, debinterfaces.IfaceStanza{})
	iface := stanzas[0].(debinterfaces.IfaceStanza)
	c.Assert(iface.Definition(), tc.HasLen, 4)
	c.Check(iface.Definition()[0], tc.Equals, "iface eth0 inet manual")
	c.Check(iface.Definition()[1], tc.Equals, "address 192.168.1.254/24")
	c.Check(iface.Definition()[2], tc.Equals, "dns-nameservers 8.8.8.8")
	c.Check(iface.Definition()[3], tc.Equals, "dns-search ubuntu.com")
	c.Check(iface.Location().Filename, tc.Equals, "")
	c.Check(iface.Location().LineNum, tc.Equals, 2)
	c.Assert(iface.Options, tc.HasLen, 3)
	c.Check(iface.Options[0], tc.Equals, "address 192.168.1.254/24")
	c.Check(iface.Options[1], tc.Equals, "dns-nameservers 8.8.8.8")
	c.Check(iface.Options[2], tc.Equals, "dns-search ubuntu.com")
	c.Check(iface.HasBondMasterOption, tc.Equals, false)
	c.Check(iface.HasBondOptions, tc.Equals, false)
	c.Check(iface.IsAlias, tc.Equals, false)
	c.Check(iface.IsBridged, tc.Equals, false)
	c.Check(iface.IsVLAN, tc.Equals, false)
}

func (s *ParserSuite) TestWithIfaceStanzaAndNoOptions(c *tc.C) {
	content := `
iface eth0 inet manual
# A comment.`
	stanzas, err := debinterfaces.ParseSource("", content, s.expander)
	c.Assert(err, tc.IsNil)
	c.Assert(stanzas, tc.HasLen, 1)
	c.Assert(stanzas[0], tc.FitsTypeOf, debinterfaces.IfaceStanza{})
	iface := stanzas[0].(debinterfaces.IfaceStanza)
	c.Assert(iface.Definition(), tc.HasLen, 1)
	c.Check(iface.Definition()[0], tc.Equals, "iface eth0 inet manual")
	c.Check(iface.Location().Filename, tc.Equals, "")
	c.Check(iface.Location().LineNum, tc.Equals, 2)
	c.Check(iface.Options, tc.HasLen, 0)
	c.Check(iface.HasBondMasterOption, tc.Equals, false)
	c.Check(iface.HasBondOptions, tc.Equals, false)
	c.Check(iface.IsAlias, tc.Equals, false)
	c.Check(iface.IsBridged, tc.Equals, false)
	c.Check(iface.IsVLAN, tc.Equals, false)
}

func (s *ParserSuite) TestAutoStanzaFollowedByIfaceStanza(c *tc.C) {
	content := `
auto eth0

iface eth0 inet static
  address 192.168.1.254/24
  gateway 192.168.1.1`
	stanzas, err := debinterfaces.ParseSource("", content, s.expander)
	c.Assert(err, tc.IsNil)
	c.Assert(stanzas, tc.HasLen, 2)
	c.Assert(stanzas[0], tc.FitsTypeOf, debinterfaces.AutoStanza{})
	c.Assert(stanzas[1], tc.FitsTypeOf, debinterfaces.IfaceStanza{})

	auto := stanzas[0].(debinterfaces.AutoStanza)
	c.Assert(auto.Definition(), tc.HasLen, 1)
	c.Check(auto.Definition()[0], tc.Equals, "auto eth0")
	c.Check(auto.Location().Filename, tc.Equals, "")
	c.Check(auto.Location().LineNum, tc.Equals, 2)

	iface := stanzas[1].(debinterfaces.IfaceStanza)
	c.Assert(iface.Definition(), tc.HasLen, 3)
	c.Check(iface.Definition()[0], tc.Equals, "iface eth0 inet static")
	c.Check(iface.Definition()[1], tc.Equals, "address 192.168.1.254/24")
	c.Check(iface.Definition()[2], tc.Equals, "gateway 192.168.1.1")
	c.Check(iface.Location().Filename, tc.Equals, "")
	c.Check(iface.Location().LineNum, tc.Equals, 4)
	c.Assert(iface.Options, tc.HasLen, 2)
	c.Check(iface.Options[0], tc.Equals, "address 192.168.1.254/24")
	c.Check(iface.Options[1], tc.Equals, "gateway 192.168.1.1")
	c.Check(iface.HasBondMasterOption, tc.Equals, false)
	c.Check(iface.HasBondOptions, tc.Equals, false)
	c.Check(iface.IsAlias, tc.Equals, false)
	c.Check(iface.IsBridged, tc.Equals, false)
	c.Check(iface.IsVLAN, tc.Equals, false)
}

func (s *ParserSuite) TestVLANIfaceStanza(c *tc.C) {
	content := `
iface eth0.100 inet static
  address 192.168.1.254/24
  vlan-raw-device eth1`
	stanzas, err := debinterfaces.ParseSource("", content, s.expander)
	c.Assert(err, tc.IsNil)
	c.Assert(stanzas, tc.HasLen, 1)
	c.Assert(stanzas[0], tc.FitsTypeOf, debinterfaces.IfaceStanza{})

	iface := stanzas[0].(debinterfaces.IfaceStanza)
	c.Assert(iface.Definition(), tc.HasLen, 3)
	c.Check(iface.Definition()[0], tc.Equals, "iface eth0.100 inet static")
	c.Check(iface.Definition()[1], tc.Equals, "address 192.168.1.254/24")
	c.Check(iface.Definition()[2], tc.Equals, "vlan-raw-device eth1")
	c.Check(iface.Location().Filename, tc.Equals, "")
	c.Check(iface.Location().LineNum, tc.Equals, 2)
	c.Assert(iface.Options, tc.HasLen, 2)
	c.Check(iface.Options[0], tc.Equals, "address 192.168.1.254/24")
	c.Check(iface.Options[1], tc.Equals, "vlan-raw-device eth1")
	c.Check(iface.HasBondMasterOption, tc.Equals, false)
	c.Check(iface.HasBondOptions, tc.Equals, false)
	c.Check(iface.IsAlias, tc.Equals, false)
	c.Check(iface.IsBridged, tc.Equals, false)
	c.Check(iface.IsVLAN, tc.Equals, true)
}

func (s *ParserSuite) TestBondedIfaceStanza(c *tc.C) {
	content := `
auto eth0
iface eth0 inet manual
    bond-lacp_rate slow
    bond-xmit_hash_policy layer2
    bond-miimon 100
    bond-master bond0
    bond-mode active-backup

auto eth1
iface eth1 inet manual
    bond-lacp_rate slow
    bond-xmit_hash_policy layer2
    bond-miimon 100
    bond-master bond0
    mtu 1500
    bond-mode active-backup

auto bond0
iface bond0 inet dhcp
    bond-lacp_rate slow
    bond-xmit_hash_policy layer2
    bond-miimon 100
    mtu 1500
    bond-mode active-backup
    hwaddress 52:54:00:1c:f1:5b
    bond-slaves none`
	stanzas, err := debinterfaces.ParseSource("", content, s.expander)
	c.Assert(err, tc.IsNil)
	c.Assert(stanzas, tc.HasLen, 6)
	c.Assert(stanzas[0], tc.FitsTypeOf, debinterfaces.AutoStanza{})
	c.Assert(stanzas[1], tc.FitsTypeOf, debinterfaces.IfaceStanza{})
	c.Assert(stanzas[2], tc.FitsTypeOf, debinterfaces.AutoStanza{})
	c.Assert(stanzas[3], tc.FitsTypeOf, debinterfaces.IfaceStanza{})
	c.Assert(stanzas[4], tc.FitsTypeOf, debinterfaces.AutoStanza{})
	c.Assert(stanzas[5], tc.FitsTypeOf, debinterfaces.IfaceStanza{})

	iface0 := stanzas[1].(debinterfaces.IfaceStanza)
	c.Assert(iface0.Definition(), tc.HasLen, 6)
	c.Check(iface0.Definition()[0], tc.Equals, "iface eth0 inet manual")
	c.Check(iface0.Location().Filename, tc.Equals, "")
	c.Check(iface0.Location().LineNum, tc.Equals, 3)
	c.Check(iface0.Options, tc.HasLen, 5)
	c.Check(iface0.HasBondMasterOption, tc.Equals, true)
	c.Check(iface0.HasBondOptions, tc.Equals, true)
	c.Check(iface0.IsAlias, tc.Equals, false)
	c.Check(iface0.IsBridged, tc.Equals, false)
	c.Check(iface0.IsVLAN, tc.Equals, false)

	iface1 := stanzas[3].(debinterfaces.IfaceStanza)
	c.Assert(iface1.Definition(), tc.HasLen, 7)
	c.Check(iface1.Definition()[0], tc.Equals, "iface eth1 inet manual")
	c.Check(iface1.Location().Filename, tc.Equals, "")
	c.Check(iface1.Location().LineNum, tc.Equals, 11)
	c.Check(iface1.Options, tc.HasLen, 6)
	c.Check(iface1.HasBondMasterOption, tc.Equals, true)
	c.Check(iface1.HasBondOptions, tc.Equals, true)
	c.Check(iface1.IsAlias, tc.Equals, false)
	c.Check(iface1.IsBridged, tc.Equals, false)
	c.Check(iface1.IsVLAN, tc.Equals, false)

	iface2 := stanzas[5].(debinterfaces.IfaceStanza)
	c.Assert(iface2.Definition(), tc.HasLen, 8)
	c.Check(iface2.Definition()[0], tc.Equals, "iface bond0 inet dhcp")
	c.Check(iface2.Location().Filename, tc.Equals, "")
	c.Check(iface2.Location().LineNum, tc.Equals, 20)
	c.Check(iface2.Options, tc.HasLen, 7)
	c.Check(iface2.HasBondOptions, tc.Equals, true)
	c.Check(iface2.HasBondMasterOption, tc.Equals, false)
	c.Check(iface2.IsAlias, tc.Equals, false)
	c.Check(iface2.IsBridged, tc.Equals, false)
	c.Check(iface2.IsVLAN, tc.Equals, false)
}

func (s *ParserSuite) TestMappingStanza(c *tc.C) {
	content := `
mapping eth0 eth1
  script /path/to/get-mac-address.sh
  map 11:22:33:44:55:66 lan
  map AA:BB:CC:DD:EE:FF internet`
	stanzas, err := debinterfaces.ParseSource("", content, s.expander)
	c.Assert(err, tc.IsNil)
	c.Assert(stanzas, tc.HasLen, 1)
	c.Assert(stanzas[0], tc.FitsTypeOf, debinterfaces.MappingStanza{})

	mapping := stanzas[0].(debinterfaces.MappingStanza)
	c.Assert(mapping.Definition(), tc.HasLen, 4)
	c.Check(mapping.Definition()[0], tc.Equals, "mapping eth0 eth1")
	c.Check(mapping.Location().Filename, tc.Equals, "")
	c.Check(mapping.Location().LineNum, tc.Equals, 2)
	c.Check(mapping.DeviceNames, tc.DeepEquals, []string{"eth0", "eth1"})
	c.Assert(mapping.Options, tc.HasLen, 3)
	c.Check(mapping.Options[0], tc.Equals, "script /path/to/get-mac-address.sh")
	c.Check(mapping.Options[1], tc.Equals, "map 11:22:33:44:55:66 lan")
	c.Check(mapping.Options[2], tc.Equals, "map AA:BB:CC:DD:EE:FF internet")
}

func (s *ParserSuite) TestNoAutoDownStanza(c *tc.C) {
	content := `no-auto-down eth0 eth1`
	stanzas, err := debinterfaces.ParseSource("", content, s.expander)
	c.Assert(err, tc.IsNil)
	c.Assert(stanzas, tc.HasLen, 1)
	c.Assert(stanzas[0], tc.FitsTypeOf, debinterfaces.NoAutoDownStanza{})

	noautodown := stanzas[0].(debinterfaces.NoAutoDownStanza)
	c.Assert(noautodown.Definition(), tc.HasLen, 1)
	c.Check(noautodown.Definition()[0], tc.Equals, "no-auto-down eth0 eth1")
	c.Check(noautodown.Location().Filename, tc.Equals, "")
	c.Check(noautodown.Location().LineNum, tc.Equals, 1)
	c.Check(noautodown.DeviceNames, tc.DeepEquals, []string{"eth0", "eth1"})
}

func (s *ParserSuite) TestNoScriptsStanza(c *tc.C) {
	content := `no-scripts eth0 eth1`
	stanzas, err := debinterfaces.ParseSource("", content, s.expander)
	c.Assert(err, tc.IsNil)
	c.Assert(stanzas, tc.HasLen, 1)
	c.Assert(stanzas[0], tc.FitsTypeOf, debinterfaces.NoScriptsStanza{})

	noscripts := stanzas[0].(debinterfaces.NoScriptsStanza)
	c.Assert(noscripts.Definition(), tc.HasLen, 1)
	c.Check(noscripts.Definition()[0], tc.Equals, "no-scripts eth0 eth1")
	c.Check(noscripts.Location().Filename, tc.Equals, "")
	c.Check(noscripts.Location().LineNum, tc.Equals, 1)
	c.Check(noscripts.DeviceNames, tc.DeepEquals, []string{"eth0", "eth1"})
}

func (s *ParserSuite) TestFailParseDirectoryAsInput(c *tc.C) {
	stanzas, err := debinterfaces.Parse("testdata/TestInputSourceStanza")
	c.Assert(err, tc.NotNil)
	c.Check(err, tc.ErrorMatches, ".* testdata/TestInputSourceStanza: .*")
	c.Assert(stanzas, tc.IsNil)
}

func (s *ParserSuite) TestSourceStanzaNonExistentFile(c *tc.C) {
	_, err := debinterfaces.Parse("testdata/TestInputSourceStanza/non-existent-file")
	c.Assert(err, tc.NotNil)
	c.Check(err, tc.ErrorMatches, ".* testdata/TestInputSourceStanza/non-existent-file: .*")
}

func (s *ParserSuite) TestSourceStanzaWhereGlobHasZeroMatches(c *tc.C) {
	stanzas, err := debinterfaces.ParseSource("testdata/TestSourceStanzaWhereGlobHasZeroMatches/interfaces", nil, s.expander)
	c.Assert(err, tc.IsNil)
	c.Assert(stanzas, tc.HasLen, 1)
	c.Assert(stanzas[0], tc.FitsTypeOf, debinterfaces.SourceStanza{})
	src := stanzas[0].(debinterfaces.SourceStanza)
	c.Assert(src.Sources, tc.HasLen, 0)
	c.Assert(src.Stanzas, tc.HasLen, 0)
}

func (s *ParserSuite) TestSourceStanzaWithRelativeFilenames(c *tc.C) {
	stanzas, err := debinterfaces.Parse("testdata/TestInputSourceStanza/interfaces")
	c.Assert(err, tc.IsNil)
	c.Assert(stanzas, tc.HasLen, 3)
	c.Assert(stanzas[0], tc.FitsTypeOf, debinterfaces.AutoStanza{})
	c.Assert(stanzas[1], tc.FitsTypeOf, debinterfaces.IfaceStanza{})
	c.Assert(stanzas[2], tc.FitsTypeOf, debinterfaces.SourceStanza{})

	c.Assert(stanzas[0].Location().Filename, tc.Equals, "testdata/TestInputSourceStanza/interfaces")
	c.Assert(stanzas[0].Location().LineNum, tc.Equals, 5)

	c.Assert(stanzas[1].Location().Filename, tc.Equals, "testdata/TestInputSourceStanza/interfaces")
	c.Assert(stanzas[1].Location().LineNum, tc.Equals, 6)

	c.Assert(stanzas[2].Location().Filename, tc.Equals, "testdata/TestInputSourceStanza/interfaces")
	c.Assert(stanzas[2].Location().LineNum, tc.Equals, 12)

	source := stanzas[2].(debinterfaces.SourceStanza)
	c.Check(source.Path, tc.Equals, "interfaces.d/*.cfg")

	c.Assert(source.Stanzas, tc.HasLen, 6)

	c.Assert(source.Stanzas[0], tc.FitsTypeOf, debinterfaces.AutoStanza{})
	c.Assert(source.Stanzas[1], tc.FitsTypeOf, debinterfaces.IfaceStanza{})
	c.Assert(source.Stanzas[2], tc.FitsTypeOf, debinterfaces.AutoStanza{})
	c.Assert(source.Stanzas[3], tc.FitsTypeOf, debinterfaces.IfaceStanza{})
	c.Assert(source.Stanzas[4], tc.FitsTypeOf, debinterfaces.AutoStanza{})
	c.Assert(source.Stanzas[5], tc.FitsTypeOf, debinterfaces.IfaceStanza{})

	basePath := filepath.Join("testdata", "TestInputSourceStanza", "interfaces.d")
	c.Check(source.Sources, tc.DeepEquals, []string{
		filepath.Join(basePath, "eth0.cfg"),
		filepath.Join(basePath, "eth1.cfg"),
		filepath.Join(basePath, "eth2.cfg"),
	})

	// Note: we don't have tests for stanzas nested > 1 deep.

	eth0 := source.Stanzas[1].(debinterfaces.IfaceStanza)
	eth1 := source.Stanzas[3].(debinterfaces.IfaceStanza)
	eth2 := source.Stanzas[5].(debinterfaces.IfaceStanza)

	c.Assert(eth0.Definition(), tc.HasLen, 1)
	c.Check(eth0.Definition()[0], tc.Equals, "iface eth0 inet dhcp")
	c.Check(eth0.Location().Filename, tc.Equals, filepath.Join(basePath, "eth0.cfg"))
	c.Check(eth0.Location().LineNum, tc.Equals, 2)

	c.Assert(eth1.Definition(), tc.HasLen, 3)
	c.Check(eth1.Definition()[0], tc.Equals, "iface eth1 inet static")
	c.Check(eth1.Definition()[1], tc.Equals, "address 192.168.1.64")
	c.Check(eth1.Definition()[2], tc.Equals, "dns-nameservers 192.168.1.254")
	c.Check(eth1.Location().Filename, tc.Equals, filepath.Join(basePath, "eth1.cfg"))
	c.Check(eth1.Location().LineNum, tc.Equals, 2)

	c.Assert(eth2.Definition(), tc.HasLen, 1)
	c.Check(eth2.Definition()[0], tc.Equals, "iface eth2 inet manual")
	c.Check(eth2.Location().Filename, tc.Equals, filepath.Join(basePath, "eth2.cfg"))
	c.Check(eth2.Location().LineNum, tc.Equals, 2)
}

func (s *ParserSuite) TestSourceStanzaFromFileWithStanzaErrors(c *tc.C) {
	_, err := debinterfaces.Parse("testdata/TestInputSourceStanzaWithErrors/interfaces")
	c.Assert(err, tc.NotNil)
	c.Assert(err, tc.FitsTypeOf, &debinterfaces.ParseError{})
	parseError := err.(*debinterfaces.ParseError)
	c.Assert(parseError, tc.DeepEquals, &debinterfaces.ParseError{
		Filename: filepath.Join("testdata", "TestInputSourceStanzaWithErrors", "interfaces.d", "eth1.cfg"),
		Line:     "iface",
		LineNum:  2,
		Message:  "missing device name",
	})
}

func (s *ParserSuite) TestSourceDirectoryStanzaWithRelativeFilenames(c *tc.C) {
	stanzas, err := debinterfaces.Parse("testdata/TestInputSourceDirectoryStanza/interfaces")
	c.Assert(err, tc.IsNil)
	c.Assert(stanzas, tc.HasLen, 3)
	c.Assert(stanzas[0], tc.FitsTypeOf, debinterfaces.AutoStanza{})
	c.Assert(stanzas[1], tc.FitsTypeOf, debinterfaces.IfaceStanza{})
	c.Assert(stanzas[2], tc.FitsTypeOf, debinterfaces.SourceDirectoryStanza{})

	c.Assert(stanzas[0].Location().Filename, tc.Equals, "testdata/TestInputSourceDirectoryStanza/interfaces")
	c.Assert(stanzas[0].Location().LineNum, tc.Equals, 1)

	c.Assert(stanzas[1].Location().Filename, tc.Equals, "testdata/TestInputSourceDirectoryStanza/interfaces")
	c.Assert(stanzas[1].Location().LineNum, tc.Equals, 2)

	c.Assert(stanzas[2].Location().Filename, tc.Equals, "testdata/TestInputSourceDirectoryStanza/interfaces")
	c.Assert(stanzas[2].Location().LineNum, tc.Equals, 4)

	source := stanzas[2].(debinterfaces.SourceDirectoryStanza)
	c.Check(source.Path, tc.Equals, "interfaces.d")

	c.Assert(source.Stanzas, tc.HasLen, 2)

	c.Assert(source.Stanzas[0], tc.FitsTypeOf, debinterfaces.AutoStanza{})
	c.Assert(source.Stanzas[1], tc.FitsTypeOf, debinterfaces.IfaceStanza{})

	c.Check(source.Sources, tc.DeepEquals, []string{
		filepath.Join("testdata", "TestInputSourceDirectoryStanza", "interfaces.d", "eth3"),
	})

	// Note: we don't have tests for stanzas nested > 1 deep.

	eth3 := source.Stanzas[1].(debinterfaces.IfaceStanza)
	c.Assert(eth3.Definition(), tc.HasLen, 3)
	c.Check(eth3.Definition()[0], tc.Equals, "iface eth3 inet static")
	c.Check(eth3.Definition()[1], tc.Equals, "address 192.168.1.128")
	c.Check(eth3.Definition()[2], tc.Equals, "dns-nameservers 192.168.1.254")
	c.Check(eth3.Location().Filename, tc.Equals, filepath.Join("testdata", "TestInputSourceDirectoryStanza", "interfaces.d", "eth3"))
	c.Check(eth3.Location().LineNum, tc.Equals, 2)
}

func (s *ParserSuite) TestSourceDirectoryStanzaFromDirectoryWithStanzaErrors(c *tc.C) {
	_, err := debinterfaces.Parse("testdata/TestInputSourceDirectoryStanzaWithErrors/interfaces")
	c.Assert(err, tc.NotNil)
	c.Assert(err, tc.FitsTypeOf, &debinterfaces.ParseError{})
	parseError := err.(*debinterfaces.ParseError)
	c.Assert(parseError, tc.DeepEquals, &debinterfaces.ParseError{
		Filename: filepath.Join("testdata", "TestInputSourceDirectoryStanzaWithErrors", "interfaces.d", "eth3"),
		Line:     "iface",
		LineNum:  2,
		Message:  "missing device name",
	})
}

func (s *ParserSuite) TestSourceStanzaWithWordExpanderError(c *tc.C) {
	_, err := debinterfaces.ParseSource("testdata/TestInputSourceStanza/interfaces", nil, wordExpanderWithError("boom"))
	c.Assert(err, tc.NotNil)
	c.Assert(err, tc.FitsTypeOf, &debinterfaces.ParseError{})
	parseError := err.(*debinterfaces.ParseError)
	c.Assert(parseError, tc.DeepEquals, &debinterfaces.ParseError{
		Filename: "testdata/TestInputSourceStanza/interfaces",
		Line:     "source interfaces.d/*.cfg",
		LineNum:  12,
		Message:  "word expansion failed: boom",
	})
}

func (s *ParserSuite) TestSourceDirectoryStanzaWithWordExpanderError(c *tc.C) {
	_, err := debinterfaces.ParseSource("testdata/TestInputSourceDirectoryStanza/interfaces", nil, wordExpanderWithError("boom"))
	c.Assert(err, tc.NotNil)
	c.Assert(err, tc.FitsTypeOf, &debinterfaces.ParseError{})
	parseError := err.(*debinterfaces.ParseError)
	c.Assert(parseError, tc.DeepEquals, &debinterfaces.ParseError{
		Filename: "testdata/TestInputSourceDirectoryStanza/interfaces",
		Line:     "source-directory interfaces.d",
		LineNum:  4,
		Message:  "word expansion failed: boom",
	})
}

func (s *ParserSuite) TestSourceStanzaWithAbsoluteFilenames(c *tc.C) {
	dir, err := os.Getwd()
	c.Assert(err, tc.ErrorIsNil)
	fullpath := fmt.Sprintf("%s/testdata/TestInputSourceStanza/interfaces.d/*.cfg", dir)
	content := fmt.Sprintf("source %s", fullpath)
	stanzas, err := debinterfaces.ParseSource("", content, s.expander)
	c.Assert(err, tc.IsNil)
	c.Assert(stanzas, tc.HasLen, 1)
	c.Assert(stanzas[0], tc.FitsTypeOf, debinterfaces.SourceStanza{})

	c.Assert(stanzas[0].Location().Filename, tc.Equals, "")
	c.Assert(stanzas[0].Location().LineNum, tc.Equals, 1)

	source := stanzas[0].(debinterfaces.SourceStanza)
	c.Check(source.Path, tc.Equals, fullpath)

	c.Assert(source.Stanzas, tc.HasLen, 6)

	c.Assert(source.Stanzas[0], tc.FitsTypeOf, debinterfaces.AutoStanza{})
	c.Assert(source.Stanzas[1], tc.FitsTypeOf, debinterfaces.IfaceStanza{})
	c.Assert(source.Stanzas[2], tc.FitsTypeOf, debinterfaces.AutoStanza{})
	c.Assert(source.Stanzas[3], tc.FitsTypeOf, debinterfaces.IfaceStanza{})
	c.Assert(source.Stanzas[4], tc.FitsTypeOf, debinterfaces.AutoStanza{})
	c.Assert(source.Stanzas[5], tc.FitsTypeOf, debinterfaces.IfaceStanza{})

	c.Check(source.Sources, tc.DeepEquals, []string{
		filepath.Join(dir, "testdata/TestInputSourceStanza/interfaces.d/eth0.cfg"),
		filepath.Join(dir, "testdata/TestInputSourceStanza/interfaces.d/eth1.cfg"),
		filepath.Join(dir, "testdata/TestInputSourceStanza/interfaces.d/eth2.cfg"),
	})

	// Note: we don't have tests for stanzas nested > 1 deep.

	eth0 := source.Stanzas[1].(debinterfaces.IfaceStanza)
	eth1 := source.Stanzas[3].(debinterfaces.IfaceStanza)
	eth2 := source.Stanzas[5].(debinterfaces.IfaceStanza)

	c.Assert(eth0.Definition(), tc.HasLen, 1)
	c.Check(eth0.Definition()[0], tc.Equals, "iface eth0 inet dhcp")
	c.Check(eth0.Location().Filename, tc.Equals, filepath.Join(dir, "testdata/TestInputSourceStanza/interfaces.d/eth0.cfg"))
	c.Check(eth0.Location().LineNum, tc.Equals, 2)

	c.Assert(eth1.Definition(), tc.HasLen, 3)
	c.Check(eth1.Definition()[0], tc.Equals, "iface eth1 inet static")
	c.Check(eth1.Definition()[1], tc.Equals, "address 192.168.1.64")
	c.Check(eth1.Definition()[2], tc.Equals, "dns-nameservers 192.168.1.254")
	c.Check(eth1.Location().Filename, tc.Equals, filepath.Join(dir, "testdata/TestInputSourceStanza/interfaces.d/eth1.cfg"))
	c.Check(eth1.Location().LineNum, tc.Equals, 2)

	c.Assert(eth2.Definition(), tc.HasLen, 1)
	c.Check(eth2.Definition()[0], tc.Equals, "iface eth2 inet manual")
	c.Check(eth2.Location().Filename, tc.Equals, filepath.Join(dir, "testdata/TestInputSourceStanza/interfaces.d/eth2.cfg"))
	c.Check(eth2.Location().LineNum, tc.Equals, 2)
}

func (s *ParserSuite) TestSourceDirectoryStanzaWithAbsoluteFilenames(c *tc.C) {
	dir, err := os.Getwd()
	c.Assert(err, tc.ErrorIsNil)
	fullpath := fmt.Sprintf("%s/testdata/TestInputSourceDirectoryStanza/interfaces.d", dir)
	content := fmt.Sprintf("source-directory %s", fullpath)
	stanzas, err := debinterfaces.ParseSource("", content, s.expander)
	c.Assert(err, tc.IsNil)
	c.Assert(stanzas, tc.HasLen, 1)
	c.Assert(stanzas[0], tc.FitsTypeOf, debinterfaces.SourceDirectoryStanza{})

	c.Assert(stanzas[0].Location().Filename, tc.Equals, "")
	c.Assert(stanzas[0].Location().LineNum, tc.Equals, 1)

	source := stanzas[0].(debinterfaces.SourceDirectoryStanza)
	c.Check(source.Path, tc.Equals, fullpath)

	c.Assert(source.Stanzas, tc.HasLen, 2)

	c.Assert(source.Stanzas[0], tc.FitsTypeOf, debinterfaces.AutoStanza{})
	c.Assert(source.Stanzas[1], tc.FitsTypeOf, debinterfaces.IfaceStanza{})

	c.Check(source.Sources, tc.DeepEquals, []string{
		filepath.Join(fullpath, "eth3"),
	})

	// Note: we don't have tests for source directory stanzas nested > 1 deep.

	eth3 := source.Stanzas[1].(debinterfaces.IfaceStanza)
	c.Assert(eth3.Definition(), tc.HasLen, 3)
	c.Check(eth3.Definition()[0], tc.Equals, "iface eth3 inet static")
	c.Check(eth3.Definition()[1], tc.Equals, "address 192.168.1.128")
	c.Check(eth3.Definition()[2], tc.Equals, "dns-nameservers 192.168.1.254")
	c.Check(eth3.Location().Filename, tc.Equals, filepath.Join(fullpath, "eth3"))
	c.Check(eth3.Location().LineNum, tc.Equals, 2)
}

func (s *ParserSuite) TestSourceStanzaWithAbsoluteNonExistentFilenames(c *tc.C) {
	dir, err := os.Getwd()
	c.Assert(err, tc.ErrorIsNil)
	fullpath := fmt.Sprintf("%s/testdata/non-existent.d/*", dir)
	content := fmt.Sprintf("source %s", fullpath)
	_, err = debinterfaces.ParseSource("", content, s.expander)
	c.Assert(err, tc.IsNil)
}

func (s *ParserSuite) TestSourceDirectoryStanzaWithAbsoluteNonExistentFilenames(c *tc.C) {
	dir, err := os.Getwd()
	c.Assert(err, tc.ErrorIsNil)
	fullpath := fmt.Sprintf("%s/testdata/non-existent", dir)
	content := fmt.Sprintf("source-directory %s", fullpath)
	_, err = debinterfaces.ParseSource("", content, s.expander)
	c.Assert(err, tc.NotNil)
}

func (s *ParserSuite) TestIfupdownPackageExample(c *tc.C) {
	_, err := debinterfaces.ParseSource("testdata/ifupdown-examples", nil, s.expander)
	c.Assert(err, tc.IsNil)
}
