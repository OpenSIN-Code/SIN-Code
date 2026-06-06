// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the advanced tool subcommands (ibd, poc, sckg, adw, oracle, efm).
package internal

import (
	"testing"
)

func TestIbdCmd_RequiresBeforeAndAfter(t *testing.T) {
	ibdBefore = ""
	ibdAfter = ""
	ibdIntent = ""
	ibdFormat = "text"
	err := IbdCmd.RunE(IbdCmd, []string{})
	if err == nil {
		t.Error("expected error when --before and --after are missing")
	}
}

func TestIbdCmd_RunWithValidInputs(t *testing.T) {
	dir := t.TempDir()
	ibdBefore = dir + "/before.py"
	ibdAfter = dir + "/after.py"
	ibdIntent = "add retry logic"
	ibdFormat = "text"
	_ = IbdCmd.RunE(IbdCmd, []string{})
}

func TestPocCmd_RequiresSpecAndCode(t *testing.T) {
	pocSpec = ""
	pocCode = ""
	pocFormat = "text"
	err := PocCmd.RunE(PocCmd, []string{})
	if err == nil {
		t.Error("expected error when --spec and --code are missing")
	}
}

func TestPocCmd_RunWithValidInputs(t *testing.T) {
	dir := t.TempDir()
	pocSpec = ""
	pocCode = dir
	pocFormat = "text"
	_ = PocCmd.RunE(PocCmd, []string{})
}

func TestSckgCmd_ActionBuild(t *testing.T) {
	dir := t.TempDir()
	sckgAction = "build"
	sckgQuery = ""
	sckgFormat = "text"
	_ = SckgCmd.RunE(SckgCmd, []string{dir})
}

func TestAdwCmd_RunWithTempDir(t *testing.T) {
	dir := t.TempDir()
	adwPath = ""
	adwFormat = "text"
	adwStrict = false
	_ = AdwCmd.RunE(AdwCmd, []string{dir})
}

func TestOracleCmd_RequiresClaim(t *testing.T) {
	oracleClaim = ""
	oracleEvidence = ""
	oracleFormat = "text"
	err := OracleCmd.RunE(OracleCmd, []string{})
	if err == nil {
		t.Error("expected error when --claim is missing")
	}
}

func TestOracleCmd_RequiresEvidence(t *testing.T) {
	oracleClaim = "src.py"
	oracleEvidence = ""
	oracleFormat = "text"
	err := OracleCmd.RunE(OracleCmd, []string{})
	if err == nil {
		t.Error("expected error when --evidence is missing")
	}
}

func TestEfmCmd_RunList(t *testing.T) {
	efmAction = "list"
	efmStack = ""
	efmTTL = 0
	efmFormat = "text"
	if err := EfmCmd.RunE(EfmCmd, []string{}); err != nil {
		t.Errorf("RunE failed: %v", err)
	}
}
