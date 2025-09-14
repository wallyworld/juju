// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"os"
	"path/filepath"
	tctesting "testing"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/lumberjack/v2"
	"github.com/juju/names/v5"
	"github.com/juju/tc"
	"github.com/juju/utils/v3/voyeur"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/jujud/agent/agentconf"
	"github.com/juju/juju/cmd/jujud/agent/caasoperator"
	"github.com/juju/juju/internal/provider/kubernetes/exec"
	coretesting "github.com/juju/juju/internal/testing"
	jujuworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/logsender"
)

type CAASOperatorSuite struct {
	coretesting.BaseSuite

	rootDir string

	prometheus *prometheus.Registry
}

func TestCAASOperatorSuite(t *tctesting.T) {
	tc.Run(t, &CAASOperatorSuite{})
}

func newExecClient(modelName string) (exec.Executor, error) {
	return nil, nil
}

func (s *CAASOperatorSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.rootDir = c.MkDir()
	s.prometheus = prometheus.NewRegistry()
}

func (s *CAASOperatorSuite) dataDir() string {
	return filepath.Join(s.rootDir, "/var/lib/juju")
}

func (s *CAASOperatorSuite) newBufferedLogWriter() *logsender.BufferedLogWriter {
	logger := logsender.NewBufferedLogWriter(1024)
	s.AddCleanup(func(*tc.C) { logger.Close() })
	return logger
}

func (s *CAASOperatorSuite) TestParseSuccess(c *tc.C) {
	// Now init actually reads the agent configuration file.
	a, err := NewCaasOperatorAgent(nil, s.newBufferedLogWriter(), func(mc *caasoperator.ManifoldsConfig) error {
		mc.NewExecClient = newExecClient
		mc.PrometheusRegisterer = s.prometheus
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	err = cmdtesting.InitCommand(a, []string{
		"--data-dir", s.dataDir(),
		"--application-name", "wordpress",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(a.AgentConf.DataDir(), tc.Equals, s.dataDir())
	c.Check(a.ApplicationName, tc.Equals, "wordpress")
}

func (s *CAASOperatorSuite) TestParseMissing(c *tc.C) {
	uc, err := NewCaasOperatorAgent(nil, s.newBufferedLogWriter(), func(mc *caasoperator.ManifoldsConfig) error {
		mc.NewExecClient = newExecClient
		mc.PrometheusRegisterer = s.prometheus
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	err = cmdtesting.InitCommand(uc, []string{
		"--data-dir", "jc",
	})

	c.Assert(err, tc.ErrorMatches, "--application-name option must be set")
}

func (s *CAASOperatorSuite) TestParseNonsense(c *tc.C) {
	for _, args := range [][]string{
		{"--application-name", "wordpress/0"},
		{"--application-name", "wordpress/seventeen"},
		{"--application-name", "wordpress/-32"},
		{"--application-name", "wordpress/wild/9"},
		{"--application-name", "20"},
	} {
		a, err := NewCaasOperatorAgent(nil, s.newBufferedLogWriter(), func(mc *caasoperator.ManifoldsConfig) error {
			mc.NewExecClient = newExecClient
			mc.PrometheusRegisterer = s.prometheus
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)

		err = cmdtesting.InitCommand(a, append(args, "--data-dir", "jc"))
		c.Check(err, tc.ErrorMatches, `--application-name option expects "<application>" argument`)
	}
}

func (s *CAASOperatorSuite) TestParseUnknown(c *tc.C) {
	a, err := NewCaasOperatorAgent(nil, s.newBufferedLogWriter(), func(mc *caasoperator.ManifoldsConfig) error {
		mc.NewExecClient = newExecClient
		mc.PrometheusRegisterer = s.prometheus
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	err = cmdtesting.InitCommand(a, []string{
		"--application-name", "wordpress",
		"thundering typhoons",
	})
	c.Check(err, tc.ErrorMatches, `unrecognized args: \["thundering typhoons"\]`)
}

func (s *CAASOperatorSuite) TestLogStderr(c *tc.C) {
	ctx, err := cmd.DefaultContext()
	c.Assert(err, tc.IsNil)

	a := CaasOperatorAgent{
		AgentConf:       FakeAgentConfig{},
		ctx:             ctx,
		ApplicationName: "mysql",
		dead:            make(chan struct{}),
	}

	err = a.Init(nil)
	c.Assert(err, tc.IsNil)

	_, ok := ctx.Stderr.(*lumberjack.Logger)
	c.Assert(ok, tc.IsFalse)
}

var agentConfigContents = `
# format 2.0
controller: controller-deadbeef-1bad-500d-9000-4b1d0d06f00d
model: model-deadbeef-0bad-400d-8000-4b1d0d06f00d
tag: machine-0
datadir: /home/user/.local/share/juju/local
logdir: /var/log/juju-user-local
upgradedToVersion: 1.2.3
apiaddresses:
- localhost:17070
apiport: 17070
`[1:]

func (s *CAASOperatorSuite) TestRunCopiesConfigTemplate(c *tc.C) {
	ctx, err := cmd.DefaultContext()
	c.Assert(err, tc.IsNil)
	dataDir := c.MkDir()
	agentDir := filepath.Join(dataDir, "agents", "application-mysql")
	err = os.MkdirAll(agentDir, 0700)
	c.Assert(err, tc.IsNil)
	templateFile := filepath.Join(agentDir, "template-agent.conf")

	err = os.WriteFile(templateFile, []byte(agentConfigContents), 0600)
	c.Assert(err, tc.IsNil)

	a := &CaasOperatorAgent{
		AgentConf:          agentconf.NewAgentConf(dataDir),
		ctx:                ctx,
		ApplicationName:    "mysql",
		bufferedLogger:     s.newBufferedLogWriter(),
		dead:               make(chan struct{}),
		prometheusRegistry: prometheus.NewRegistry(),
	}

	dummy := jujuworker.NewSimpleWorker(func(stopCh <-chan struct{}) error {
		return jujuworker.ErrTerminateAgent
	})
	s.PatchValue(&CaasOperatorManifolds, func(config caasoperator.ManifoldsConfig) dependency.Manifolds {
		return dependency.Manifolds{"test": dependency.Manifold{
			Start: func(context dependency.Context) (worker.Worker, error) {
				return dummy, nil
			},
		}}
	})

	err = a.Init(nil)
	c.Assert(err, tc.ErrorIsNil)
	err = a.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { c.Check(a.Stop(), tc.IsNil) }()

	agentConfig := a.CurrentConfig()
	c.Assert(agentConfig.Controller(), tc.Equals, names.NewControllerTag("deadbeef-1bad-500d-9000-4b1d0d06f00d"))
	addr, err := agentConfig.APIAddresses()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr, tc.SameContents, []string{"localhost:17070"})
}

func (s *CAASOperatorSuite) TestChangeConfig(c *tc.C) {
	config := FakeAgentConfig{}
	configChanged := voyeur.NewValue(true)
	a := CaasOperatorAgent{
		AgentConf:          config,
		configChangedVal:   configChanged,
		prometheusRegistry: prometheus.NewRegistry(),
	}

	var mutateCalled bool
	mutate := func(config agent.ConfigSetter) error {
		mutateCalled = true
		return nil
	}

	configChangedCh := make(chan bool)
	watcher := configChanged.Watch()
	watcher.Next() // consume initial event
	go func() {
		configChangedCh <- watcher.Next()
	}()

	err := a.ChangeConfig(mutate)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(mutateCalled, tc.IsTrue)
	select {
	case result := <-configChangedCh:
		c.Check(result, tc.IsTrue)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for config changed signal")
	}
}
