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
	ibdBefore = "v1.0"
	ibdAfter = "HEAD"
	ibdIntent = "add retry logic"
	ibdFormat = "text"
	if err := IbdCmd.RunE(IbdCmd, []string{}); err != nil {
		t.Errorf("RunE failed: %v", err)
	}
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
	pocSpec = dir + "/spec.md"
	pocCode = dir + "/main.py"
	pocFormat = "text"
	if err := PocCmd.RunE(PocCmd, []string{}); err != nil {
		t.Errorf("RunE failed: %v", err)
	}
}

func TestSckgCmd_ActionBuild(t *testing.T) {
	dir := t.TempDir()
	sckgAction = "build"
	sckgQuery = ""
	sckgFormat = "text"
	if err := SckgCmd.RunE(SckgCmd, []string{dir}); err != nil {
		t.Errorf("RunE failed: %v", err)
	}
}

func TestAdwCmd_RunWithTempDir(t *testing.T) {
	dir := t.TempDir()
	adwPath = ""
	adwFormat = "text"
	adwStrict = false
	if err := AdwCmd.RunE(AdwCmd, []string{dir}); err != nil {
		t.Errorf("RunE failed: %v", err)
	}
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

func TestEfmCmd_RunList(t *testing.T) {
	efmAction = "list"
	efmStack = ""
	efmTTL = 0
	efmFormat = "text"
	if err := EfmCmd.RunE(EfmCmd, []string{}); err != nil {
		t.Errorf("RunE failed: %v", err)
	}
}
