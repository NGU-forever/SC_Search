package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/frankxiong/sc-search/internal/db"
	"github.com/frankxiong/sc-search/internal/ingest"
	scmcp "github.com/frankxiong/sc-search/internal/mcp"
	"github.com/frankxiong/sc-search/internal/query"
	"github.com/frankxiong/sc-search/internal/repo"
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
	case "ingest":
		if err := runIngest(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "sc-search ingest: %v\n", err)
			os.Exit(1)
		}
	case "mcp-config":
		// Hidden helper for setup_mcp: renders MCP configs with absolute paths.
		// No Python/Go toolchain needed on the user's machine.
		if len(os.Args) != 5 {
			fmt.Fprintf(os.Stderr, "usage: sc-search mcp-config <ROOT> <BIN> <DB>\n")
			os.Exit(2)
		}
		if err := renderMCPConfigs(os.Args[2], os.Args[3], os.Args[4]); err != nil {
			fmt.Fprintf(os.Stderr, "sc-search mcp-config: %v\n", err)
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
	fmt.Fprintf(os.Stderr, "usage: sc-search <mcp|ingest|mcp-config|help>\n")
	fmt.Fprintf(os.Stderr, "  ingest init            create/apply schema\n")
	fmt.Fprintf(os.Stderr, "  ingest status          show recent ingest_runs\n")
	fmt.Fprintf(os.Stderr, "  ingest sync aliases|wiki|localization|uex|scmdb|scw|scw-blueprints\n")
}

func runMCP() error {
	// MCP hosts only guarantee command+args+env; cwd is not part of the stdio
	// contract, so never require a repo root here. SC_SEARCH_DB is enough, and
	// a release zip still works with no env at all via its own directory.
	root, rootErr := repo.FindRoot("")
	if rootErr != nil {
		if bin, err := os.Executable(); err == nil {
			candidate := filepath.Dir(filepath.Dir(bin))
			if _, statErr := os.Stat(filepath.Join(candidate, "data", "sc_knowledge.sqlite")); statErr == nil {
				root = candidate
			}
		}
	}
	if root != "" {
		query.SetGuidesRoot(root)
	}
	sqlDB, err := db.Open(root)
	if err != nil {
		return fmt.Errorf("open DB: %w (set SC_SEARCH_DB to data/sc_knowledge.sqlite or run: sc-search ingest init)", err)
	}
	defer sqlDB.Close()
	server := scmcp.NewServer(sqlDB)
	return server.Run(context.Background(), &mcp.StdioTransport{})
}

func runIngest(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: sc-search ingest <init|status|sync>")
	}
	root, err := repo.FindRoot("")
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
	case "sync":
		if len(args) < 2 {
			return fmt.Errorf("usage: sc-search ingest sync <aliases|wiki|localization|uex|scmdb|scw|scw-blueprints>")
		}
		conn, err := ingest.Open(path)
		if err != nil {
			return err
		}
		defer conn.Close()
		if err := ingest.InitDB(conn); err != nil {
			return err
		}
		switch args[1] {
		case "aliases":
			n, err := ingest.SyncAliases(conn, root, "")
			if err != nil {
				return err
			}
			payload, _ := json.Marshal(map[string]any{
				"ok": true, "source": "aliases", "row_count": n, "db": path,
			})
			fmt.Println(string(payload))
			return nil
		case "wiki":
			n, err := ingest.SyncWikiLive(conn, nil, nil, 0)
			if err != nil {
				return err
			}
			payload, _ := json.Marshal(map[string]any{
				"ok": true, "source": "starcitizen_tools", "row_count": n, "db": path,
			})
			fmt.Println(string(payload))
			return nil
		case "localization", "zh":
			n, err := ingest.SyncLocalization(conn, root, nil, nil)
			if err != nil {
				return err
			}
			payload, _ := json.Marshal(map[string]any{
				"ok": true, "source": "localization", "row_count": n, "db": path,
			})
			fmt.Println(string(payload))
			return nil
		case "uex":
			n, err := ingest.SyncUexLive(conn, root)
			if err != nil {
				return err
			}
			payload, _ := json.Marshal(map[string]any{
				"ok": true, "source": "uex", "row_count": n, "db": path,
			})
			fmt.Println(string(payload))
			return nil
		case "scmdb":
			channel := "live"
			version := ""
			if len(args) >= 3 {
				channel = args[2]
			}
			if len(args) >= 4 {
				version = args[3]
			}
			n, err := ingest.SyncScmdbLive(conn, root, channel, version)
			if err != nil {
				return err
			}
			payload, _ := json.Marshal(map[string]any{
				"ok": true, "source": "scmdb", "row_count": n, "db": path,
			})
			fmt.Println(string(payload))
			return nil
		case "scw":
			n, err := ingest.SyncScwVehicles(conn, nil)
			if err != nil {
				return err
			}
			payload, _ := json.Marshal(map[string]any{
				"ok": true, "source": "scw_api", "row_count": n, "db": path,
			})
			fmt.Println(string(payload))
			return nil
		case "scw-blueprints":
			n, err := ingest.SyncScwBlueprints(conn, nil)
			if err != nil {
				return err
			}
			payload, _ := json.Marshal(map[string]any{
				"ok": true, "source": "scw_blueprint", "row_count": n, "db": path,
			})
			fmt.Println(string(payload))
			return nil
		default:
			return fmt.Errorf("unknown sync source %q", args[1])
		}
	default:
		return fmt.Errorf("unknown ingest subcommand %q", args[0])
	}
}
