package e2e

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/evmos/evmos/v10/tests/e2e/upgrade"
)

const (
	localRepository       = "evmos"
	localVersionTag       = "local"
	defaultChainID        = "evmos_9000-1"
	defaultManagerNetwork = "evmos-local"
	tharsisRepo           = "tharsishq/evmos"
	firstUpgradeHeight    = 50
)

type upgradeParams struct {
	InitialVersion string
	TargetVersion  string
	MountPath      string

	ChainID     string
	TargetRepo  string
	SkipCleanup bool
}

type IntegrationTestSuite struct {
	suite.Suite

	upgradeManager *upgrade.Manager
	upgradeParams  upgradeParams
}

func TestIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}

func (s *IntegrationTestSuite) SetupSuite() {
	s.T().Log("setting up e2e integration test suite...")
	var err error

	s.loadUpgradeParams()

	s.upgradeManager, err = upgrade.NewManager(defaultManagerNetwork)
	s.Require().NoError(err, "upgrade manager creation error")
}

func (s *IntegrationTestSuite) runInitialNode() {
	node := upgrade.NewNode(localRepository, s.upgradeParams.InitialVersion)
	err := s.upgradeManager.RunNode(node)
	s.Require().NoError(err, "can't run initial node")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// wait untill node starts and produce some blocks
	err = s.upgradeManager.WaitForHeight(ctx, 5)
	s.Require().NoError(err)

	s.T().Logf("successfully started initial node version: [%s]", s.upgradeParams.InitialVersion)
}

func (s *IntegrationTestSuite) proposeUpgrade() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec, err := s.upgradeManager.CreateSubmitProposalExec(
		s.upgradeParams.TargetVersion,
		defaultChainID,
		firstUpgradeHeight,
	)
	s.Require().NoError(err, "can't create submit proposal exec")
	outBuf, errBuf, err := s.upgradeManager.RunExec(ctx, exec)
	s.Require().NoErrorf(
		err,
		"failed to submit upgrade proposal; stdout: %s, stderr: %s", outBuf.String(), errBuf.String(),
	)

	s.Require().Truef(
		strings.Contains(outBuf.String(), "code: 0"),
		"tx returned non code 0"+outBuf.String(),
	)

	s.T().Logf(
		"successfully submitted upgrade proposal: upgrade height: [%d] upgrade version: [%s]",
		firstUpgradeHeight,
		s.upgradeParams.TargetVersion,
	)
}

func (s *IntegrationTestSuite) depositToProposal() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exec, err := s.upgradeManager.CreateDepositProposalExec(defaultChainID)
	s.Require().NoError(err, "can't create deposit to proposal tx exec")
	outBuf, errBuf, err := s.upgradeManager.RunExec(ctx, exec)
	s.Require().NoErrorf(
		err,
		"failed to submit deposit to proposal tx; stdout: %s, stderr: %s", outBuf.String(), errBuf.String(),
	)

	s.Require().Truef(
		strings.Contains(outBuf.String(), "code: 0"),
		"tx returned non code 0"+outBuf.String(),
	)

	s.T().Logf("successfully deposited to proposal")
}

func (s *IntegrationTestSuite) voteForProposal() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exec, err := s.upgradeManager.CreateVoteProposalExec(defaultChainID)
	s.Require().NoError(err, "can't create vote for proposal exec")
	outBuf, errBuf, err := s.upgradeManager.RunExec(ctx, exec)
	s.Require().NoErrorf(
		err,
		"failed to vote for proposal tx; stdout: %s, stderr: %s", outBuf.String(), errBuf.String(),
	)

	s.Require().Truef(
		strings.Contains(outBuf.String(), "code: 0"),
		"tx returned non code 0"+outBuf.String(),
	)

	s.T().Logf("successfully voted for upgrade proposal")
}

func (s *IntegrationTestSuite) upgrade() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.T().Log("wait for node to reach upgrade height...")
	// wait for proposed upgrade height
	err := s.upgradeManager.WaitForHeight(ctx, firstUpgradeHeight)
	s.Require().NoError(err, "can't reach upgrade height")
	buildDir := strings.Split(s.upgradeParams.MountPath, ":")[0]

	s.T().Log("exporting state to local...")
	// export node .evmosd to local build/
	err = s.upgradeManager.ExportState(buildDir)
	s.Require().NoError(err, "can't export node container state to local")

	s.T().Log("killing initial node...")
	err = s.upgradeManager.KillCurrentNode()
	s.Require().NoError(err, "can't kill current node")

	s.T().Logf(
		"starting upgraded node: version: [%s] mount point: [%s]",
		s.upgradeParams.TargetVersion,
		s.upgradeParams.MountPath,
	)

	node := upgrade.NewNode(localRepository, localVersionTag)
	node.Mount(s.upgradeParams.MountPath)
	err = s.upgradeManager.RunNode(node)
	s.Require().NoError(err, "can't mount and run upgraded node container")

	s.T().Log("node started! waiting for node to produce 25 blocks")
	// make sure node produce blocks after upgrade
	err = s.upgradeManager.WaitForHeight(ctx, firstUpgradeHeight+25)
	s.Require().NoError(err, "node not produce blocks")
}

func (s *IntegrationTestSuite) TearDownSuite() {
	if s.upgradeParams.SkipCleanup {
		return
	}
	s.T().Log("tearing down e2e integration test suite...")

	s.Require().NoError(s.upgradeManager.KillCurrentNode())

	s.Require().NoError(s.upgradeManager.RemoveNetwork())

	s.Require().NoError(os.RemoveAll(strings.Split(s.upgradeParams.MountPath, ":")[0]))
}
