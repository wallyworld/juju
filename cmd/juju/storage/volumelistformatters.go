package storage

import (
	"bytes"
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/dustin/go-humanize"
	"github.com/juju/errors"
)

// formatVolumeListTabular returns a tabular summary of volume instances.
func formatVolumeListTabular(value interface{}) ([]byte, error) {
	volumes, ok := value.([]VolumeInfo)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", volumes, value)
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

	p := func(values ...interface{}) {
		for _, v := range values {
			fmt.Fprintf(tw, "%v\t", v)
		}
		fmt.Fprintln(tw)
	}
	p("MACHINE\tDEVICE NAME\tVOLUME\tATTACHED\tSIZE")

	for _, oneVolume := range volumes {
		var machines []string
		for m := range oneVolume.Attachments {
			machines = append(machines, m)
		}
		// Order by machines
		sort.Strings(machines)
		for _, aMachine := range machines {

			devices := oneVolume.Attachments[aMachine]
			var deviceNames []string
			for d := range devices {
				deviceNames = append(deviceNames, d)
			}
			// then order by device names
			sort.Strings(deviceNames)
			for _, aDeviceName := range deviceNames {

				volumeNames := devices[aDeviceName]
				var orderedNames []string
				for volumeName := range volumeNames {
					orderedNames = append(orderedNames, volumeName)
				}
				// then order by volume name
				sort.Strings(orderedNames)

				for _, vname := range orderedNames {
					attachment := volumeNames[vname]
					attachmentSize := "(unknown)"
					if attachment.Size != nil {
						attachmentSize = humanize.IBytes(*attachment.Size * humanize.MiByte)
					}
					p(aMachine, aDeviceName, vname, attachment.Attached, attachmentSize)
				}
			}

		}
	}
	tw.Flush()

	return out.Bytes(), nil
}
