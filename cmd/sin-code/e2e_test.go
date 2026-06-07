package main

import (
	"flag"
	"os"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

var updateFlag = flag.Bool("update", false, "update golden/script files")

func TestMain(m *testing.M) {
	os.Exit(testscript.RunMain(m, map[string]func() int{
		"sin-code": func() int {
			if err := rootCmd.Execute(); err != nil {
				return 1
			}
			return 0
		},
	}))
}

func TestE2E(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir:           "testdata/scripts",
		UpdateScripts: *updateFlag,
	})
}
