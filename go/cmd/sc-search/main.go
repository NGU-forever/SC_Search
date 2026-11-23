package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/frankxiong/sc-search/internal/bridge"
	"github.com/frankxiong/sc-search/internal/db"
	"github.com/frankxiong/sc-search/internal/ingest"
	scmcp "github.com/frankxiong/sc-search/internal/mcp"
	"github.com/frankxiong/sc-search/internal/query"
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
	case "ingest":
		if err := runIngest(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "sc-search ingest: %v\n", err)
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
	fmt.Fprintf(os.Stderr, "usage: sc-search <mcp|bridge-smoke|ingest|help>\n")
	fmt.Fprintf(os.Stderr, "  ingest init     create/apply schema\n")
	fmt.Fprintf(os.Stderr, "  ingest status   show recent ingest_runs\n")
}

func runMCP() error {
	root, err := bridge.FindRepoRoot("")
	if err != nil {
		return err
	}
	caller := bridge.New(root)
	query.SetGuidesRoot(root)
	sqlDB, err := db.Open(root)
	if err != nil {
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

func runIngest(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: sc-search ingest <init|status>")
	}
	root, err := bridge.FindRepoRoot("")
	if err != nil {
		return err
	}
	path := ingest.DefaultDBPath(root)
	switch args[0] {
	case "init":
		conn, err := ingest.Open(path)
		if err != nil {
			return err
		}
		defer conn.Close()
		if err := ingest.InitDB(conn); err != nil {
			return err
		}
		fmt.Printf("initialized %s\n", path)
		return nil
	case "status":
		if _, err := os.Stat(path); err != nil {
			payload, _ := json.Marshal(map[string]any{"ok": false, "error": fmt.Sprintf("db missing: %s", path)})
			fmt.Println(string(payload))
			return fmt.Errorf("db missing")
		}
		conn, err := ingest.Open(path)
		if err != nil {
			return err
		}
		defer conn.Close()
		runs, err := ingest.RecentRuns(conn, 20)
		if err != nil {
			return err
		}
		payload, _ := json.MarshalIndent(map[string]any{"ok": true, "db": path, "runs": runs}, "", "  ")
		fmt.Println(string(payload))
		return nil
	default:
		return fmt.Errorf("unknown ingest subcommand %q", args[0])
	}
}
