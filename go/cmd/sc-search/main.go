package main

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/frankxiong/sc-search/internal/bridge"
	"github.com/frankxiong/sc-search/internal/db"
	scmcp "github.com/frankxiong/sc-search/internal/mcp"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "mcp":
		if err := runMCP(); err != nil {
			fmt.Fprintf(os.Stderr, "sc-search mcp: %v\n", err)
			os.Exit(1)
		}
	case "bridge-smoke":
		if err := runBridgeSmoke(); err != nil {
			fmt.Fprintf(os.Stderr, "sc-search bridge-smoke: %v\n", err)
			os.Exit(1)
		}
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: sc-search <mcp|bridge-smoke|help>\n")
}

func runMCP() error {
	root, err := bridge.FindRepoRoot("")
	if err != nil {
		return err
	}
	caller := bridge.New(root)
	sqlDB, err := db.Open(root)
	if err != nil {
		// Still serve bridged tools if DB missing.
		fmt.Fprintf(os.Stderr, "sc-search mcp: native DB unavailable (%v); bridge-only mode\n", err)
		server := scmcp.NewServer(caller, nil)
		return server.Run(context.Background(), &mcp.StdioTransport{})
	}
	defer sqlDB.Close()
	server := scmcp.NewServer(caller, sqlDB)
	return server.Run(context.Background(), &mcp.StdioTransport{})
}

func runBridgeSmoke() error {
	root, err := bridge.FindRepoRoot("")
	if err != nil {
		return err
	}
	caller := bridge.New(root)
	out, err := caller.Call(context.Background(), "sc_intents", map[string]any{})
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}
