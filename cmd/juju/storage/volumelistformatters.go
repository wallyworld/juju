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
	p("VOLUME\tATTACHED\tMACHINE\tDEVICE NAME\tSIZE")

	var volumeTags []string
	for _, oneVolume := range volumes {
		for tag := range oneVolume.Attachments {
			volumeTags = append(volumeTags, tag)
		}
		sort.Strings(byVolumeTags(volumeTags))

		for _, tag := range volumeTags {
			attachment := oneVolume.Attachments[tag]
			attachmentSize := "(unknown)"
			if attachment.Size != nil {
				attachmentSize = humanize.IBytes(*attachment.Size * humanize.MiByte)
			}
			p(tag, attachment.Attached, attachment.Machine, attachment.DeviceName, attachmentSize)
		}
	}
	tw.Flush()

	return out.Bytes(), nil
}

type byVolumeTags []string

func (s byVolumeTags) Len() int {
	return len(s)
}

func (s byVolumeTags) Swap(a, b int) {
	s[a], s[b] = s[b], s[a]
}

func (s byVolumeTags) Less(a, b int) bool {
	return s[a] < s[b]
}
