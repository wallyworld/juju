// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
)

const PoolListCommandDoc = `
Lists storage pools.
The user can filter on pool type, name.

* note use of positional arguments

options:
-e, --environment (= "")
   juju environment to operate in
-o, --output (= "")
   specify an output
--format (= yaml)
   specify output format (json|tabular|yaml)
<pool type>
<pool name>

`

// PoolListCommand lists storage pools.
type PoolListCommand struct {
	PoolCommandBase
	Type []string
	Name []string
	out  cmd.Output
}

// Init implements Command.Init.
func (c *PoolListCommand) Init(args []string) (err error) {
	return nil
}

// Info implements Command.Info.
func (c *PoolListCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Purpose: "list storage pools",
		Doc:     PoolListCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *PoolListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.StorageCommandBase.SetFlags(f)
	f.Var(cmd.NewAppendStringsValue(&c.Type), "type", "only show pools of these types")
	f.Var(cmd.NewAppendStringsValue(&c.Name), "name", "only show pools with these names")

	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.formatTabular,
	})
}

// PoolInfo defines the serialization behaviour of the storage pool information.
type PoolInfo struct {
	Name   string                 `yaml:"name" json:"name"`
	Type   string                 `yaml:"type" json:"type"`
	Traits map[string]interface{} `yaml:"characteristics,omitempty" json:"characteristics,omitempty"`
}

// Run implements Command.Run.
func (c *PoolListCommand) Run(ctx *cmd.Context) (err error) {
	api, err := getPoolListAPI(c)
	if err != nil {
		return err
	}
	defer api.Close()

	result, err := api.PoolList(c.Type, c.Name)
	if err != nil {
		return err
	}
	output := c.convertFromAPIPools(result)
	return c.out.Write(ctx, output)
}

var (
	getPoolListAPI = (*PoolListCommand).getPoolListAPI
)

// PoolListAPI defines the API methods that the storage commands use.
type PoolListAPI interface {
	Close() error
	PoolList(types, names []string) ([]params.PoolInstance, error)
}

func (c *PoolListCommand) getPoolListAPI() (PoolListAPI, error) {
	return c.NewStorageAPI()
}

func (c *PoolListCommand) convertFromAPIPools(all []params.PoolInstance) []PoolInfo {
	var output []PoolInfo
	for _, one := range all {
		outInfo := PoolInfo{
			Name:   one.Name,
			Type:   one.Type,
			Traits: one.Traits,
		}
		output = append(output, outInfo)
	}
	return output
}

func (c *PoolListCommand) formatTabular(value interface{}) ([]byte, error) {
	pools, valueConverted := value.([]PoolInfo)
	if !valueConverted {
		return nil, errors.Errorf("expected value of type %T, got %T", pools, value)
	}
	var out bytes.Buffer
	const (
		// To format things into columns.
		minwidth = 0
		tabwidth = 1
		padding  = 2
		padchar  = ' '
		flags    = 0
	)
	tw := tabwriter.NewWriter(&out, minwidth, tabwidth, padding, padchar, flags)
	fmt.Fprintf(tw, "TYPE\tNAME\tCHARACTERISTICS\n")
	for _, pool := range pools {
		traits := make([]string, len(pool.Traits))
		var i int
		for key, value := range pool.Traits {
			traits[i] = fmt.Sprintf("%v=%v", key, value)
			i++
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\n", pool.Type, pool.Name, strings.Join(traits, ","))
	}
	tw.Flush()
	return out.Bytes(), nil
}
