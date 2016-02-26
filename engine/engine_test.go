package engine

import (
	"errors"
	"testing"

	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
	"github.com/dropbox/changes-client/client/reporter"

	"github.com/stretchr/testify/assert"
)

type noartReporter struct {
	reporter.NoopReporter
}

func (nar *noartReporter) PublishArtifacts(_ client.ConfigCmd, _ adapter.Adapter, _ *client.Log) error {
	return errors.New("Couldn't publish artifacts somehow")
}

type noopAdapter struct {
	cmdIdsToFail map[string]bool
}

// FailCommandForTest takes a Command ID, and ensures that any Command with that
// id passed to Run will fail. Not quite so noop with this.
func (na *noopAdapter) FailCommandForTest(id string) {
	if na.cmdIdsToFail == nil {
		na.cmdIdsToFail = make(map[string]bool)
	}
	na.cmdIdsToFail[id] = true
}

func (_ *noopAdapter) Init(*client.Config) error                   { return nil }
func (_ *noopAdapter) Prepare(*client.Log) (client.Metrics, error) { return nil, nil }
func (na *noopAdapter) Run(cmd *client.Command, _ *client.Log) (*client.CommandResult, error) {
	fail := na.cmdIdsToFail[cmd.ID]
	return &client.CommandResult{
		Success: !fail,
	}, nil
}
func (_ *noopAdapter) Shutdown(*client.Log) (client.Metrics, error) { return nil, nil }
func (_ *noopAdapter) CaptureSnapshot(string, *client.Log) error    { return nil }
func (_ *noopAdapter) GetRootFs() string {
	return "/"
}
func (_ *noopAdapter) GetArtifactRoot() string {
	return "/"
}
func (_ *noopAdapter) CollectArtifacts([]string, *client.Log) ([]string, error) {
	return nil, nil
}

func TestFailedArtifactInfraFails(t *testing.T) {
	nar := new(noartReporter)
	log := client.NewLog()
	defer log.Close()
	go log.Drain()
	eng := Engine{reporter: nar,
		clientLog: log,
		adapter:   &noopAdapter{},
		config: &client.Config{Cmds: []client.ConfigCmd{
			{Artifacts: []string{"result.xml"}},
		}}}
	r, e := eng.executeCommands()
	assert.Equal(t, r, RESULT_INFRA_FAILED)
	assert.Error(t, e)
}

func TestDebugForceInfraFailure(t *testing.T) {
	config, err := client.LoadConfig([]byte(`{"debugConfig": {"forceInfraFailure": true}}`))
	assert.NoError(t, err)
	log := client.NewLog()
	defer log.Close()
	eng := Engine{reporter: &reporter.NoopReporter{},
		clientLog: log,
		adapter:   &noopAdapter{},
		config:    config,
	}

	result, err := eng.runBuildPlan()
	assert.Equal(t, result, RESULT_INFRA_FAILED)
	assert.Error(t, err)
}

func TestInfraSetupCommandFailsInfra(t *testing.T) {
	cmd := client.ConfigCmd{ID: "failme1234", Script: "exit 1"}
	cmd.Type.ID = "infra_setup"
	adapter := &noopAdapter{}
	adapter.FailCommandForTest(cmd.ID)
	log := client.NewLog()
	defer log.Close()
	go log.Drain()
	eng := Engine{reporter: &reporter.NoopReporter{},
		clientLog: log,
		adapter:   adapter,
		config: &client.Config{
			Cmds: []client.ConfigCmd{cmd},
		},
	}

	result, err := eng.runBuildPlan()
	assert.Equal(t, RESULT_INFRA_FAILED, result)
	assert.Error(t, err)
}

func makeResetFunc(s *string) func() {
	previous := *s
	return func() {
		*s = previous
	}
}

func TestOutputSnapshotID(t *testing.T) {
	// Leave things as we found them.
	defer makeResetFunc(&outputSnapshotFlag)()

	type testcase struct {
		Flag, Config string
		// Whether we find an inconsistency.
		Error bool
	}
	cases := []testcase{
		{Flag: "", Config: "1234", Error: true},
		{Flag: "", Config: "", Error: false},
		{Flag: "abcd", Config: "", Error: true},
		{Flag: "abcd", Config: "abcd", Error: false},
		{Flag: "abcd", Config: "1234", Error: true},
	}
	for _, c := range cases {
		var cfg client.Config
		cfg.ExpectedSnapshot.ID = c.Config
		outputSnapshotFlag = c.Flag

		eng := Engine{config: &cfg}
		// For now, flag always wins.
		assert.Equal(t, eng.outputSnapshotID(), c.Flag, "For outputSnapshotID() with %#v", c)
		err := eng.checkForSnapshotInconsistency()
		if c.Error {
			assert.Error(t, err, "%#v", c)
		} else {
			assert.NoError(t, err, "%#v", c)
		}
	}
}
