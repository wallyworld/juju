package storage

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/juju/errors"
)

// formatPoolListTabular returns a tabular summary of pool instances.
func formatPoolListTabular(value interface{}) ([]byte, error) {
	pools, ok := value.(map[string]PoolInfo)
	if !ok {
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
	p := func(values ...interface{}) {
		for _, v := range values {
			fmt.Fprintf(tw, "%v\t", v)
		}
		fmt.Fprintln(tw)
	}

	poolNames := make([]string, 0, len(pools))
	for name := range pools {
		poolNames = append(poolNames, name)
	}
	sort.Strings(byPoolName(poolNames))

	p("NAME\tTYPE\tCONFIG")
	for _, name := range poolNames {
		pool := pools[name]
		traits := make([]string, len(pool.Config))
		var i int
		for key, value := range pool.Config {
			traits[i] = fmt.Sprintf("%v=%v", key, value)
			i++
		}
		p(name, pool.Type, strings.Join(traits, ","))
	}
	tw.Flush()

	return out.Bytes(), nil
}

type byPoolName []string

func (s byPoolName) Len() int {
	return len(s)
}

func (s byPoolName) Swap(a, b int) {
	s[a], s[b] = s[b], s[a]
}

func (s byPoolName) Less(a, b int) bool {
	return s[a] < s[b]
}
