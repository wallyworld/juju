// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"bytes"
	"fmt"
	"text/tabwriter"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
)

const VolumeListCommandDoc = `
List volumes (disks) in the environment.

options:
-e, --environment (= "")
    juju environment to operate in
-o, --output (= "")
    specify an output file
[machine]
    machine ids for filtering the list

`

// VolumeListCommand lists storage volumes.
type VolumeListCommand struct {
	VolumeCommandBase
	Ids []string
	out cmd.Output
}

// Init implements Command.Init.
func (c *VolumeListCommand) Init(args []string) (err error) {
	c.Ids = args
	return nil
}

// Info implements Command.Info.
func (c *VolumeListCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Purpose: "list storage volumes",
		Doc:     VolumeListCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *VolumeListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.StorageCommandBase.SetFlags(f)

	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.formatTabular,
	})
}

// DiskInfo defines the serialization behaviour of the storage volume (disk) information.
type DiskInfo struct {
	Attachments []AttachmentInfo `yaml:"attachments" json:"attachments `
}

type AttachmentInfo struct {
	Volume      string `yaml:"volume" json:"volume"`
	Storage     string `yaml:"storage" json:"storage"`
	Assigned    bool   `yaml:"assigned" json:"assigned"`
	Machine     string `yaml:"machine" json:"machine"`
	Attached    bool   `yaml:"attached" json:"attached"`
	DeviceName  string `yaml:"device-name" json:"device-name"`
	UUID        string `yaml:"uuid" json:"uuid"`
	Label       string `yaml:"label" json:"label"`
	Size        uint64 `yaml:"size" json:"size"`
	InUse       bool   `yaml:"in-use" json:"in-use"`
	FSType      string `yaml:"file-system-type" json:"file-system-type"`
	Provisioned bool   `yaml:"provisioned" json:"provisioned"`
}

// Run implements Command.Run.
func (c *VolumeListCommand) Run(ctx *cmd.Context) (err error) {
	api, err := getVolumeListAPI(c)
	if err != nil {
		return err
	}
	defer api.Close()

	result, err := api.ListVolumes(c.Ids)
	if err != nil {
		return err
	}
	output := c.convertFromAPIDisks(result)
	return c.out.Write(ctx, output)
}

var (
	getVolumeListAPI = (*VolumeListCommand).getVolumeListAPI
)

// VolumeListAPI defines the API methods that the volume list command use.
type VolumeListAPI interface {
	Close() error
	ListVolumes(machines []string) ([]params.StorageDisk, error)
}

func (c *VolumeListCommand) getVolumeListAPI() (VolumeListAPI, error) {
	return c.NewStorageAPI()
}

func (c *VolumeListCommand) convertFromAPIDisks(all []params.StorageDisk) []DiskInfo {
	result := make([]DiskInfo, len(all))
	for i, one := range all {
		result[i] = DiskInfo{
			Attachments: c.convertFromAPIAttachments(one.Attachments),
		}
	}
	return result
}

func (c *VolumeListCommand) convertFromAPIAttachments(all []params.VolumeAttachment) []AttachmentInfo {
	result := make([]AttachmentInfo, len(all))
	for i, one := range all {
		result[i] = AttachmentInfo{
			Volume:      one.Volume,
			Storage:     one.Storage,
			Assigned:    one.Assigned,
			Machine:     one.Machine,
			Attached:    one.Attached,
			DeviceName:  one.DeviceName,
			UUID:        one.UUID,
			Label:       one.Label,
			Size:        one.Size,
			InUse:       one.InUse,
			FSType:      one.FSType,
			Provisioned: one.Provisioned,
		}
	}
	return result
}

func (c *VolumeListCommand) formatTabular(value interface{}) ([]byte, error) {
	disks, valueConverted := value.([]DiskInfo)
	if !valueConverted {
		return nil, errors.Errorf("expected value of type %T, got %T", disks, value)
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
	fmt.Fprintf(tw, "VOLUME\tATTACHED\tMACHINE\tDEVICE NAME\tUUID\tLABEL\tSIZE\n")
	for _, disk := range disks {
		for _, attachment := range disk.Attachments {
			fmt.Fprintf(tw, "%s\t%t\t%s\t%s\t%s\t%s\t%d\n",
				attachment.Volume,
				attachment.Attached,
				attachment.Machine,
				attachment.DeviceName,
				attachment.UUID,
				attachment.Label,
				attachment.Size,
			)
		}
	}
	tw.Flush()
	return out.Bytes(), nil
}
