// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package cmdtesting

import (
	"bytes"
	"context"
	"io/ioutil"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	"github.com/juju/tc"
)

// NewFlagSet creates a new flag set using the standard options, particularly
// the option to stop the gnuflag methods from writing to StdErr or StdOut.
func NewFlagSet() *gnuflag.FlagSet {
	fs := gnuflag.NewFlagSet("", gnuflag.ContinueOnError)
	fs.SetOutput(ioutil.Discard)
	return fs
}

// InitCommand will create a new flag set, and call the Command's SetFlags and
// Init methods with the appropriate args.
func InitCommand(c cmd.Command, args []string) error {
	f := gnuflag.NewFlagSetWithFlagKnownAs(c.Info().Name, gnuflag.ContinueOnError, cmd.FlagAlias(c, "flag"))
	f.SetOutput(ioutil.Discard)
	c.SetFlags(f)
	if err := f.Parse(c.AllowInterspersedFlags(), args); err != nil {
		return err
	}
	return c.Init(f.Args())
}

// Context creates a simple command execution context with the current
// dir set to a newly created directory within the test directory.
func Context(c tc.LikeC) *cmd.Context {
	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdin:  &bytes.Buffer{},
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	}
	ctx.Context = context.Background()
	return ctx
}

// Stdout takes a command Context that we assume has been created in this
// package, and gets the content of the Stdout buffer as a string.
func Stdout(ctx *cmd.Context) string {
	return ctx.Stdout.(*bytes.Buffer).String()
}

// Stderr takes a command Context that we assume has been created in this
// package, and gets the content of the Stderr buffer as a string.
func Stderr(ctx *cmd.Context) string {
	return ctx.Stderr.(*bytes.Buffer).String()
}

// RunCommand runs a command with the specified args.  The returned error
// may come from either the parsing of the args, the command initialisation, or
// the actual running of the command.  Access to the resulting output streams
// is provided through the returned context instance.
func RunCommand(c *tc.C, com cmd.Command, args ...string) (*cmd.Context, error) {
	var context = Context(c)
	return runCommand(context, com, args)
}

func runCommand(ctx *cmd.Context, com cmd.Command, args []string) (*cmd.Context, error) {
	if err := InitCommand(com, args); err != nil {
		cmd.WriteError(ctx.Stderr, err)
		return ctx, err
	}
	return ctx, com.Run(ctx)
}
