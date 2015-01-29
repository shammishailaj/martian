//
// Copyright (c) 2014 10X Genomics, Inc. All rights reserved.
//
// Martian MRO editor.
//
package main

import (
	"github.com/docopt/docopt.go"
	"martian/core"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func main() {
	core.SetupSignalHandlers()

	//=========================================================================
	// Commandline argument and environment variables.
	//=========================================================================
	// Parse commandline.
	doc := `Martian MRO Editor.

Usage:
    mre [--port=<num>]
    mre -h | --help | --version

Options:
    --port=<num>  Serve UI at http://localhost:<num>
                    Overrides $MROPORT_EDITOR environment variable.
                    Defaults to 3601 if not otherwise specified.
    -h --help     Show this message.
    --version     Show version.`
	martianVersion := core.GetVersion()
	opts, _ := docopt.Parse(doc, nil, true, martianVersion, false)
	core.LogInfo("*", "Martian MRO Editor")
	core.LogInfo("version", martianVersion)
	core.LogInfo("cmdline", strings.Join(os.Args, " "))

	// Compute UI port.
	uiport := "3601"
	if value := os.Getenv("MROPORT_EDITOR"); len(value) > 0 {
		core.LogInfo("environ", "MROPORT_EDITOR = %s", value)
		uiport = value
	}
	if value := opts["--port"]; value != nil {
		uiport = value.(string)
	}

	// Compute MRO path.
	cwd, _ := filepath.Abs(path.Dir(os.Args[0]))
	mroPath := cwd
	if value := os.Getenv("MROPATH"); len(value) > 0 {
		mroPath = value
	}
	core.LogInfo("environ", "MROPATH = %s", mroPath)

	// Compute version.
	mroVersion, err := core.GetGitTag(mroPath)
	if err == nil {
		core.LogInfo("version", "MRO_STAGES = %s", mroVersion)
	}

	//=========================================================================
	// Configure Martian runtime.
	//=========================================================================
	rt := core.NewRuntime("local", "disable", mroPath, martianVersion, mroVersion, false, false, false)

	//=========================================================================
	// Start web server.
	//=========================================================================
	go runWebServer(uiport, rt, mroPath)

	// Let daemons take over.
	done := make(chan bool)
	<-done
}
